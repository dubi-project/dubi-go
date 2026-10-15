package web

import (
	"context"
	"net/http/httptest"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestRunLoadingInvokesMethodAndStoresLocals(t *testing.T) {
	rt := testRuntime()
	rt.Register("user.get", dubi.Method{Action: func(ctx *dubi.Context) Map {
		return Map{"id": ctx.Args["id"], "name": "alice"}
	}})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	ctx := &Context{
		Meta:  dubi.NewMeta(),
		Value: Map{"id": "u-1"},
		Args:  Map{"id": "u-1"},
	}
	if !runLoading(ctx, Loading{
		"user": {Invoke: "user.get", Required: true},
	}) {
		t.Fatal("required loader failed")
	}
	if got := ctx.Locals["user"]; got.(Map)["name"] != "alice" {
		t.Fatalf("loaded local = %#v", got)
	}
}

func TestRunLoadingFailsWhenRequiredValueIsMissing(t *testing.T) {
	ctx := &Context{
		Meta:  dubi.NewMeta(),
		Value: Map{},
		Args:  Map{},
	}
	if runLoading(ctx, Loading{
		"user": {Invoke: "user.get", Required: true},
	}) {
		t.Fatal("missing required value should fail")
	}
	if ctx.Code != 400 || ctx.handling != "failed" || ctx.Result() == nil {
		t.Fatalf("loading failure = code=%d handling=%q result=%v", ctx.Code, ctx.handling, ctx.Result())
	}
}

func TestRouterOutcomeOverridesPipelineHandler(t *testing.T) {
	called := false
	ctx := &Context{handling: "denied", Code: 403}
	runRouterOutcome(ctx, Router{
		Denied: func(ctx *Context) {
			called = true
			ctx.Body = "route denied"
		},
	})
	if !called || ctx.handling != "" || ctx.Body != "route denied" {
		t.Fatalf("route outcome = called:%v handling:%q body:%v", called, ctx.handling, ctx.Body)
	}
}

func TestDefaultHTTPOutcomeResults(t *testing.T) {
	tests := []struct {
		name   string
		call   func(*Context)
		status string
		code   int
		text   string
	}{
		{name: "notfound", call: func(ctx *Context) { ctx.NotFound() }, status: "notfound", code: 404, text: "Not Found"},
		{name: "error", call: func(ctx *Context) { ctx.Error() }, status: "error", code: 500, text: "Internal Server Error"},
		{name: "failed", call: func(ctx *Context) { ctx.Failed() }, status: "failed", code: 400, text: "Bad Request"},
		{name: "unsigned", call: func(ctx *Context) { ctx.Unsigned() }, status: "unsigned", code: 401, text: "Signature Required"},
		{name: "unauthed", call: func(ctx *Context) { ctx.Unauthed() }, status: "unauthed", code: 401, text: "Authentication Required"},
		{name: "denied", call: func(ctx *Context) { ctx.Denied() }, status: "denied", code: 403, text: "Forbidden"},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			ctx := &Context{Meta: dubi.NewMeta()}
			item.call(ctx)
			if ctx.Code != item.code || ctx.Result() == nil || ctx.Result().Code() != 1 || ctx.Result().Status() != item.status || ctx.Body != item.text {
				t.Fatalf("outcome = code:%d result:%v body:%q", ctx.Code, ctx.Result(), ctx.Body)
			}
		})
	}
}

func TestNewContextUsesProcessCapabilities(t *testing.T) {
	request := httptest.NewRequest("GET", "http://example.test/", nil)
	ctx := NewContext(httptest.NewRecorder(), request)
	if ctx.Meta == nil {
		t.Fatal("context did not bind the default Runtime")
	}
	if got := ctx.String("notfound"); got != "Not Found" {
		t.Fatalf("context text = %q", got)
	}
	encoded, err := ctx.Marshal("json", Map{"ok": true})
	if err != nil || string(encoded) != `{"ok":true}` {
		t.Fatalf("context codec = %q, %v", encoded, err)
	}
}
