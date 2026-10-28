package dubi

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

type triggerProbeModule struct {
	setup, open, start, stop, close func(*Context) error
}

func (m *triggerProbeModule) Reset()             { *m = triggerProbeModule{} }
func (*triggerProbeModule) Register(string, any) {}
func (*triggerProbeModule) Config(Map) error     { return nil }
func (m *triggerProbeModule) Setup(ctx *Context) error {
	if m.setup != nil {
		return m.setup(ctx)
	}
	return nil
}
func (m *triggerProbeModule) Open(ctx *Context) error {
	if m.open != nil {
		return m.open(ctx)
	}
	return nil
}
func (m *triggerProbeModule) Start(ctx *Context) error {
	if m.start != nil {
		return m.start(ctx)
	}
	return nil
}
func (m *triggerProbeModule) Stop(ctx *Context) error {
	if m.stop != nil {
		return m.stop(ctx)
	}
	return nil
}
func (m *triggerProbeModule) Close(ctx *Context) error {
	if m.close != nil {
		return m.close(ctx)
	}
	return nil
}

var triggerProbe = &triggerProbeModule{}
var triggerHost Host

func init() { triggerHost = Mount(triggerProbe) }

func awaitTrigger[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for trigger")
		var zero T
		return zero
	}
}

func TestRegistryAppendsTriggersAndKeepsOrdinaryReplacement(t *testing.T) {
	r := NewRegistry()
	r.Register("shared", Method{Name: "old"})
	r.Register("shared", Trigger{Name: "first"})
	r.Register("shared", &Trigger{Name: "second"})
	r.Register("shared", Method{Name: "new"})
	entries := r.Entries()
	if len(entries) != 3 || entries[0].Value.(Method).Name != "new" || entries[1].Value.(Trigger).Name != "first" || entries[2].Value.(*Trigger).Name != "second" {
		t.Fatalf("entries: %+v", entries)
	}
	if value, ok := r.Lookup("shared"); !ok || value.(Method).Name != "new" {
		t.Fatalf("lookup: %v", value)
	}
	r.Register("only.trigger", Trigger{Name: "one"})
	r.Register("only.trigger", Trigger{Name: "two"})
	if value, ok := r.Lookup("only.trigger"); !ok || value.(Trigger).Name != "two" {
		t.Fatalf("trigger lookup: %v", value)
	}
	for _, entry := range entries[1:] {
		if entry.Name != "shared" || entry.Key() != "core:trigger:shared" {
			t.Fatalf("profile key: %+v", entry)
		}
	}
	r.Freeze()
	defer func() {
		if recover() == nil {
			t.Error("late trigger registration accepted")
		}
	}()
	r.Register("shared", Trigger{})
}

func TestTriggerProfileSelectionAndInternalNames(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	var order []string
	Scope("data").Register("test", Trigger{Action: func(ctx *Context) {
		order = append(order, "first")
		if ctx.Name != "data.test" || ctx.Config.(Trigger).Name != "data.test" {
			t.Error("internal name leaked to callback")
		}
	}})
	Register("data.test", &Trigger{Action: func(*Context) { order = append(order, "second") }})
	Register("other", Trigger{Action: func(*Context) { t.Error("excluded trigger ran") }})
	Register("data.test", Method{Action: func(*Context) Map { return Map{"ordinary": true} }})
	Register("_.trigger.data.test.1", Method{Action: func(*Context) Map { return Map{"reserved": true} }})
	Register("_.trigger.data.test.2", Service{Action: func(*Context) Map { return Map{"reserved.service": true} }})
	Register("selected", Profile{Include: []string{"core:trigger:data.*", "core:method:*", "core:service:*"}})
	if err := rt.Run("selected"); err != nil {
		t.Fatal(err)
	}
	SyncToggle("data.test")
	SyncToggle("other")
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Fatalf("order: %v", order)
	}
	if len(core.triggerMethods) != 2 || core.triggerMethods[0] == "_.trigger.data.test.1" || core.triggerMethods[0] == "_.trigger.data.test.2" {
		t.Fatalf("internal names collided: %v", core.triggerMethods)
	}
	count := 0
	for _, entry := range rt.Entries() {
		if entry.Scope == "core:trigger" {
			count++
			if entry.Key() != "core:trigger:data.test" {
				t.Fatalf("logical key: %v", entry)
			}
		}
		if strings.HasPrefix(entry.Name, "_.trigger.") && entry.Name != "_.trigger.data.test.1" && entry.Name != "_.trigger.data.test.2" {
			t.Fatal("generated method entered Registry")
		}
	}
	if count != 2 {
		t.Fatalf("Runtime dropped listeners: %v", rt.Entries())
	}
	if data, res := rt.Execute(nil, "data.test", nil); res.Fail() || data["ordinary"] != true {
		t.Fatalf("ordinary method overwritten: %v %v", data, res)
	}
	if data, res := rt.Execute(nil, "_.trigger.data.test.1", nil); res.Fail() || data["reserved"] != true {
		t.Fatalf("reserved method overwritten: %v %v", data, res)
	}
	if data, res := rt.Invoke(nil, "_.trigger.data.test.2", nil); res.Fail() || data["reserved.service"] != true {
		t.Fatalf("reserved service overwritten: %v %v", data, res)
	}
}

