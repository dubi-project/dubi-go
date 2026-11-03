package web

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

//go:embed testdata/assets
var embeddedAssets embed.FS

func TestEmbeddedStaticFiles(t *testing.T) {
	files, err := fs.Sub(embeddedAssets, "testdata/assets")
	if err != nil {
		t.Fatal(err)
	}
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(files)
	data, err := fs.ReadFile(files, "public/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, pipeline := range []bool{false, true} {
		t.Run(strconv.FormatBool(pipeline), func(t *testing.T) {
			var filters map[string]Filter
			if pipeline {
				filters = map[string]Filter{"response": {Response: func(ctx *Context) {
					ctx.Writer.Header().Set("X-Filtered", "yes")
					ctx.Next()
				}}}
			}
			server := newServerState(ServerConfig{
				Name: defaultServerName, Static: "public",
				Settings: Map{"cache_control": "public, max-age=60"},
			}, map[string]struct{}{defaultServerName: {}})
			if err := server.register(map[string]Router{
				"route": {Uri: "/index.html", Method: http.MethodPost, Action: func(ctx *Context) { ctx.Text("route") }},
			}, filters, nil); err != nil {
				t.Fatal(err)
			}
			request := func(method, target string, headers map[string]string) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(method, "http://example.test"+target, nil)
				for name, value := range headers {
					req.Header.Set(name, value)
				}
				response := httptest.NewRecorder()
				server.serveHTTP(response, req)
				return response
			}
			response := request(http.MethodGet, "/", nil)
			if response.Code != http.StatusOK || response.Body.String() != string(data) {
				t.Fatalf("embedded response = %d %q", response.Code, response.Body.String())
			}
			if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/html") || response.Header().Get("Content-Length") != strconv.Itoa(len(data)) {
				t.Fatalf("embedded headers = %v", response.Header())
			}
			if response.Header().Get("Cache-Control") != "public, max-age=60" || (pipeline && response.Header().Get("X-Filtered") != "yes") {
				t.Fatalf("static filter/cache headers = %v", response.Header())
			}
			etag := response.Header().Get("ETag")
			if etag == "" {
				t.Fatal("embedded response is missing an ETag")
			}
			response = request(http.MethodHead, "/", nil)
			if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Header().Get("Content-Length") != strconv.Itoa(len(data)) {
				t.Fatalf("HEAD response = %d %q %v", response.Code, response.Body.String(), response.Header())
			}
			response = request(http.MethodGet, "/", map[string]string{"Range": "bytes=0-3"})
			if response.Code != http.StatusPartialContent || response.Body.String() != string(data[:4]) {
				t.Fatalf("Range response = %d %q", response.Code, response.Body.String())
			}
			response = request(http.MethodGet, "/", map[string]string{"If-None-Match": etag})
			if response.Code != http.StatusNotModified || response.Body.Len() != 0 {
				t.Fatalf("conditional response = %d %q", response.Code, response.Body.String())
			}
			response = request(http.MethodGet, "/", map[string]string{"Range": "bytes=0-3", "If-Range": "\"outdated\""})
			if response.Code != http.StatusOK || response.Body.String() != string(data) {
				t.Fatalf("stale If-Range response = %d %q", response.Code, response.Body.String())
			}
			response = request(http.MethodPost, "/index.html", nil)
			if response.Code != http.StatusOK || response.Body.String() != "route" {
				t.Fatalf("route response = %d %q", response.Code, response.Body.String())
			}
			response = request(http.MethodGet, "/missing.txt", nil)
			if response.Code != http.StatusNotFound {
				t.Fatalf("missing file status = %d", response.Code)
			}
		})
	}
}

func TestStaticAssetPrecedenceAndDiskFallback(t *testing.T) {
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(fstest.MapFS{"public/index.html": {Data: []byte("embedded")}})
	t.Chdir(t.TempDir())
	if err := os.Mkdir("public", 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"index.html": "disk index", "local.txt": "disk file"} {
		if err := os.WriteFile(filepath.Join("public", name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	server := newServerState(ServerConfig{Name: defaultServerName, Static: "public"}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{"/": "embedded", "/local.txt": "disk file"} {
		response := httptest.NewRecorder()
		server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://example.test"+target, nil))
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("%s response = %d %q, want %q", target, response.Code, response.Body.String(), want)
		}
	}
}

func TestStaticAssetsBySite(t *testing.T) {
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(fstest.MapFS{
		"public/index.html": {Data: []byte("www")},
		"admin/index.html":  {Data: []byte("adm")},
	})
	server := newServerState(ServerConfig{
		Name: defaultServerName, Static: "public",
		Sites: map[string]SiteConfig{
			"www":   {Domains: []string{"www.example.test"}},
			"admin": {Domains: []string{"admin.example.test"}, Static: "admin"},
		},
	}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	etags := make(map[string]bool)
	for site, want := range map[string]string{"www": "www", "admin": "adm"} {
		response := httptest.NewRecorder()
		server.serveHTTP(response, httptest.NewRequest(http.MethodGet, "http://"+site+".example.test/", nil))
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("site %s = %d %q", site, response.Code, response.Body.String())
		}
		etag := response.Header().Get("ETag")
		if etag == "" || etags[etag] {
			t.Fatalf("different same-sized assets must have distinct ETags: %q", etag)
		}
		etags[etag] = true
	}
}
