package web

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestParseSingleHTTPConfig(t *testing.T) {
	config, err := parseConfig(Map{
		"port":    8080,
		"setting": Map{"gzip": true},
		"cross":   Map{"origins": []any{"https://example.com"}},
		"sites": Map{
			"www": Map{"hosts": []any{"www.example.com"}},
			"api": Map{"hosts": []any{"api.example.com"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Port != 8080 || len(config.Sites) != 2 || config.Setting["gzip"] != true {
		t.Fatalf("config = %#v", config)
	}
	server := defaultServerConfig(config)
	if server.Port != 8080 || !reflect.DeepEqual(server.Sites["www"].Hosts, []string{"www.example.com"}) {
		t.Fatalf("default server = %#v", server)
	}
}

func TestHTTPConfigOnlyStartsWhenRequiredOrRegistered(t *testing.T) {
	instance := &Module{}
	if err := instance.Config(Map{"web": Map{"port": 8080}}); err != nil {
		t.Fatal(err)
	}
	if instance.enabled {
		t.Fatal("plain HTTP config should not enable an empty runtime")
	}
	instance.Register("index", Router{Uri: "/"})
	if !instance.enabled {
		t.Fatal("HTTP router should enable the runtime")
	}
	required := &Module{}
	if err := required.Config(Map{"web": Map{"port": 8080, "require": true}}); err != nil {
		t.Fatal(err)
	}
	if !required.enabled {
		t.Fatal("require=true should enable an empty HTTP runtime")
	}
}

func TestParseMultipleHTTPServers(t *testing.T) {
	config, err := parseConfig(Map{
		"host": "0.0.0.0",
		"servers": Map{
			"a": Map{"port": 8081},
			"b": Map{
				"port": 8082,
				"sites": Map{
					"www": Map{"hosts": []any{"www.example.com"}},
					"api": Map{"hosts": []any{"api.example.com"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Servers) != 2 || config.Servers["a"].Port != 8081 || len(config.Servers["b"].Sites) != 2 {
		t.Fatalf("servers = %#v", config.Servers)
	}
	if err := validateServers(config.Servers); err != nil {
		t.Fatal(err)
	}
}

func TestParseWebProductionOptions(t *testing.T) {
	config, err := parseConfig(Map{
		"read_timeout":        "2s",
		"read_header_timeout": "1500ms",
		"request_timeout":     "3s",
		"shutdown_timeout":    "4s",
		"max_header_bytes":    1024,
		"max_body_bytes":      2048,
		"max_concurrency":     16,
		"compress":            true,
		"trusted_proxies":     []any{"127.0.0.1", "10.0.0.0/8"},
		"tls":                 Map{"cert": "server.crt", "key": "server.key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := defaultServerConfig(config)
	if server.ReadTimeout != 2*time.Second || server.ReadHeaderTimeout != 1500*time.Millisecond || server.RequestTimeout != 3*time.Second || server.ShutdownTimeout != 4*time.Second || server.MaxBodyBytes != 2048 || server.MaxConcurrency != 16 || !server.Compress {
		t.Fatalf("server options = %#v", server)
	}
	if server.TLS.Cert != "server.crt" || len(server.TrustedProxies) != 2 {
		t.Fatalf("server security options = %#v", server)
	}
	if err := validateServers(map[string]ServerConfig{"web": server}); err != nil {
		t.Fatal(err)
	}
}

func TestSingleAndMultipleHTTPFormsCannotMixSites(t *testing.T) {
	if _, err := parseConfig(Map{
		"sites":   Map{"www": Map{}},
		"servers": Map{"api": Map{"port": 8080}},
	}); err == nil {
		t.Fatal("expected mixed single and multi site configuration error")
	}
}

func TestCORSPreflightUsesServerPolicy(t *testing.T) {
	server := newServerState(ServerConfig{
		Name: "default",
		Cross: Map{
			"origins":     []any{"https://app.example.com"},
			"credentials": true,
		},
	}, map[string]struct{}{"default": {}})
	request := httptest.NewRequest(stdhttp.MethodOptions, "http://127.0.0.1/", nil)
	request.Header.Set("Origin", "https://app.example.com")
	request.Header.Set("Access-Control-Request-Headers", "Content-Type")
	writer := httptest.NewRecorder()
	if !server.applyCORS(writer, request, "") {
		t.Fatal("expected preflight to be handled")
	}
	if writer.Code != stdhttp.StatusNoContent || writer.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" || writer.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("cors response = %d %#v", writer.Code, writer.Header())
	}
}

func TestHTTPRuntimeServesStaticFile(t *testing.T) {
	port := freePort(t)
	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("hello static"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "http-static.json")
	config := fmt.Sprintf(`{"web":{"port":%d,"static":%q}}`, port, staticDir)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != stdhttp.StatusOK || string(body) != "hello static" {
		t.Fatalf("static response = %d %q", response.StatusCode, body)
	}
	etag := response.Header.Get("ETag")
	if etag == "" {
		t.Fatal("static response did not include ETag")
	}
	request, err := stdhttp.NewRequest(stdhttp.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("If-None-Match", etag)
	conditional, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	conditional.Body.Close()
	if conditional.StatusCode != stdhttp.StatusNotModified {
		t.Fatalf("conditional static response = %d", conditional.StatusCode)
	}
}

func TestHTTPResponseHelpers(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-response.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	state := dubi.Result(77, "test.created", "created")
	rt := testRuntime()
	rt.Register("html", Router{Uri: "/html", Action: func(ctx *Context) { ctx.HTML("<h1>ok</h1>") }})
	rt.Register("redirect", Router{Uri: "/redirect", Action: func(ctx *Context) { ctx.Redirect("/html", 301) }})
	rt.Register("binary", Router{Uri: "/binary", Action: func(ctx *Context) { ctx.Binary([]byte("bytes"), "bytes.bin") }})
	rt.Register("stream", Router{Uri: "/stream", Action: func(ctx *Context) {
		ctx.Stream(io.NopCloser(strings.NewReader("streamed")), -1)
	}})
	rt.Register("answer", Router{Uri: "/answer", Action: func(ctx *Context) {
		ctx.Answer(state, Map{"ok": true})
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	client := &stdhttp.Client{CheckRedirect: func(*stdhttp.Request, []*stdhttp.Request) error { return stdhttp.ErrUseLastResponse }}
	cases := []struct {
		path, want, contentType string
		code                    int
	}{
		{path: "/html", want: "<h1>ok</h1>", contentType: "text/html; charset=utf-8", code: 200},
		{path: "/redirect", want: "", contentType: "", code: 301},
		{path: "/binary", want: "bytes", contentType: "application/octet-stream", code: 200},
		{path: "/stream", want: "streamed", contentType: "application/octet-stream", code: 200},
	}
	for _, item := range cases {
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, item.path))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != item.code || (item.want != "" && string(body) != item.want) {
			t.Fatalf("%s response = %d %q", item.path, response.StatusCode, body)
		}
		if item.contentType != "" && response.Header.Get("Content-Type") != item.contentType {
			t.Fatalf("%s content type = %q", item.path, response.Header.Get("Content-Type"))
		}
	}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/answer", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != 200 || !strings.Contains(string(body), `"code":77`) || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("answer response = %d %q", response.StatusCode, body)
	}
}

func TestHTTPRuntimeServesRouterAction(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("index", Router{Uri: "/", Action: func(ctx *Context) {
		ctx.JSON(Map{"ok": true})
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	var response *stdhttp.Response
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestHTTPMethodMismatchReturns405(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-method.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("create", Router{Method: stdhttp.MethodPost, Uri: "/items", Action: func(ctx *Context) { ctx.Text("created") }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/items", port))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusMethodNotAllowed || response.Header.Get("Allow") != stdhttp.MethodPost {
		t.Fatalf("method mismatch = %d allow=%q", response.StatusCode, response.Header.Get("Allow"))
	}
}

func TestHTTPRequestBodyLimitReturns413(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-body-limit.json")
	data := fmt.Sprintf(`{"web":{"port":%d,"max_body_bytes":4}}`, port)
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("body", Router{Method: stdhttp.MethodPost, Uri: "/body", Action: func(ctx *Context) { ctx.Text("ok") }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	request, err := stdhttp.NewRequest(stdhttp.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/body", port), strings.NewReader(`{"value":"too-large"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != stdhttp.StatusRequestEntityTooLarge {
		t.Fatalf("body limit status = %d", response.StatusCode)
	}
}

func TestHTTPPanicRecoveryAddsRequestID(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-panic.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("panic", Router{Uri: "/panic", Action: func(*Context) { panic("boom") }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/panic", port))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != stdhttp.StatusInternalServerError || response.Header.Get("X-Request-ID") == "" {
		t.Fatalf("panic response = %d request-id=%q", response.StatusCode, response.Header.Get("X-Request-ID"))
	}
}

func TestHTTPMultipartUploadsAndNestedValues(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-upload.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("upload", Router{Method: stdhttp.MethodPost, Uri: "/upload", Action: func(ctx *Context) {
		user, _ := ctx.Form["user"].(Map)
		file, ok := ctx.Upload("avatar")
		if !ok {
			ctx.Failed()
			return
		}
		ctx.JSON(Map{"name": user["name"], "file": file.Filename})
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("user.name", "alice")
	part, err := form.CreateFormFile("avatar", "avatar.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("hello"))
	_ = form.Close()
	request, err := stdhttp.NewRequest(stdhttp.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/upload", port), &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got Map
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusOK || got["name"] != "alice" || got["file"] != "avatar.txt" {
		t.Fatalf("upload response = %d %#v", response.StatusCode, got)
	}
}

func TestHTTPTrustedProxyAndCompression(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-proxy.json")
	data := fmt.Sprintf(`{"web":{"port":%d,"compress":true,"trusted_proxies":["127.0.0.1"]}}`, port)
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("proxy", Router{Uri: "/proxy", Action: func(ctx *Context) {
		ctx.Text("a long response that should be compressed when the client supports gzip: " + ctx.ClientIP())
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	request, err := stdhttp.NewRequest(stdhttp.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/proxy", port), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	request.Header.Set("X-Forwarded-For", "203.0.113.9")
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("content encoding = %q", response.Header.Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded), "203.0.113.9") {
		t.Fatalf("proxy response = %q", decoded)
	}
}

func TestHTTPContextParsesPathAndQueryValues(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-params.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("user", Router{Uri: "/users/{id}", Action: func(ctx *Context) {
		ctx.JSON(Map{"id": ctx.Params["id"], "page": ctx.Query["page"]})
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/users/u-1?page=2", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != stdhttp.StatusOK || string(body) != `{"id":"u-1","page":"2"}` {
		t.Fatalf("params response = %d %q", response.StatusCode, body)
	}
}

func TestHTTPContextBuildsRouteURLs(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-url.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("user.show", Router{Uri: "/users/{id}", Action: func(ctx *Context) {
		ctx.JSON(Map{
			"uri": ctx.RouteUri("user.show", Map{"{id}": "u-1", "page": 2}),
			"url": ctx.RouteUrl("user.show", Map{"{id}": "u-1", "page": 2}),
		})
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/users/u-1", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var values Map
	if err := json.Unmarshal(body, &values); err != nil {
		t.Fatal(err)
	}
	if values["uri"] != "/users/u-1?page=2" || values["url"] != fmt.Sprintf("http://127.0.0.1:%d/users/u-1?page=2", port) {
		t.Fatalf("route urls = %#v", values)
	}
}

func TestHTTPRouterMapsVarsAndJSONBody(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-body.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("create", Router{
		Uri: "/users",
		Args: Vars{
			"name": {Type: "string", Required: true},
			"age":  {Type: "int", Required: true},
		},
		Action: func(ctx *Context) {
			ctx.JSON(Map{"name": ctx.Args["name"], "age": ctx.Args["age"]})
		},
	})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	request, err := stdhttp.NewRequest(stdhttp.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/users", port), bytes.NewBufferString(`{"name":"alice","age":"42"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var decoded Map
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusOK || decoded["name"] != "alice" || decoded["age"] != float64(42) {
		t.Fatalf("mapped body = %d %q", response.StatusCode, body)
	}
}

func TestHTTPRouteLoadingAndAnswerData(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-loading-answer.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("user.get", dubi.Method{Action: func(ctx *dubi.Context) Map {
		return Map{"id": ctx.Args["id"], "name": "alice"}
	}})
	rt.Register("user.show", Router{
		Uri: "/users/{id}",
		Loading: Loading{
			"user": {Invoke: "user.get", Required: true},
		},
		Data: Vars{
			"id":   {Type: "string", Required: true},
			"name": {Type: "string", Required: true},
		},
		Action: func(ctx *Context) {
			ctx.Answer(dubi.OK, ctx.Locals["user"].(Map))
		},
	})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	var response *stdhttp.Response
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/users/u-1", port))
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var decoded Map
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	data, ok := decoded["data"].(map[string]any)
	if response.StatusCode != stdhttp.StatusOK || !ok || data["id"] != "u-1" || data["name"] != "alice" {
		t.Fatalf("loaded answer = %d %#v", response.StatusCode, decoded)
	}
	if decoded["code"] != float64(0) || decoded["text"] != "成功" {
		t.Fatalf("answer result = %#v", decoded)
	}
}

func TestHTTPFailuresUseFailedResultPipeline(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-failures.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	failed := Handler{Failed: func(ctx *Context) {
		ctx.Answer(ctx.Result())
	}}
	rt.Register("errors", failed)
	rt.Register("required", Router{
		Uri:  "/required",
		Args: Vars{"name": {Type: "string", Required: true}},
		Action: func(ctx *Context) {
			ctx.Text("unexpected")
		},
	})
	rt.Register("json", Router{
		Uri:  "/json",
		Args: Vars{"name": {Type: "string", Required: true}},
		Action: func(ctx *Context) {
			ctx.Text("unexpected")
		},
	})
	rt.Register("loading", Router{
		Uri: "/loading",
		Loading: Loading{
			"user": {Invoke: "missing.user", Required: true},
		},
		Action: func(ctx *Context) {
			ctx.Text("unexpected")
		},
	})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	tests := []struct {
		name       string
		request    func() *stdhttp.Request
		status     int
		resultCode float64
		resultText string
	}{
		{
			name: "missing argument",
			request: func() *stdhttp.Request {
				request, _ := stdhttp.NewRequest(stdhttp.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/required", port), nil)
				return request
			},
			status:     stdhttp.StatusBadRequest,
			resultCode: 400,
			resultText: "missing argument: name",
		},
		{
			name: "invalid json",
			request: func() *stdhttp.Request {
				request, _ := stdhttp.NewRequest(stdhttp.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/json", port), strings.NewReader("{"))
				request.Header.Set("Content-Type", "application/json")
				return request
			},
			status:     stdhttp.StatusBadRequest,
			resultCode: 1,
		},
		{
			name: "required loading",
			request: func() *stdhttp.Request {
				request, _ := stdhttp.NewRequest(stdhttp.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/loading?id=u-1", port), nil)
				return request
			},
			status:     stdhttp.StatusBadRequest,
			resultCode: 1,
			resultText: "resource could not be loaded",
		},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			response, err := stdhttp.DefaultClient.Do(item.request())
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			var decoded Map
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("response = %d %q: %v", response.StatusCode, body, err)
			}
			if response.StatusCode != item.status || decoded["code"] != item.resultCode {
				t.Fatalf("failure response = %d %#v", response.StatusCode, decoded)
			}
			if item.resultText != "" && decoded["text"] != item.resultText {
				t.Fatalf("failure text = %#v, want %q", decoded["text"], item.resultText)
			}
		})
	}
}

func TestHTTPNotFoundCarriesResult(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-notfound.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("errors", Handler{NotFound: func(ctx *Context) {
		ctx.Answer(ctx.Result())
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/missing", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	var decoded Map
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusNotFound || decoded["code"] != float64(1) || decoded["text"] != "Not Found" {
		t.Fatalf("not found response = %d %#v", response.StatusCode, decoded)
	}
}

func TestHTTPRuntimeSelectsSiteByHost(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-sites.json")
	data := fmt.Sprintf(`{"web":{"port":%d,"sites":{"www":{"hosts":["www.example.test"]},"api":{"hosts":["api.example.test"]}}}}`, port)
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("www.index", Router{Uri: "/", Action: func(ctx *Context) { ctx.Text("www") }})
	rt.Register("api.index", Router{Uri: "/", Action: func(ctx *Context) { ctx.Text("api") }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	for host, want := range map[string]string{"www.example.test": "www", "api.example.test": "api"} {
		request := httptest.NewRequest(stdhttp.MethodGet, "/", nil)
		request.URL.Scheme = "http"
		request.URL.Host = fmt.Sprintf("127.0.0.1:%d", port)
		request.RequestURI = ""
		request.Host = host
		response, err := stdhttp.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != stdhttp.StatusOK {
			t.Fatalf("host %q status = %d", host, response.StatusCode)
		}
		if string(body) != want {
			t.Fatalf("host %q body = %q, want %q", host, body, want)
		}
	}
	request := httptest.NewRequest(stdhttp.MethodGet, "/", nil)
	request.URL.Scheme = "http"
	request.URL.Host = fmt.Sprintf("127.0.0.1:%d", port)
	request.RequestURI = ""
	request.Host = "abc.example.test"
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != stdhttp.StatusNotFound {
		t.Fatalf("unmatched host status = %d, want 404", response.StatusCode)
	}
}

func TestHTTPContextMergesWebSiteAndRouterSettings(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "web-settings.json")
	data := fmt.Sprintf(`{"web":{"port":%d,"setting":{"web":true,"shared":"web"},"sites":{"www":{"hosts":["www.settings.test"],"setting":{"site":true,"shared":"site"}}}}}`, port)
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("www.index", Router{
		Uri:     "/",
		Setting: Map{"router": true, "shared": "router"},
		Action: func(ctx *Context) {
			ctx.JSON(ctx.Setting)
		},
	})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	request := httptest.NewRequest(stdhttp.MethodGet, "/", nil)
	request.URL.Scheme = "http"
	request.URL.Host = fmt.Sprintf("127.0.0.1:%d", port)
	request.RequestURI = ""
	request.Host = "www.settings.test"
	response, err := stdhttp.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var got Map
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != stdhttp.StatusOK || got["web"] != true || got["site"] != true || got["router"] != true || got["shared"] != "router" {
		t.Fatalf("settings response = %d %#v", response.StatusCode, got)
	}
}

func TestHTTPRuntimeServesMultipleServers(t *testing.T) {
	firstPort := freePort(t)
	secondPort := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-servers.json")
	data := fmt.Sprintf(`{"web":{"servers":{"a":{"port":%d},"b":{"port":%d}}}}`, firstPort, secondPort)
	if err := os.WriteFile(configPath, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	rt := testRuntime()
	rt.Register("a.index", Router{Uri: "/", Action: func(ctx *Context) { ctx.Text("a") }})
	rt.Register("b.index", Router{Uri: "/", Action: func(ctx *Context) { ctx.Text("b") }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	for port, want := range map[int]string{firstPort: "a", secondPort: "b"} {
		response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != stdhttp.StatusOK || string(body) != want {
			t.Fatalf("port %d response = %d %q, want 200 %q", port, response.StatusCode, body, want)
		}
	}
}

func TestHTTPFilterAndHandlerChains(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "http-chain.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	steps := make([]string, 0, 4)
	actionSteps := make([]string, 0, 3)
	rt := testRuntime()
	rt.Register("access", Filter{
		Serve: func(ctx *Context) {
			steps = append(steps, "before")
			ctx.Next()
			steps = append(steps, "after")
		},
	})
	rt.Register("errors", Handler{
		Error: func(ctx *Context) {
			ctx.JSON(Map{"handled": true}, 500)
		},
		Unsigned: func(ctx *Context) {
			ctx.JSON(Map{"signed": false}, 401)
		},
	})
	rt.Register("ok", Router{Uri: "/ok", Action: func(ctx *Context) {
		steps = append(steps, "action")
		ctx.Text("ok")
	}})
	rt.Register("fail", Router{Uri: "/fail", Action: func(ctx *Context) {
		ctx.Error(dubi.ErrorResult(errors.New("failed")))
	}})
	rt.Register("unsigned", Router{Uri: "/unsigned", Action: func(ctx *Context) {
		ctx.Unsigned(dubi.ErrorResult(errors.New("signature required")))
	}})
	rt.Register("actions", Router{
		Uri: "/actions",
		Actions: []func(*Context){func(ctx *Context) {
			actionSteps = append(actionSteps, "before")
			ctx.Next()
			actionSteps = append(actionSteps, "after")
		}},
		Action: func(ctx *Context) {
			actionSteps = append(actionSteps, "action")
			ctx.Text("actions")
		},
	})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	response, err := stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/ok", port))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !reflect.DeepEqual(steps, []string{"before", "action", "after"}) {
		t.Fatalf("filter chain = %v", steps)
	}
	response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/actions", port))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if !reflect.DeepEqual(actionSteps, []string{"before", "action", "after"}) {
		t.Fatalf("router action chain = %v", actionSteps)
	}
	response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/fail", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != stdhttp.StatusInternalServerError || string(body) != `{"handled":true}` {
		t.Fatalf("handled error = %d %q", response.StatusCode, body)
	}
	response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/unsigned", port))
	if err != nil {
		t.Fatal(err)
	}
	body, readErr = io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if response.StatusCode != stdhttp.StatusUnauthorized || string(body) != `{"signed":false}` {
		t.Fatalf("handled unsigned = %d %q", response.StatusCode, body)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
