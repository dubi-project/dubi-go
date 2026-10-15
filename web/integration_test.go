package web

import (
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

// TestBusinessProfileFlow exercises the path an application normally uses:
// registrations are selected by Profile, a Web route loads a Method, and the
// response is produced through Dubi's Result/Answer format.
func TestBusinessProfileFlow(t *testing.T) {
	port := freePort(t)
	configPath := filepath.Join(t.TempDir(), "business-flow.json")
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf(`{"web":{"port":%d}}`, port)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)

	rt := testRuntime()
	rt.Register("api", dubi.Profile{Include: []string{
		"core:method:*",
		"web:router:*",
	}})
	rt.Register("user.get", dubi.Method{
		Args: Vars{"id": {Type: "string", Required: true}},
		Action: func(ctx *dubi.Context) Map {
			return Map{"id": ctx.Args["id"], "name": "Ada"}
		},
	})
	rt.Register("api.user", Router{
		Uri: "/users/{id}",
		Loading: Loading{
			"user": {Invoke: "user.get", Value: "id", Required: true},
		},
		Data: Vars{
			"id":   {Type: "string", Required: true},
			"name": {Type: "string", Required: true},
		},
		Action: func(ctx *Context) {
			ctx.Answer(dubi.OK, ctx.Locals["user"].(Map))
		},
	})
	if err := rt.Run("api"); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	var response *stdhttp.Response
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		response, err = stdhttp.Get(fmt.Sprintf("http://127.0.0.1:%d/users/u-7", port))
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
	if response.StatusCode != stdhttp.StatusOK || !ok || data["id"] != "u-7" || data["name"] != "Ada" {
		t.Fatalf("business response = %d %#v", response.StatusCode, decoded)
	}
	entries := rt.Entries()
	if len(entries) != 2 || entries[0].Name != "api.user" || entries[1].Name != "user.get" {
		t.Fatalf("profile entries = %#v", entries)
	}
}

func TestRoutePrecedenceAndConcurrentDispatch(t *testing.T) {
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	var requests int
	var requestMu sync.Mutex
	if err := server.register(map[string]Router{
		"user": {
			Uri: "/users/{id}",
			Action: func(ctx *Context) {
				requestMu.Lock()
				requests++
				requestMu.Unlock()
				ctx.Text("user:" + fmt.Sprint(ctx.Params["id"]))
			},
		},
		"settings": {
			Uri:    "/users/settings",
			Action: func(ctx *Context) { ctx.Text("settings") },
		},
	}, nil, nil); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(stdhttp.MethodGet, "http://example.test/users/settings", nil)
	response := httptest.NewRecorder()
	server.serveHTTP(response, request)
	if response.Code != stdhttp.StatusOK || response.Body.String() != "settings" {
		t.Fatalf("specific route = %d %q", response.Code, response.Body.String())
	}

	const parallel = 64
	errors := make(chan string, parallel)
	var group sync.WaitGroup
	group.Add(parallel)
	for index := 0; index < parallel; index++ {
		go func(index int) {
			defer group.Done()
			request := httptest.NewRequest(stdhttp.MethodGet, fmt.Sprintf("http://example.test/users/u-%d", index), nil)
			response := httptest.NewRecorder()
			server.serveHTTP(response, request)
			if response.Code != stdhttp.StatusOK || response.Body.String() != fmt.Sprintf("user:u-%d", index) {
				errors <- fmt.Sprintf("request %d = %d %q", index, response.Code, response.Body.String())
			}
		}(index)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if requests != parallel {
		t.Fatalf("handled requests = %d, want %d", requests, parallel)
	}
}
