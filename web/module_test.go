package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestPipelineDispatchPreservesRouteRequestValues(t *testing.T) {
	runtime := dubi.New()
	server := newServerRuntime(ServerConfig{Name: defaultServerName}, runtime, map[string]struct{}{defaultServerName: {}})
	err := server.register(
		map[string]Router{
			"user": {
				Uri: "/users/{id}",
				Action: func(ctx *Context) {
					ctx.JSON(Map{
						"id":      ctx.Params["id"],
						"path_id": ctx.Request.PathValue("id"),
						"pattern": ctx.Request.Pattern,
					})
				},
			},
		},
		map[string]Filter{
			"trace": {Request: func(ctx *Context) { ctx.Next() }},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.test/users/u-1", nil)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if got, want := response.Body.String(), `{"id":"u-1","path_id":"u-1","pattern":"/users/{id}"}`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestURLFormParsingPreservesNestedValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.test/", strings.NewReader("user.name=Ada&tags%5B%5D=go&tags%5B%5D=dubi"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	values, files, err := requestFormValues(request, dubi.New())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || values["user"].(Map)["name"] != "Ada" {
		t.Fatalf("form values = %#v, files = %#v", values, files)
	}
	if got, want := values["tags"], []any{"go", "dubi"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
}

func TestPipelineDispatchPreservesServeMuxMethodMismatch(t *testing.T) {
	runtime := dubi.New()
	server := newServerRuntime(ServerConfig{Name: defaultServerName}, runtime, map[string]struct{}{defaultServerName: {}})
	err := server.register(
		map[string]Router{
			"create": {Method: http.MethodPost, Uri: "/items", Action: func(ctx *Context) { ctx.Text("created") }},
		},
		map[string]Filter{
			"trace": {Request: func(ctx *Context) { ctx.Next() }},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.test/items", nil)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if got, want := response.Header().Get("Allow"), http.MethodPost; got != want {
		t.Fatalf("allow = %q, want %q", got, want)
	}
	if request.Method != http.MethodGet {
		t.Fatalf("request method changed during route probe: %q", request.Method)
	}
}

func TestPipelineDispatchPreservesMultiSegmentPathValue(t *testing.T) {
	runtime := dubi.New()
	server := newServerRuntime(ServerConfig{Name: defaultServerName}, runtime, map[string]struct{}{defaultServerName: {}})
	err := server.register(map[string]Router{
		"files": {
			Uri: "/files/{path...}",
			Action: func(ctx *Context) {
				ctx.JSON(Map{
					"param": ctx.Params["path"],
					"value": ctx.Request.PathValue("path"),
				})
			},
		},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "http://example.test/files/a/b%2Fc", nil)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
	if got, want := response.Body.String(), `{"param":"a/b/c","value":"a/b/c"}`; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestServerMaxConcurrencyRejectsOverload(t *testing.T) {
	runtime := dubi.New()
	server := newServerRuntime(ServerConfig{
		Name:           defaultServerName,
		MaxConcurrency: 1,
	}, runtime, map[string]struct{}{defaultServerName: {}})
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	if err := server.register(map[string]Router{
		"slow": {
			Uri: "/",
			Action: func(ctx *Context) {
				entered <- struct{}{}
				<-release
				ctx.Text("ok")
			},
		},
	}, nil, nil); err != nil {
		t.Fatal(err)
	}

	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.serveHTTP(first, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
		close(done)
	}()
	<-entered

	second := httptest.NewRecorder()
	server.serveHTTP(second, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
	if second.Code != http.StatusServiceUnavailable {
		t.Fatalf("overloaded status = %d, want %d", second.Code, http.StatusServiceUnavailable)
	}
	if got := second.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("retry-after = %q, want 1", got)
	}

	close(release)
	<-done
	if first.Code != http.StatusOK || first.Body.String() != "ok" {
		t.Fatalf("first response = %d %q", first.Code, first.Body.String())
	}
}
