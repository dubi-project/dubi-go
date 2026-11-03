package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/view"
)

func setupViewTests(t *testing.T, files fstest.MapFS) {
	t.Helper()
	runtime := testRuntime()
	t.Cleanup(dubi.Reset)
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(files)
	dubi.Register("en", dubi.Language{Accepts: []string{"en"}, Strings: Map{"hello": "Hello"}})
	dubi.Register("uppercase", view.Helper{Action: strings.ToUpper})
	if err := runtime.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestViewRendersAfterResponseFilters(t *testing.T) {
	setupViewTests(t, fstest.MapFS{
		"asset/views/page.html": {Data: []byte(`<h1>{% uppercase .model.title %}</h1>|{% .data.message %}|{% .locals.filtered %}|{% .settings.label %}|{% .args.id %}|{% string "hello" %}|{% language %}|{% timezone %}|{% uri "detail" .model.link %}|{% url "detail" .model.link %}|{% (ctx).Name %}`)},
	})
	server := newServerState(ServerConfig{Name: defaultServerName, Settings: Map{"label": "server"}}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{
		"detail": {Uri: "/items/{id}", Settings: Map{"label": "route"}, Args: Vars{"id": {Type: "string", Required: true}}, Action: func(ctx *Context) {
			ctx.Language("en")
			ctx.Timezone(time.UTC)
			ctx.Data["message"] = "before"
			ctx.View("page", Map{"title": "welcome", "link": Map{"{id}": ctx.Args["id"]}}, http.StatusCreated)
		}},
	}, map[string]Filter{"response": {Response: func(ctx *Context) {
		if _, ok := ctx.Body.(viewBody); !ok {
			t.Error("view rendered before Response")
		}
		ctx.Data["message"] = "after"
		ctx.Locals["filtered"] = "yes"
		ctx.Next()
	}}}, nil); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test/items/7", nil))
	want := `<h1>WELCOME</h1>|after|yes|route|7|Hello|en|UTC|/items/7|http://example.test/items/7|detail`
	if response.Code != http.StatusCreated || response.Body.String() != want {
		t.Fatalf("view response = %d %q, want %q", response.Code, response.Body.String(), want)
	}
	if response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q", response.Header().Get("Content-Type"))
	}
}

func TestViewUsesSiteAndRequestLanguage(t *testing.T) {
	setupViewTests(t, fstest.MapFS{
		"asset/views/shop/en/page.html": {Data: []byte(`site English`)},
		"asset/views/shop/page.html":    {Data: []byte(`site default`)},
		"asset/views/page.html":         {Data: []byte(`global`)},
	})
	server := newServerState(ServerConfig{
		Name:  defaultServerName,
		Sites: map[string]SiteConfig{"shop": {Domains: []string{"shop.example.test"}}},
	}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{"shop.page": {Uri: "/", Action: func(ctx *Context) { ctx.View("page") }}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://shop.example.test/", nil)
	request.Header.Set("Accept-Language", "en")
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "site English" {
		t.Fatalf("site view = %d %q", response.Code, response.Body.String())
	}
}

func TestViewRenderFailureUsesErrorHandler(t *testing.T) {
	for _, errorTemplate := range []string{"error", "missing"} {
		t.Run(errorTemplate, func(t *testing.T) {
			setupViewTests(t, fstest.MapFS{
				"asset/views/page.html":  {Data: []byte(`DO NOT WRITE THIS {% render "missing" %}`)},
				"asset/views/error.html": {Data: []byte(`<h1>Rendering failed</h1>`)},
			})
			calls := 0
			server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
			if err := server.register(map[string]Router{"page": {
				Uri: "/", Action: func(ctx *Context) { ctx.View("page") },
				Error: func(ctx *Context) {
					calls++
					if ctx.output.code != 0 {
						t.Error("headers committed before render error")
					}
					ctx.View(errorTemplate)
				},
			}}, nil, nil); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
			want := "<h1>Rendering failed</h1>"
			if errorTemplate == "missing" {
				want = http.StatusText(http.StatusInternalServerError)
			}
			if calls != 1 || response.Code != http.StatusInternalServerError || response.Body.String() != want {
				t.Fatalf("render error = %d %q, handler calls=%d", response.Code, response.Body.String(), calls)
			}
		})
	}
}

func TestViewMayBeReplacedBeforeRendering(t *testing.T) {
	setupViewTests(t, fstest.MapFS{})
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{"page": {Uri: "/", Action: func(ctx *Context) { ctx.View("missing") }}},
		map[string]Filter{"response": {Response: func(ctx *Context) { ctx.JSON(Map{"replaced": true}) }}}, nil); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test/", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"replaced":true}` {
		t.Fatalf("replacement response = %d %q", response.Code, response.Body.String())
	}
}

func TestViewAcceptsModelStatusAndMIME(t *testing.T) {
	ctx := &Context{}
	ctx.View("page", Map{"id": 1}, http.StatusAccepted, "html")
	body, ok := ctx.Body.(viewBody)
	if !ok || body.name != "page" || body.model["id"] != 1 || ctx.Code != http.StatusAccepted {
		t.Fatalf("view selection = %#v", ctx)
	}
}
