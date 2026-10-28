package web

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestAnswerOptionalTextAndData(t *testing.T) {
	for _, test := range []struct {
		name     string
		action   func(*Context)
		response func(*Context)
		code     float64
		text     string
		data     bool
	}{
		{name: "nil result and nil data", action: func(ctx *Context) { ctx.Answer(nil, nil) }},
		{name: "nil result and empty data", action: func(ctx *Context) { ctx.Answer(nil, Map{}) }},
		{name: "nil result and implicit data", action: func(ctx *Context) { ctx.Answer(nil) }},
		{name: "nil result with data", action: func(ctx *Context) { ctx.Answer(nil, Map{"id": 1}) }, data: true},
		{name: "explicit success", action: func(ctx *Context) { ctx.Answer(dubi.OK, Map{}) }, text: "成功"},
		{name: "explicit failure", action: func(ctx *Context) { ctx.Answer(dubi.ResultOf(7, "invalid"), Map{}) }, code: 7, text: "invalid"},
		{name: "empty status", action: func(ctx *Context) { ctx.Answer(dubi.ResultOf(0, "")) }},
		{name: "implicit ctx data", action: func(ctx *Context) { ctx.Data = Map{"id": 1}; ctx.Answer(nil) }, data: true},
		{name: "explicit empty clears data", action: func(ctx *Context) { ctx.Data = Map{"id": 1}; ctx.Answer(nil, Map{}) }},
		{name: "explicit nil clears data", action: func(ctx *Context) { ctx.Data = Map{"id": 1}; ctx.Answer(nil, nil) }},
		{name: "response filter adds data", action: func(ctx *Context) { ctx.Answer(nil, Map{}) }, response: func(ctx *Context) { ctx.Data = Map{"id": 1} }, data: true},
		{name: "response filter clears data", action: func(ctx *Context) { ctx.Answer(nil, Map{"id": 1}) }, response: func(ctx *Context) { clear(ctx.Data) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
			if err := server.register(map[string]Router{"answer": {Uri: "/", Action: test.action}}, map[string]Filter{"answer": {Response: test.response}}, nil); err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			server.serveHTTP(response, httptest.NewRequest(stdhttp.MethodGet, "http://example.test/", nil))
			var body Map
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != stdhttp.StatusOK || body["code"] != test.code || body["time"] == nil {
				t.Fatalf("invalid answer: status=%d body=%v", response.Code, body)
			}
			text, hasText := body["text"]
			if hasText != (test.text != "") || (hasText && text != test.text) {
				t.Fatalf("text = %v (present=%v), want %q: %v", text, hasText, test.text, body)
			}
			data, hasData := body["data"]
			if hasData != test.data {
				t.Fatalf("data present=%v, want %v: %v", hasData, test.data, body)
			}
			if hasData && data.(map[string]any)["id"] != float64(1) {
				t.Fatalf("data = %v", data)
			}
		})
	}
}
