package dubi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func receiveBus[T any](t *testing.T, channel <-chan T) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for BUS execution")
	}
	var zero T
	return zero
}

func startTestBus(t *testing.T, rt *Runtime, profile ...string) {
	t.Helper()
	if err := rt.Run(profile...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.End(); err != nil {
			t.Error(err)
		}
	})
}

func waitMemoryJobs() { bus.memory.active.Wait() }

func TestBusRequestAndInvokeRouting(t *testing.T) {
	rt := testRuntime()
	rt.Register("same", Method{Action: func(*Context) Map { return Map{"kind": "method"} }})
	rt.Register("same", Service{Action: func(*Context) Map { return Map{"kind": "service"} }})
	for _, name := range []string{"a", "b"} {
		rt.Register(name, Service{Listen: "topic", Action: func(ctx *Context) Map { return Map{"name": ctx.Name} }})
	}
	startTestBus(t, rt)
	if got, _ := Invoke("same"); got["kind"] != "method" {
		t.Fatalf("Invoke = %v", got)
	}
	if got, _ := Request("same"); got["kind"] != "service" {
		t.Fatalf("Request = %v", got)
	}
	for i := 0; i < 4; i++ {
		name := []string{"a", "b"}[i%2]
		got, res := Request(name)
		if (res != nil && res.Fail()) || got["name"] != []string{"a", "b"}[i%2] {
			t.Fatalf("round robin = %v, %v", got, res)
		}
	}
	meta := NewMeta()
	if got := meta.Request("same"); got["kind"] != "service" || meta.Result().Fail() {
		t.Fatalf("Meta.Request = %v", got)
	}
	if got := meta.Invoke("topic"); got != nil || meta.Result() == nil || meta.Result().OK() {
		t.Fatal("Invoke treated Listen as a service alias")
	}
	if _, res := Request("missing"); res == nil || res.OK() {
		t.Fatal("missing request succeeded")
	}
	if _, _, found := (moduleHost{}).InvokeLocalService(meta, "topic", nil); found {
		t.Fatal("Host recursed into BUS")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	meta.WithContext(cancelled)
	if meta.Request("same") != nil || meta.Result() == nil || meta.Result().OK() {
		t.Fatal("cancelled Request succeeded")
	}
}

func TestBusBroadcastAndDispatch(t *testing.T) {
	rt := testRuntime(WithRole("worker"), WithTenant("tenant"))
	seen := make(chan string, 16)
	for name, group := range map[string]string{"mail.a": "mail", "mail.b": "mail", "audit": "audit", "default": ""} {
		rt.Register(name, Service{Listen: "created", Group: group, Action: func(ctx *Context) { seen <- ctx.Name }})
	}
	startTestBus(t, rt)
	if core.services["default"].Group != "worker" {
		t.Fatal("default role group not resolved")
	}
	meta := NewMeta()
	meta.Result(Fail)
	for _, name := range []string{"mail.a", "mail.b", "audit", "default"} {
		if err := meta.Broadcast(name); err != nil {
			t.Fatal(err)
		}
	}
	waitMemoryJobs()
	counts := map[string]int{}
	for len(seen) > 0 {
		counts[<-seen]++
	}
	if len(counts) != 4 {
		t.Fatalf("broadcast recipients = %v", counts)
	}
	for i := 0; i < 2; i++ {
		if err := meta.Dispatch("mail.a"); err != nil {
			t.Fatal(err)
		}
	}
	waitMemoryJobs()
	counts = map[string]int{}
	for len(seen) > 0 {
		counts[<-seen]++
	}
	if !reflect.DeepEqual(counts, map[string]int{"mail.a": 2}) {
		t.Fatalf("dispatch recipients = %v", counts)
	}
	if meta.Result() != Fail {
		t.Fatal("async notification changed caller result")
	}
	if err := Broadcast("no.listeners"); err != nil {
		t.Fatal(err)
	}
	if err := Dispatch("no.listeners"); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue("no.listeners"); !errors.Is(err, ErrBusNoService) {
		t.Fatalf("missing queue consumer = %v", err)
	}
}

func TestServiceEffectiveGroup(t *testing.T) {
	for _, tc := range []struct{ group, role, tenant, want string }{
		{"explicit", "worker", "tenant", "explicit"}, {"", "worker", "tenant", "worker"},
		{"", "", "tenant", "tenant"}, {"", "", "", "dubi"},
	} {
		if got := serviceGroup(tc.group, Identity{Role: tc.role, Tenant: tc.tenant}); got != tc.want {
			t.Fatalf("group = %q, want %q", got, tc.want)
		}
	}
}

func TestBusQueueSelectsOneAndBroadcastIsolatesRecipients(t *testing.T) {
	rt := testRuntime()
	type observation struct {
		name, value string
		attempt     int
	}
	seen := make(chan observation, 8)
	for _, name := range []string{"a", "b"} {
		rt.Register(name, Service{Listen: "shared", Group: name, Action: func(ctx *Context) {
			payload := ctx.Value["nested"].(Map)
			seen <- observation{ctx.Name, payload["value"].(string), ctx.Attempt()}
			payload["value"] = ctx.Name
		}})
	}
	startTestBus(t, rt)
	value := Map{"nested": Map{"value": "original"}}
	for _, name := range []string{"a", "b"} {
		if err := Enqueue(name, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"a", "b"} {
		if err := Broadcast(name, value); err != nil {
			t.Fatal(err)
		}
	}
	waitMemoryJobs()
	counts := map[observation]int{}
	for len(seen) > 0 {
		counts[<-seen]++
	}
	for _, name := range []string{"a", "b"} {
		for _, attempt := range []int{0, 1} {
			if counts[observation{name, "original", attempt}] != 1 {
				t.Fatalf("deliveries = %v", counts)
			}
		}
	}
	if len(counts) != 4 || value["nested"].(Map)["value"] != "original" {
		t.Fatalf("unexpected deliveries/payload: %v, %v", counts, value)
	}
}

func TestBusQueueRetriesAndSnapshots(t *testing.T) {
	rt := testRuntime()
	type attempt struct {
		id       string
		number   int
		language string
		payload  string
		at       time.Time
	}
	seen := make(chan attempt, 4)
	gate := make(chan struct{})
	rt.Register("job", Service{Retry: []time.Duration{10 * time.Millisecond, 15 * time.Millisecond}, Action: func(ctx *Context) (Map, Res) {
		<-gate
		payload := ctx.Value["nested"].(Map)
		seen <- attempt{ctx.Id(), ctx.Attempt(), ctx.Language(), payload["value"].(string), time.Now()}
		payload["value"] = "mutated by handler"
		if ctx.Attempt() < 3 {
			return nil, Fail
		}
		return Map{"ok": true}, OK
	}})
	startTestBus(t, rt)
	ctx, cancel := context.WithCancel(context.Background())
	meta := NewMeta().WithContext(ctx)
	meta.Language("original")
	value := Map{"nested": Map{"value": "original"}}
	if err := meta.Enqueue("job", value); err != nil {
		t.Fatal(err)
	}
	cancel() // Accepted jobs outlive their HTTP/entry request.
	meta.Reset(context.Background())
	meta.Language("reused")
	value["nested"].(Map)["value"] = "caller mutation"
	close(gate)
	first := receiveBus(t, seen)
	second := receiveBus(t, seen)
	third := receiveBus(t, seen)
	waitMemoryJobs()
	for i, item := range []attempt{first, second, third} {
		if item.id == "" || item.id != first.id || item.number != i+1 || item.language != "original" || item.payload != "original" {
			t.Fatalf("attempt snapshot = %#v", item)
		}
	}
	if second.at.Sub(first.at) < 10*time.Millisecond || third.at.Sub(second.at) < 15*time.Millisecond {
		t.Fatal("retry delays were skipped")
	}
	if failures := BusFailures(); len(failures) != 0 {
		t.Fatalf("successful job recorded as failed: %v", failures)
	}
}

func TestBusExhaustionPanicAndNotificationNoRetry(t *testing.T) {
	rt := testRuntime()
	seen := make(chan int, 16)
	rt.Register("broken", Service{Retry: []time.Duration{0}, Action: func(ctx *Context) {
		seen <- ctx.Attempt()
		panic("broken")
	}})
	startTestBus(t, rt)
	if err := Broadcast("broken"); err != nil {
		t.Fatal(err)
	}
	if err := Dispatch("broken"); err != nil {
		t.Fatal(err)
	}
	if err := Enqueue("broken", Map{"v": "saved"}); err != nil {
		t.Fatal(err)
	}
	waitMemoryJobs()
	counts := map[int]int{}
	for len(seen) > 0 {
		counts[<-seen]++
	}
	if !reflect.DeepEqual(counts, map[int]int{0: 2, 1: 1, 2: 1}) {
		t.Fatalf("attempts = %v", counts)
	}
	failures := BusFailures()
	if len(failures) != 1 || failures[0].Attempts != 2 || failures[0].ID == "" || failures[0].Error == "" {
		t.Fatalf("failures = %#v", failures)
	}
	failures[0].Value["v"] = "changed"
	if BusFailures()[0].Value["v"] != "saved" {
		t.Fatal("failure payload leaked")
	}
}

func TestBusCapacityAndShutdown(t *testing.T) {
	rt := testRuntime()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"bus":{"options":{"workers":1,"capacity":1}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", path)
	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	Register("fan", Notification{})
	for _, name := range []string{"fan.a", "fan.b"} {
		rt.Register(name, Service{Listen: "fan", Group: name, Action: func(*Context) { t.Error("partial fan-out despite rejected admission") }})
	}
	rt.Register("block", Service{Action: func(ctx *Context) { started <- struct{}{}; <-ctx.Done(); finished <- struct{}{} }})
	startTestBus(t, rt)
	if err := Notify("fan"); !errors.Is(err, ErrBusFull) {
		t.Fatalf("fan-out capacity = %v", err)
	}
	if err := Enqueue("block"); err != nil {
		t.Fatal(err)
	}
	receiveBus(t, started)
	if err := Enqueue("block"); !errors.Is(err, ErrBusFull) {
		t.Fatalf("capacity = %v", err)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	receiveBus(t, finished)
	if err := Enqueue("block"); !errors.Is(err, ErrBusNotReady) {
		t.Fatalf("enqueue after stop = %v", err)
	}
	if _, res := Request("block"); res == nil || res.OK() {
		t.Fatal("request after stop succeeded")
	}
}

func TestBusShutdownCancelsRetry(t *testing.T) {
	rt := testRuntime()
	seen := make(chan int, 4)
	rt.Register("retry", Service{Retry: []time.Duration{time.Hour}, Action: func(ctx *Context) Res { seen <- ctx.Attempt(); return Fail }})
	startTestBus(t, rt)
	if err := Enqueue("retry"); err != nil {
		t.Fatal(err)
	}
	receiveBus(t, seen)
	done := make(chan error, 1)
	go func() { done <- rt.End() }()
	if err := receiveBus(t, done); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatal("retry ran after stop")
	}
}

type testBusHook struct {
	testModule
	host     Host
	services []ServiceInfo
	calls    []string
	notified Map
}

func (h *testBusHook) Setup(*Context) error { h.services = h.host.Services(); return nil }
func (h *testBusHook) Request(meta *Meta, name string, value Map) (Map, Res) {
	h.calls = append(h.calls, "request:"+name)
	if name == "remote" {
		return Map{"remote": true}, OK
	}
	data, result, _ := h.host.InvokeLocalService(meta, name, value)
	return data, result
}
func (h *testBusHook) Broadcast(*Meta, string, Map) error {
	h.calls = append(h.calls, "broadcast")
	return nil
}
func (h *testBusHook) Dispatch(*Meta, string, Map) error {
	h.calls = append(h.calls, "dispatch")
	return nil
}
func (h *testBusHook) Enqueue(*Meta, string, Map) error {
	h.calls = append(h.calls, "enqueue")
	return nil
}

func mountTestBus(t *testing.T, hook *testBusHook) {
	t.Helper()
	previous := globalModules()
	hook.host = Mount(hook)
	t.Cleanup(func() {
		Reset()
		mounted.Lock()
		mounted.items = previous
		mounted.Unlock()
	})
}

func TestBusHookTakeoverAndProfileCatalog(t *testing.T) {
	rt := testRuntime(WithEndpoint("reserved-server:7000"), WithRole("worker"))
	hook := &testBusHook{}
	mountTestBus(t, hook)
	rt.Register("keep", Service{Listen: "topic", Retry: []time.Duration{time.Second}, Action: func(*Context) Map { return Map{"local": true} }})
	rt.Register("keep", Method{Action: func(*Context) Map { return Map{"wrong": true} }})
	rt.Register("skip", Service{})
	rt.Register("selected", Profile{Include: []string{"core:service:keep", "core:method:keep"}})
	startTestBus(t, rt, "selected")
	if bus.memory != nil {
		t.Fatal("memory BUS allocated despite external hook")
	}
	if len(hook.services) != 1 || hook.services[0].Name != "keep" || !reflect.DeepEqual(hook.services[0].Listens, []string{"topic"}) || hook.services[0].Group != "worker" {
		t.Fatalf("catalog = %#v", hook.services)
	}
	hook.services[0].Retry[0] = time.Hour
	if hook.host.Services()[0].Retry[0] != time.Second {
		t.Fatal("catalog leaked retry slice")
	}
	if data, res := Request("keep"); (res != nil && res.Fail()) || data["local"] != true {
		t.Fatalf("Host execution = %v, %v", data, res)
	}
	if data, _ := Invoke("remote"); data["remote"] != true {
		t.Fatalf("remote fallback = %v", data)
	}
	for _, call := range []func(string, ...Map) error{Broadcast, Dispatch, Enqueue} {
		if err := call("topic"); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(hook.calls, []string{"request:keep", "request:remote", "broadcast", "dispatch", "enqueue"}) {
		t.Fatalf("hook calls = %v", hook.calls)
	}
}

func TestBusHookConflict(t *testing.T) {
	testRuntime()
	hook := &testBusHook{}
	mountTestBus(t, hook)
	Mount(hook) // Same instance is idempotent.
	defer func() {
		if recover() == nil {
			t.Error("second external BUS hook was accepted")
		}
	}()
	Mount(&testBusHook{})
}

func TestBusHostRestoresDeliveryIdentity(t *testing.T) {
	rt := testRuntime()
	rt.Register("job", Service{Action: func(ctx *Context) Map {
		return Map{"id": ctx.Id(), "attempt": ctx.Attempt()}
	}})
	startTestBus(t, rt)
	meta := NewMeta()
	id := meta.Id()
	data, result, found := (moduleHost{}).InvokeLocalService(meta, "job", nil, BusDelivery{ID: "remote-task", Attempt: 2})
	if !found || (result != nil && result.Fail()) || data["id"] != "remote-task" || data["attempt"] != 2 {
		t.Fatalf("delivery = %v, %v, %v", data, result, found)
	}
	if meta.Id() != id || meta.Attempt() != 0 {
		t.Fatal("delivery changed caller Meta")
	}
}

func TestBusExternalDeliveryCancellationAndPanic(t *testing.T) {
	rt := testRuntime()
	hook := &testBusHook{}
	mountTestBus(t, hook)
	started := make(chan struct{}, 1)
	rt.Register("incoming", Service{Action: func(ctx *Context) Res {
		started <- struct{}{}
		<-ctx.Done()
		return ErrorResult(ctx.Err())
	}})
	rt.Register("panic", Service{Action: func(*Context) { panic("external delivery panic") }})
	startTestBus(t, rt)
	if _, result, found := hook.host.InvokeLocalService(nil, "panic", nil); !found || result == nil || result.OK() {
		t.Fatalf("incoming panic = %v, %v", result, found)
	}
	done := make(chan struct{}, 1)
	go func() {
		hook.host.InvokeLocalService(NewMeta(), "incoming", nil)
		done <- struct{}{}
	}()
	receiveBus(t, started)
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	receiveBus(t, done)
	if _, result, found := hook.host.InvokeLocalService(nil, "incoming", nil); found || result == nil || result.OK() {
		t.Fatal("incoming delivery accepted after stop")
	}
}

func TestBusStartupRetryRebuildsSelectedSubscriptions(t *testing.T) {
	rt := testRuntime()
	rt.Register("keep", Service{Listen: "shared", Action: func(*Context) Map { return Map{"keep": true} }})
	rt.Register("removed", Service{Listen: "removed", Action: func(*Context) Map { return Map{"wrong": true} }})
	rt.Register("selected", Profile{Include: []string{"core:service:keep"}})
	testLifecycleModule.startError = errors.New("startup failure")
	if err := rt.Run(""); err == nil {
		t.Fatal("startup unexpectedly succeeded")
	}
	if err := Broadcast("shared"); !errors.Is(err, ErrBusNotReady) {
		t.Fatalf("BUS active after rollback: %v", err)
	}
	testLifecycleModule.startError = nil
	startTestBus(t, rt, "selected")
	if data, _ := Request("keep"); data["keep"] != true {
		t.Fatalf("selected subscription = %v", data)
	}
	if _, result := Invoke("removed"); result == nil || result.OK() {
		t.Fatal("old subscription survived startup retry")
	}
}

func TestBusServerDoesNotFallBackAndStartupCanRetry(t *testing.T) {
	rt := testRuntime(WithEndpoint("server:7000"))
	if err := rt.Run(""); !errors.Is(err, ErrBusServerUnavailable) {
		t.Fatalf("server transport = %v", err)
	}
	Configure(WithEndpoints())
	rt.Register("hello", Service{Action: func(*Context) Map { return Map{"ok": true} }})
	startTestBus(t, rt)
	if data, _ := Request("hello"); data["ok"] != true {
		t.Fatalf("startup retry = %v", data)
	}
}

func TestBusConcurrentDispatchAndStop(t *testing.T) {
	rt := testRuntime()
	rt.Register("hello", Service{Action: func(*Context) {}})
	startTestBus(t, rt)
	var calls sync.WaitGroup
	for i := 0; i < 8; i++ {
		calls.Add(1)
		go func() {
			defer calls.Done()
			for j := 0; j < 30; j++ {
				if err := Dispatch("hello"); err != nil && !errors.Is(err, ErrBusNotReady) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrBusFull) {
					t.Error(err)
				}
			}
		}()
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	calls.Wait()
}

func TestBusRejectsInvalidRetryAndOptions(t *testing.T) {
	rt := testRuntime()
	rt.Register("invalid", Service{Retry: []time.Duration{-time.Second}})
	if err := rt.Run(""); err == nil {
		t.Fatal("negative retry accepted")
	}
	for _, options := range []Map{{"workers": 0}, {"capacity": -1}, {"workers": 1.5}, {"unknown": 1}} {
		if _, err := parseMemoryBusOptions(Map{"bus": Map{"options": options}}); err == nil {
			t.Fatalf("invalid options accepted: %v", options)
		}
	}
}

func (h *testBusHook) Notify(_ *Meta, _ string, value Map) error {
	h.calls = append(h.calls, "notify")
	h.notified = value
	return nil
}
