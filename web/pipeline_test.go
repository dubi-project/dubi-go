package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestExecuteFiltersRunAfterRoutePreparation(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(dubi.Reset)
	var order []string
	rt.Register("load", dubi.Method{Action: func(ctx *dubi.Context) Map {
		order = append(order, "loading")
		return Map{"id": ctx.Args["id"]}
	}})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	token, err := rt.SignToken(dubi.Token{Auth: true, Kind: "staff", Payload: Map{"staff_id": "s1"}})
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(ServerConfig{Name: defaultServerName, Settings: Map{"server": true}}, map[string]struct{}{defaultServerName: {}})
	err = server.register(map[string]Router{
		"detail": {
			Method: http.MethodGet, Uri: "/items/{id}", Name: "Item detail", Auth: true, Kind: "staff",
			Settings: Map{"power": "item.read"}, Args: Vars{"id": {Type: "int", Required: true}},
			Loading: Loading{"item": {Invoke: "load", Required: true}},
			Actions: []func(*Context){func(ctx *Context) {
				order = append(order, "actions-before")
				ctx.Next()
				order = append(order, "actions-after")
			}},
			Action: func(ctx *Context) {
				order = append(order, "action")
				if ctx.Locals["staff"] != "s1" || ctx.Locals["request"] != true || ctx.Locals["serve"] != true {
					t.Errorf("request locals lost: %#v", ctx.Locals)
				}
				ctx.Text("done")
			},
		},
	}, map[string]Filter{
		"a": {
			Serve: func(ctx *Context) {
				order = append(order, "serve-before")
				ctx.Locals["serve"] = true
				ctx.Next()
				order = append(order, "serve-after")
				if ctx.output.code != http.StatusOK {
					t.Error("Serve unwound before the response completed")
				}
			},
			Request: func(ctx *Context) {
				order = append(order, "request")
				if ctx.Name != "detail" || ctx.Config.Name != "Item detail" || ctx.Settings["power"] != "item.read" || ctx.Params["id"] != "7" {
					t.Error("Request did not receive matched route metadata")
				}
				if len(ctx.Args) != 0 || len(ctx.Query) != 0 {
					t.Error("request processing ran before Request filter")
				}
				ctx.Locals["request"] = true
				ctx.Next()
				order = append(order, "request-after")
			},
			Execute: func(ctx *Context) {
				order = append(order, "before-a")
				if ctx.Name != "detail" || ctx.Config.Name != "Item detail" || ctx.Settings["power"] != "item.read" || ctx.Settings["server"] != true {
					t.Errorf("route metadata unavailable: name=%q config=%#v settings=%#v", ctx.Name, ctx.Config, ctx.Settings)
				}
				if !ctx.Authed() || ctx.Kind() != "staff" || ctx.Payload()["staff_id"] != "s1" {
					t.Error("token was not verified before Execute")
				}
				if fmt.Sprint(ctx.Args["id"]) != "7" || reflect.TypeOf(ctx.Args["id"]).Kind() == reflect.String || ctx.Locals["item"].(Map)["id"] != ctx.Args["id"] {
					t.Errorf("arguments or Loading unavailable: %#v %#v", ctx.Args, ctx.Locals)
				}
				ctx.Locals["staff"] = "s1"
				ctx.Next()
				order = append(order, "after-a")
			},
			Response: func(ctx *Context) {
				order = append(order, "response")
				if ctx.Body == nil || ctx.output.code != 0 {
					t.Error("Response did not run between Action and output")
				}
				ctx.Header("X-Response-Filter", "yes")
				ctx.Next()
				order = append(order, "response-after")
				if ctx.output.code != http.StatusOK {
					t.Error("Response.Next did not write the response")
				}
			},
		},
		"b": {Execute: func(ctx *Context) {
			order = append(order, "before-b")
			ctx.Next()
			order = append(order, "after-b")
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/items/7", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "done" {
		t.Fatalf("response: %d %q", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Response-Filter") != "yes" {
		t.Error("Response ran after headers were sent")
	}
	want := []string{"serve-before", "request", "loading", "before-a", "before-b", "actions-before", "action", "actions-after", "after-b", "after-a", "request-after", "response", "response-after", "serve-after"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order: %v, want %v", order, want)
	}
}

func TestExecuteFiltersSkipUnpreparedRoutes(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(dubi.Reset)
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	executed, actions := 0, 0
	action := func(ctx *Context) { actions++; ctx.Text("unexpected") }
	if err := server.register(map[string]Router{
		"private":  {Uri: "/private", Auth: true, Action: action},
		"args":     {Uri: "/args/{id}", Args: Vars{"id": {Type: "int", Required: true}}, Action: action},
		"load":     {Uri: "/load", Loading: Loading{"item": {Invoke: "missing", Required: true}}, Action: action},
		"post":     {Uri: "/post", Method: http.MethodPost, Action: action},
		"redirect": {Uri: "/trailing/", Action: action},
	}, map[string]Filter{"all": {Execute: func(ctx *Context) { executed++; ctx.Next() }}}, nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		code int
	}{
		{"/private", http.StatusUnauthorized},
		{"/args/bad", http.StatusBadRequest},
		{"/load", http.StatusBadRequest},
		{"/missing", http.StatusNotFound},
		{"/post", http.StatusMethodNotAllowed},
		{"/trailing", http.StatusTemporaryRedirect},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test"+test.path, nil))
			if response.Code != test.code {
				t.Errorf("status = %d, want %d", response.Code, test.code)
			}
			if test.code == http.StatusMethodNotAllowed && response.Header().Get("Allow") != http.MethodPost {
				t.Error("405 lost Allow header")
			}
		})
	}
	if executed != 0 || actions != 0 {
		t.Fatalf("execute=%d actions=%d", executed, actions)
	}
}

func TestExecuteFilterCanStopAction(t *testing.T) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{"private": {
		Uri: "/private", Action: func(*Context) { t.Error("action ran after Execute denied the request") },
	}}, map[string]Filter{"deny": {Execute: func(ctx *Context) { ctx.Denied() }}}, nil); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test/private", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestRequestFilterRunsBeforeParsingAndUsesRouteOutcome(t *testing.T) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	responses := 0
	if err := server.register(map[string]Router{"blocked": {
		Uri: "/blocked", Name: "Blocked route",
		Action: func(*Context) { t.Error("Action ran after Request denied the request") },
		Denied: func(ctx *Context) { ctx.Text("denied by route", http.StatusForbidden) },
	}}, map[string]Filter{"all": {
		Request: func(ctx *Context) {
			if ctx.Name != "blocked" || ctx.Config.Name != "Blocked route" || len(ctx.Form) != 0 {
				t.Error("invalid Request phase")
			}
			ctx.Denied()
		},
		Execute: func(*Context) { t.Error("Execute ran after Request denied the request") },
		Response: func(ctx *Context) {
			responses++
			if ctx.output.code != 0 {
				t.Error("response already committed")
			}
			ctx.Next()
		},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://example.test/blocked", strings.NewReader("invalid JSON"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusForbidden || response.Body.String() != "denied by route" || responses != 1 {
		t.Fatalf("response: %d %q, Response count=%d", response.Code, response.Body.String(), responses)
	}
}

func TestServeFilterRunsBeforeCORSPreflight(t *testing.T) {
	server := newServerState(ServerConfig{Name: defaultServerName, Cross: Map{"allow": true}}, map[string]struct{}{defaultServerName: {}})
	served, responded := 0, 0
	if err := server.register(nil, map[string]Filter{"all": {
		Serve:    func(ctx *Context) { served++; ctx.Next() },
		Request:  func(*Context) { t.Error("preflight entered Request") },
		Execute:  func(*Context) { t.Error("preflight entered Execute") },
		Response: func(ctx *Context) { responded++; ctx.Next() },
	}}, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodOptions, "http://example.test/items", nil)
	request.Header.Set("Origin", "https://client.test")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusNoContent || served != 1 || responded != 1 {
		t.Fatalf("preflight: status=%d Serve=%d Response=%d", response.Code, served, responded)
	}
}
