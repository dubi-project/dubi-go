package dubi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestMethodAndServiceInvocation(t *testing.T) {
	rt := testRuntime()
	rt.Register("math.add", Method{Action: func(ctx *Context) (Map, Res) {
		left, _ := ctx.Args["left"].(int)
		right, _ := ctx.Args["right"].(int)
		return Map{"sum": left + right, "request": ctx.Id()}, nil
	}})
	rt.Register("mail.send", Service{Action: func(ctx *Context) Map {
		return Map{"name": ctx.Value["name"]}
	}})

	ctx := context.Background()
	if err := rt.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	meta := NewMeta()
	data := meta.Invoke("math.add", Map{"left": 2, "right": 3})
	if !meta.InvokeOK("math.add", Map{"left": 1, "right": 1}) {
		t.Fatalf("method result = %#v", meta.Result())
	}
	if data["sum"] != 5 {
		t.Fatalf("method data = %#v", data)
	}
	if data["request"] == "" {
		t.Fatal("method did not receive Meta")
	}

	data = meta.Invoke("mail.send", Map{"name": "dubi"})
	if data["name"] != "dubi" {
		t.Fatalf("service data = %#v", data)
	}

	if _, result := rt.Execute(meta, "mail.send", nil); result == nil || result.OK() {
		t.Fatalf("Execute accepted a Service: %#v", result)
	}
}

func TestMethodProfileUsesCoreScope(t *testing.T) {
	rt := testRuntime()
	rt.Register("web", Profile{Include: []string{"core:method:*"}})
	rt.Register("user.get", Method{Action: func(*Context) Map { return Map{"ok": true} }})
	rt.Register("mail.send", Service{Action: func(*Context) Map { return Map{"ok": true} }})

	if err := rt.Run("web"); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	if len(rt.Entries()) != 1 || rt.Entries()[0].Key() != "core:method:user.get" {
		t.Fatalf("profile entries = %#v", rt.Entries())
	}
}

func TestMethodSupportsDynamicErrorReturns(t *testing.T) {
	rt := testRuntime()
	rt.Register("dynamic", Method{Action: func(*Context) (any, error) {
		return map[string]any{"ok": true}, nil
	}})
	rt.Register("broken", Service{Action: func(*Context) (Map, error) {
		return nil, errors.New("service failed")
	}})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	meta := NewMeta()
	data := meta.Invoke("dynamic")
	if data["ok"] != true || meta.Result() == nil || meta.Result().Fail() {
		t.Fatalf("dynamic return = %#v, %v", data, meta.Result())
	}
	if data := meta.Invoke("broken"); data != nil || meta.Result() == nil || meta.Result().OK() || meta.Result().Error() != "service failed" {
		t.Fatalf("error return = %#v, %v", data, meta.Result())
	}
}

func TestMethodMapsArgsAndData(t *testing.T) {
	rt := testRuntime()
	rt.Register("user.get", Method{
		Args: Vars{"id": {Type: "int", Required: true}},
		Data: Vars{"name": {Type: "string"}},
		Action: func(ctx *Context) Map {
			return Map{"name": "user-" + fmt.Sprint(ctx.Args["id"]), "hidden": true}
		},
	})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	data, result := rt.Invoke(nil, "user.get", Map{"id": "7"})
	if result != nil && result.Fail() {
		t.Fatal(result)
	}
	if data["name"] != "user-7" || data["hidden"] != nil {
		t.Fatalf("mapped method data = %#v", data)
	}
}

func TestServiceSupportsListAndBooleanReturns(t *testing.T) {
	rt := testRuntime()
	rt.Register("users.list", Service{Action: func(*Context) []Map {
		return []Map{{"id": "u-1"}, {"id": "u-2"}}
	}})
	rt.Register("feature.enabled", Service{Action: func(*Context) bool { return true }})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	list, result := rt.Invoke(nil, "users.list", nil)
	if result != nil && result.Fail() {
		t.Fatal(result)
	}
	if items, ok := list["items"].([]Map); !ok || len(items) != 2 {
		t.Fatalf("list result = %#v", list)
	}
	flag, result := rt.Invoke(nil, "feature.enabled", nil)
	if result != nil && result.Fail() {
		t.Fatal(result)
	}
	if flag["ok"] != true {
		t.Fatalf("boolean result = %#v", flag)
	}
}
