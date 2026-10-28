package dubi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestLibraryFoundationIgnoresProfileSelection(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register("minimal", Profile{
		Include: []string{"web:router:*"},
		Exclude: []string{"core:library:*", "core:method:*"},
	})
	rt.Register("mail.sendcloud", &Library{
		Name: "SendCloud", Desc: "Reusable mail methods",
		Settings: Map{"account": "default"},
		Methods: Methods{"send": {Action: func(ctx *Context) Map {
			return Map{"account": ctx.Settings["account"], "subject": ctx.Args["subject"]}
		}}},
	})
	rt.Register("business", Method{Action: func(*Context) Map { return Map{"business": true} }})
	if err := rt.Run("minimal"); err != nil {
		t.Fatal(err)
	}
	entries := rt.Entries()
	if len(entries) != 1 || entries[0].Key() != "core:library:mail.sendcloud" {
		t.Fatalf("foundation library missing from entries: %+v", entries)
	}
	meta := NewMeta()
	lib := meta.Library("mail.sendcloud")
	data := lib.Invoke("send", Map{"subject": "hello"})
	if lib.Result().Fail() || data["account"] != "default" || data["subject"] != "hello" {
		t.Fatalf("library call: %v %v", data, lib.Result())
	}
	if _, result := rt.Invoke(meta, "business", nil); result.OK() {
		t.Fatal("ordinary Method bypassed Profile")
	}
	if data, result := rt.Execute(meta, "mail.sendcloud.send", Map{"subject": "direct"}); result.Fail() || data["subject"] != "direct" {
		t.Fatalf("expanded Method unavailable: %v %v", data, result)
	}
}

func TestLibrarySettingsPrecedenceAndIsolation(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	defaults := Map{"choice": "library", "nested": Map{"value": "original"}, "list": []any{Map{"value": "original"}}}
	methodDefaults := Map{"choice": "method", "method": true}
	rt.Register("provider", Library{Settings: defaults, Methods: Methods{
		"read": {Settings: methodDefaults, Action: func(ctx *Context) Map {
			data := Map{"choice": ctx.Settings["choice"], "method": ctx.Settings["method"], "nested": ctx.Settings["nested"].(Map)["value"], "list": ctx.Settings["list"].([]any)[0].(Map)["value"]}
			ctx.Settings["nested"].(Map)["value"] = "mutated"
			ctx.Settings["list"].([]any)[0].(Map)["value"] = "mutated"
			ctx.Settings["choice"] = "mutated"
			return data
		}},
	}})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	meta := NewMeta()
	overrides := Map{"choice": "caller"}
	lib := meta.Library("provider", overrides, Map{"choice": "last"})
	overrides["choice"] = "changed after binding"
	for i := 0; i < 2; i++ {
		data := lib.Invoke("read")
		want := Map{"choice": "last", "method": true, "nested": "original", "list": "original"}
		if lib.Result().Fail() || !reflect.DeepEqual(data, want) {
			t.Fatalf("settings: %v %v", data, lib.Result())
		}
	}
	if data := meta.Library("provider").Invoke("read"); data["choice"] != "library" {
		t.Fatalf("Library default did not override Method default: %v", data)
	}
	if defaults["nested"].(Map)["value"] != "original" || methodDefaults["choice"] != "method" {
		t.Fatal("request mutated shared definitions")
	}
	// Registration snapshots remain stable even if caller-owned defaults change.
	defaults["choice"] = "changed after setup"
	if data := meta.Library("provider").Invoke("read"); data["choice"] != "library" {
		t.Fatalf("Library shared registration map: %v", data)
	}
}

func TestLibraryReusesMetaMappingAndResult(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	type requestKey struct{}
	meta := NewMeta().WithContext(context.WithValue(context.Background(), requestKey{}, "request-value"))
	meta.Language("zh")
	meta.TraceId("trace-1")
	called := 0
	sentinel := ResultOf(17, "library test failed")
	rt.Register("local", Library{Methods: Methods{
		"get": {
			Args: Vars{"id": {Type: "int", Required: true}},
			Data: Vars{"id": {Type: "int", Required: true}},
			Action: func(ctx *Context) Map {
				called++
				if ctx.Meta != meta || ctx.Id() != meta.Id() || ctx.TraceId() != "trace-1" || ctx.Language() != "zh" || ctx.Context.Value(requestKey{}) != "request-value" {
					t.Error("Library lost request Meta")
				}
				return Map{"id": ctx.Args["id"], "extra": true}
			},
		},
		"fail":  {Action: func(*Context) (Map, Res) { return nil, sentinel }},
		"error": {Action: func(*Context) error { return errors.New("library error") }},
	}})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	lib := meta.Library("local")
	if !lib.Result().OK() {
		t.Fatal("fresh invoker result is not OK")
	}
	if data := lib.Invoke("get", Map{"id": "7"}); lib.Result().Fail() || data["id"] != int64(7) || data["extra"] != nil {
		t.Fatalf("mapped Library result: %v %v", data, lib.Result())
	}
	if data := lib.Invoke("get"); data != nil || lib.Result().OK() || called != 1 {
		t.Fatalf("invalid args reached action: data=%v result=%v called=%d", data, lib.Result(), called)
	}
	if !lib.InvokeFail("fail") || lib.Result() != sentinel || meta.Result() != sentinel {
		t.Fatal("Library and Meta results diverged")
	}
	first, second := lib.Result(), lib.Result()
	if first != sentinel || second != sentinel {
		t.Fatal("reading Result cleared the failure")
	}
	if !lib.InvokeFail("error") || lib.Result().Error() != "library error" {
		t.Fatalf("Go error was not preserved: %v", lib.Result())
	}
	if !lib.InvokeOK("get", Map{"id": "8"}) || !lib.Result().OK() || !meta.Result().OK() {
		t.Fatal("successful call did not replace old failure")
	}
}