func TestHostAndLifecycleTriggers(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	var started atomic.Bool
	var stopped atomic.Bool
	startSeen := make(chan struct{}, 1)
	var phases []string
	for _, phase := range []string{"setup", "open", "start", "stop"} {
		Register("probe."+phase, Trigger{Action: func(ctx *Context) { phases = append(phases, ctx.Name) }})
	}
	triggerProbe.setup = func(*Context) error { triggerHost.SyncToggle("probe.setup"); return nil }
	triggerProbe.open = func(*Context) error { triggerHost.SyncToggle("probe.open"); return nil }
	triggerProbe.start = func(*Context) error { triggerHost.SyncToggle("probe.start"); started.Store(true); return nil }
	triggerProbe.stop = func(*Context) error { triggerHost.SyncToggle("probe.stop"); stopped.Store(true); return nil }
	Register(START, Trigger{Action: func(*Context) {
		if !started.Load() {
			t.Error("START ran before module Start")
		}
		startSeen <- struct{}{}
	}})
	Register(STOP, Trigger{Action: func(*Context) {
		if stopped.Load() {
			t.Error("STOP ran after module Stop")
		}
		triggerHost.SyncToggle("probe.start")
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	awaitTrigger(t, startSeen)
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	want := []string{"probe.setup", "probe.open", "probe.start", "probe.start", "probe.stop"}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("host phases: %v", phases)
	}
	SyncToggle("probe.start") // Closed runtimes reject new notifications.
	if len(phases) != len(want) {
		t.Fatal("closed runtime accepted trigger")
	}
}

func TestToggleIsConcurrentAndSnapshotsPayloads(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	entered := make(chan string, 2)
	release := make(chan struct{})
	finished := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		Register("work", Trigger{Settings: Map{"nested": Map{"value": "default"}}, Action: func(ctx *Context) {
			entered <- ctx.Id()
			<-release
			if ctx.Args["nested"].(Map)["value"] != "original" || ctx.Args["bytes"].([]byte)[0] != 1 || ctx.Settings["nested"].(Map)["value"] != "default" {
				t.Error("payload snapshot shared mutable state")
			}
			ctx.Args["nested"].(Map)["value"] = "changed"
			ctx.Args["bytes"].([]byte)[0] = 9
			ctx.Settings["nested"].(Map)["value"] = "changed"
			finished <- struct{}{}
		}})
	}
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	value := Map{"nested": Map{"value": "original"}, "bytes": []byte{1}}
	triggerHost.Toggle("work", value)
	one, two := awaitTrigger(t, entered), awaitTrigger(t, entered)
	if one == two {
		t.Fatal("callbacks share Meta")
	}
	value["nested"].(Map)["value"] = "caller mutation"
	value["bytes"].([]byte)[0] = 7
	close(release)
	awaitTrigger(t, finished)
	awaitTrigger(t, finished)
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
}

func TestTriggerMappingErrorsAndPanicDoNotStopOtherListeners(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	output := captureStartupLog(t)
	var seen []string
	Register("test", Trigger{Args: Vars{"id": {Type: "int", Required: true}}, Action: func(ctx *Context) {
		seen = append(seen, "mapped")
		if ctx.Args["id"] != int64(7) {
			t.Errorf("mapping: %v", ctx.Args)
		}
	}})
	Register("test", Trigger{Action: func(*Context) { panic("boom") }})
	Register("test", Trigger{Action: func(ctx *Context) { ctx.Result(ErrorResult(errors.New("failure"))) }})
	Register("test", Trigger{Action: func(*Context) { seen = append(seen, "last") }})
	Register("optional", Trigger{Nullable: true, Args: Vars{"id": {Required: true}}, Action: func(ctx *Context) {
		if len(ctx.Args) != 0 {
			t.Error("nullable arguments were injected")
		}
		seen = append(seen, "optional")
	}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	SyncToggle("test", Map{"id": "7"})
	SyncToggle("test", Map{})
	SyncToggle("optional")
	if !reflect.DeepEqual(seen, []string{"mapped", "last", "last", "optional"}) {
		t.Fatalf("listeners: %v", seen)
	}
	if !strings.Contains(output.String(), `trigger "test" failed: failure`) {
		t.Fatalf("callback result was not recorded: %s", output.String())
	}
}

func TestTriggerSetupRetryDoesNotDuplicateCallbacks(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	var calls int
	Register("test", Trigger{Action: func(*Context) { calls++ }})
	triggerProbe.setup = func(*Context) error { return errors.New("setup failure") }
	if err := rt.Run(); err == nil {
		t.Fatal("expected setup failure")
	}
	triggerProbe.setup = nil
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	SyncToggle("test")
	if calls != 1 || len(core.triggerMethods) != 1 {
		t.Fatalf("duplicate callbacks after retry: calls=%d methods=%v", calls, core.triggerMethods)
	}
}

func TestFailedStartupDoesNotEmitLifecycleTriggers(t *testing.T) {
	for _, phase := range []string{"open", "start"} {
		t.Run(phase, func(t *testing.T) {
			rt := testRuntime()
			t.Cleanup(Reset)
			var calls atomic.Int64
			Register(START, Trigger{Action: func(*Context) { calls.Add(1) }})
			Register(STOP, Trigger{Action: func(*Context) { calls.Add(1) }})
			failure := func(*Context) error { return errors.New("startup failed") }
			if phase == "open" {
				triggerProbe.open = failure
			} else {
				triggerProbe.start = failure
			}
			if err := rt.Run(); err == nil {
				t.Fatal("expected startup failure")
			}
			if calls.Load() != 0 {
				t.Fatal("failed startup emitted lifecycle notification")
			}
		})
	}
}

func TestOpenRollbackJoinsModuleTriggeredCallbacks(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	entered, finished := make(chan struct{}), make(chan struct{})
	Register("probe.opening", Trigger{Action: func(ctx *Context) { close(entered); <-ctx.Context.Done(); close(finished) }})
	triggerProbe.open = func(*Context) error {
		triggerHost.Toggle("probe.opening")
		awaitTrigger(t, entered)
		return errors.New("open failed")
	}
	if err := rt.Run(); err == nil {
		t.Fatal("expected open failure")
	}
	select {
	case <-finished:
	default:
		t.Fatal("rollback retained active callback")
	}
}

func TestEndCancelsAndJoinsActiveTriggers(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	entered := make(chan struct{})
	finished := make(chan struct{})
	Register("wait", Trigger{Action: func(ctx *Context) { close(entered); <-ctx.Context.Done(); close(finished) }})
	triggerProbe.stop = func(*Context) error {
		select {
		case <-finished:
		default:
			t.Error("module stopped before trigger completed")
		}
		return nil
	}
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	Toggle("wait")
	awaitTrigger(t, entered)
	ended := make(chan error, 1)
	go func() { ended <- rt.End() }()
	if err := awaitTrigger(t, ended); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("callback not joined")
	}
}

func TestConcurrentToggleAndReset(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	var calls atomic.Int64
	Register("count", Trigger{Action: func(*Context) { calls.Add(1) }})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	var senders sync.WaitGroup
	for i := 0; i < 8; i++ {
		senders.Add(1)
		go func() {
			defer senders.Done()
			for j := 0; j < 30; j++ {
				Toggle("count")
			}
		}()
	}
	senders.Wait()
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 240 {
		t.Fatalf("lost callbacks: %d", calls.Load())
	}
	Reset()
	if len(core.triggerMethods) != 0 || len(core.triggers) != 0 || len(triggers.methods) != 0 {
		t.Fatal("Reset retained triggers")
	}
	if err := DefaultRuntime().Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	SyncToggle("count")
	if calls.Load() != 240 {
		t.Fatal("old callbacks survived Reset")
	}
}
