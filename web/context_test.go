package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestInputPathTokens(t *testing.T) {
	tests := map[string][]string{
		"user.name":    {"user", "name"},
		"users[0].id":  {"users", "0", "id"},
		"tags[]":       {"tags", ""},
		"matrix[0][1]": {"matrix", "0", "1"},
		"broken[":      {"broken", "["},
	}
	for input, want := range tests {
		if got := inputPathTokens(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("inputPathTokens(%q) = %#v, want %#v", input, got, want)
		}
	}
}

func TestPooledContextClearsRequestState(t *testing.T) {
	first := acquireContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	first.Params["stale"] = true
	first.Locals["stale"] = true
	first.Body = "stale"
	releaseContext(first)

	second := acquireContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	defer releaseContext(second)
	if len(second.Params) != 0 || len(second.Locals) != 0 || second.Body != nil || second.Code != 0 {
		t.Fatalf("pooled context retained state: params=%#v locals=%#v body=%#v code=%d", second.Params, second.Locals, second.Body, second.Code)
	}
}

func TestContextStatusCodePredictsResponseStatus(t *testing.T) {
	ctx := acquireContext(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	defer releaseContext(ctx)
	if got, want := ctx.StatusCode(), http.StatusNoContent; got != want {
		t.Fatalf("empty status = %d, want %d", got, want)
	}
	ctx.Text("ok")
	if got, want := ctx.StatusCode(), http.StatusOK; got != want {
		t.Fatalf("body status = %d, want %d", got, want)
	}
	ctx.Status(http.StatusCreated)
	if got, want := ctx.StatusCode(), http.StatusCreated; got != want {
		t.Fatalf("explicit status = %d, want %d", got, want)
	}
}

func TestRouterAuthUsesKindAndCookie(t *testing.T) {
	runtime := testRuntime()
	if err := runtime.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(context.Background())
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	err := server.register(map[string]Router{
		"login": {
			Uri:  "/login",
			Kind: "user",
			Action: func(ctx *Context) {
				ctx.Text(ctx.Auth(Map{"id": "u-1"}))
			},
		},
		"profile": {
			Uri:  "/profile",
			Kind: "user",
			Auth: true,
			Action: func(ctx *Context) {
				ctx.JSON(Map{"id": ctx.Payload()["id"], "kind": ctx.Kind()})
			},
		},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	loginResponse := httptest.NewRecorder()
	server.serveHTTP(loginResponse, httptest.NewRequest(http.MethodGet, "http://example.test/login", nil))
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %q", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != runtime.AuthCookieName() || cookies[0].Value == "" {
		t.Fatalf("auth cookie = %#v", cookies)
	}

	profileRequest := httptest.NewRequest(http.MethodGet, "http://example.test/profile", nil)
	profileRequest.AddCookie(cookies[0])
	profileResponse := httptest.NewRecorder()
	server.serveHTTP(profileResponse, profileRequest)
	if profileResponse.Code != http.StatusOK || !strings.Contains(profileResponse.Body.String(), `"kind":"user"`) {
		t.Fatalf("profile response = %d %q", profileResponse.Code, profileResponse.Body.String())
	}

	adminToken, err := runtime.SignToken(dubi.Token{Auth: true, Kind: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	wrongKindRequest := httptest.NewRequest(http.MethodGet, "http://example.test/profile", nil)
	wrongKindRequest.Header.Set("Authorization", "Bearer "+adminToken)
	wrongKindResponse := httptest.NewRecorder()
	server.serveHTTP(wrongKindResponse, wrongKindRequest)
	if wrongKindResponse.Code != http.StatusForbidden {
		t.Fatalf("wrong kind status = %d, body = %q", wrongKindResponse.Code, wrongKindResponse.Body.String())
	}
}

func TestAssignInputValueNested(t *testing.T) {
	values := Map{}
	assignInputValue(values, "users[0].name", "Ada")
	assignInputValue(values, "users[1].name", "Grace")
	assignInputValue(values, "tags[]", "go")
	assignInputValue(values, "tags[]", "dubi")
	users, ok := values["users"].([]any)
	if !ok || len(users) != 2 || users[0].(Map)["name"] != "Ada" || users[1].(Map)["name"] != "Grace" {
		t.Fatalf("users = %#v", values["users"])
	}
	if want := []any{"go", "dubi"}; !reflect.DeepEqual(values["tags"], want) {
		t.Fatalf("tags = %#v, want %#v", values["tags"], want)
	}
}