func TestLibraryMethodsAreLocalAndNamesAreNormalized(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register(" .MAIL.Provider. ", Library{Methods: Methods{
		" .Send. ": {Action: func(ctx *Context) Map { return Map{"name": ctx.Name} }},
	}})
	serviceCalled := false
	rt.Register("remote.send", Service{Action: func(*Context) Map { serviceCalled = true; return Map{} }})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	meta := NewMeta()
	if data := meta.Library(" .Mail.Provider. ").Invoke(" .SEND. "); data["name"] != "mail.provider.send" {
		t.Fatalf("normalized method name: %v", data)
	}
	for _, prefix := range []string{"remote", "unknown"} {
		lib := meta.Library(prefix)
		if data := lib.Invoke("send"); data != nil || lib.Result().OK() || !strings.Contains(lib.Result().Error(), "library method not found") || meta.Result() != lib.Result() {
			t.Fatalf("missing local method: %v %v", data, lib.Result())
		}
	}
	if serviceCalled {
		t.Fatal("Library dispatched a Service")
	}
}

func TestExplicitMethodCanOverrideLibraryImplementation(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register("group", Library{Settings: Map{"shared": true}, Methods: Methods{"run": {Action: func(*Context) Map { return Map{"version": "original"} }}}})
	rt.Register("group.run", Method{Action: func(ctx *Context) Map { return Map{"version": "override", "shared": ctx.Settings["shared"]} }})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	if data := NewMeta().Library("group").Invoke("run"); data["version"] != "override" || data["shared"] != true {
		t.Fatalf("Method override lost: %v", data)
	}
}

func TestLibraryIsolatedAcrossConcurrentRequests(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register("shared", Library{Settings: Map{"nested": Map{"value": "original"}}, Methods: Methods{
		"read": {Action: func(ctx *Context) Map {
			value := ctx.Settings["nested"].(Map)["value"]
			ctx.Settings["nested"].(Map)["value"] = "changed"
			return Map{"request": ctx.Settings["request"], "value": value}
		}},
	}})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Go(func() {
			lib := NewMeta().Library("shared", Map{"request": i})
			data := lib.Invoke("read")
			if lib.Result().Fail() || data["request"] != i || data["value"] != "original" {
				t.Errorf("request %d: %v %v", i, data, lib.Result())
			}
		})
	}
	group.Wait()
}

func TestResetClearsLibraryDefinitionsAndMethods(t *testing.T) {
	rt := testRuntime()
	rt.Register("old", Library{Methods: Methods{"call": {Action: func(*Context) Map { return Map{} }}}})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	Reset()
	t.Cleanup(Reset)
	if _, ok := core.libraries["old"]; ok {
		t.Fatal("Reset retained Library metadata")
	}
	if _, ok := core.methods["old.call"]; ok {
		t.Fatal("Reset retained expanded Method")
	}
	lib := NewMeta().Library("old")
	if data := lib.Invoke("call"); data != nil || lib.Result().OK() {
		t.Fatal("old Library still callable after Reset")
	}
}

func TestLibraryReplacementAfterFailedStartupRemovesOldMethods(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register("replace", Library{Methods: Methods{"old": {Action: func(*Context) Map { return Map{} }}}})
	testLifecycleModule.startError = errors.New("fail startup once")
	if err := rt.Run(""); err == nil {
		t.Fatal("expected startup failure")
	}
	rt.Register("replace", Library{Methods: Methods{"new": {Action: func(*Context) Map { return Map{"new": true} }}}})
	testLifecycleModule.startError = nil
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	lib := NewMeta().Library("replace")
	if !lib.InvokeFail("old") {
		t.Fatal("old method survived Library replacement")
	}
	if data := lib.Invoke("new"); lib.Result().Fail() || data["new"] != true {
		t.Fatalf("replacement Library failed: %v %v", data, lib.Result())
	}
}
