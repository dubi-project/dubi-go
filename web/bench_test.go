package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

type benchmarkWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *benchmarkWriter) Header() http.Header { return w.header }

func (w *benchmarkWriter) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}

func (w *benchmarkWriter) Write(value []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.body.Write(value)
}

func (w *benchmarkWriter) reset() {
	clear(w.header)
	w.body.Reset()
	w.code = 0
}

func BenchmarkQueryValuesNested(b *testing.B) {
	query := url.Values{
		"user.name":   {"Ada"},
		"users[0].id": {"1"},
		"users[1].id": {"2"},
		"tags[]":      {"go", "dubi"},
	}
	request := &http.Request{URL: &url.URL{RawQuery: query.Encode()}}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = queryValues(request)
	}
}

func BenchmarkMappingPrepared(b *testing.B) {
	runtime := testRuntime()
	definition := Vars{
		"user": {Type: "object", Children: Vars{
			"name": {Type: "string", Required: true},
			"age":  {Type: "int"},
		}},
	}
	plan := runtime.PrepareMapping(definition)
	data := Map{"user": Map{"name": "Ada", "age": "36"}}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_, _ = runtime.ApplyMapping(plan, data)
	}
}

func BenchmarkRequestFormValuesJSON(b *testing.B) {
	payload := []byte(`{"user":{"name":"Ada","age":36}}`)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		request, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/json")
		_, _, _ = requestFormValues(request)
	}
}

func BenchmarkRequestFormValuesForm(b *testing.B) {
	payload := []byte("user.name=Ada&users%5B0%5D.id=1&tags%5B%5D=go&tags%5B%5D=dubi")
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		request, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(payload))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		_, _, _ = requestFormValues(request)
	}
}

func BenchmarkHTTPRouteEndToEnd(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	err := server.register(map[string]Router{
		"index": {Uri: "/", Action: func(ctx *Context) { ctx.JSON(Map{"ok": true}) }},
	}, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		server.serveHTTP(response, request)
		if response.Code != http.StatusOK {
			b.Fatalf("status = %d", response.Code)
		}
	}
}

func BenchmarkHTTPRouteFrameworkOnly(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{
		"index": {Uri: "/", Action: func(ctx *Context) { ctx.JSON(Map{"ok": true}) }},
	}, nil, nil); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	writer := &benchmarkWriter{header: make(http.Header)}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		writer.reset()
		server.serveHTTP(writer, request)
		if writer.code != http.StatusOK {
			b.Fatalf("status = %d", writer.code)
		}
	}
}

func BenchmarkHTTPRoutePipeline(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(
		map[string]Router{
			"index": {Uri: "/", Action: func(ctx *Context) { ctx.JSON(Map{"ok": true}) }},
		},
		map[string]Filter{
			"trace": {Request: func(ctx *Context) { ctx.Next() }},
		},
		nil,
	); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	writer := &benchmarkWriter{header: make(http.Header)}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		writer.reset()
		server.serveHTTP(writer, request)
		if writer.code != http.StatusOK {
			b.Fatalf("status = %d", writer.code)
		}
	}
}

func BenchmarkHTTPRouteParallel(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{
		"user": {
			Uri:    "/users/{id}",
			Action: func(ctx *Context) { ctx.Status(http.StatusNoContent) },
		},
	}, nil, nil); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			request := httptest.NewRequest(http.MethodGet, "http://example.test/users/u-1", nil)
			response := httptest.NewRecorder()
			server.serveHTTP(response, request)
			if response.Code != http.StatusNoContent {
				b.Fatalf("status = %d", response.Code)
			}
		}
	})
}

func BenchmarkHTTPRoutePathParameter(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{
		"user": {Uri: "/users/{id}", Action: func(ctx *Context) { ctx.Status(http.StatusNoContent) }},
	}, nil, nil); err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "http://example.test/users/u-1", nil)
	writer := &benchmarkWriter{header: make(http.Header)}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		writer.reset()
		server.serveHTTP(writer, request)
		if writer.code != http.StatusNoContent {
			b.Fatalf("status = %d", writer.code)
		}
	}
}

func BenchmarkHTTPNotFoundEndToEnd(b *testing.B) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	request := httptest.NewRequest(http.MethodGet, "http://example.test/missing", nil)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		response := httptest.NewRecorder()
		server.serveHTTP(response, request)
		if response.Code != http.StatusNotFound {
			b.Fatalf("status = %d", response.Code)
		}
	}
}
