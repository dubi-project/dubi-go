package dubi

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

type testModule struct {
	state    []string
	metaGood bool
}

func (m *testModule) New() Instance        { return m }
func (m *testModule) Register(string, any) {}
func (m *testModule) Config(Map) error     { m.state = append(m.state, "config"); return nil }
func (m *testModule) Setup(ctx *Context) error {
	m.metaGood = ctx != nil && ctx.Meta != nil && ctx.Meta.runtime == ctx.Runtime
	m.state = append(m.state, "setup")
	return nil
}
func (m *testModule) Open(*Context) error  { m.state = append(m.state, "open"); return nil }
func (m *testModule) Start(*Context) error { m.state = append(m.state, "start"); return nil }
func (m *testModule) Stop(*Context) error  { m.state = append(m.state, "stop"); return nil }
func (m *testModule) Close(*Context) error { m.state = append(m.state, "close"); return nil }

type receivingModule struct {
	testModule
	registered []string
}

func (m *receivingModule) New() Instance { return m }

type scopedRouter struct{}

func (scopedRouter) Scope() string { return "web:router" }

func (m *receivingModule) Register(name string, _ any) {
	m.registered = append(m.registered, name)
}

type isolatedModule struct {
	mu        sync.Mutex
	instances []*isolatedInstance
}

type isolatedInstance struct {
	mu    sync.Mutex
	names []string
}

func (m *isolatedModule) New() Instance {
	instance := &isolatedInstance{}
	m.mu.Lock()
	m.instances = append(m.instances, instance)
	m.mu.Unlock()
	return instance
}

func (i *isolatedInstance) Register(name string, _ any) {
	i.mu.Lock()
	i.names = append(i.names, name)
	i.mu.Unlock()
}
func (*isolatedInstance) Config(Map) error     { return nil }
func (*isolatedInstance) Setup(*Context) error { return nil }
func (*isolatedInstance) Open(*Context) error  { return nil }
func (*isolatedInstance) Start(*Context) error { return nil }
func (*isolatedInstance) Stop(*Context) error  { return nil }
func (*isolatedInstance) Close(*Context) error { return nil }

var (
	testLifecycleModule = &testModule{}
	testReceiverModule  = &receivingModule{}
	testIsolatedModule  = &isolatedModule{}
)

func init() {
	Mount(testLifecycleModule)
	Mount(testReceiverModule)
	Mount(testIsolatedModule)
}

func TestRuntimeLoadsAllAndRunsLifecycle(t *testing.T) {
	r := NewRegistry()
	r.Register("user.get", struct{}{})
	rt := NewRuntime()
	copyRegistry(rt, r)
	m := testLifecycleModule
	m.state = nil
	ctx := context.Background()
	if err := rt.Setup(ctx); err != nil {
		t.Fatal(err)
	}
	if got := names(rt.Entries()); !reflect.DeepEqual(got, []string{"user.get"}) {
		t.Fatalf("entries = %v", got)
	}
	if err := rt.Open(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
	if got, want := m.state, []string{"config", "setup", "open", "start", "stop", "close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle = %v, want %v", got, want)
	}
	if !m.metaGood {
		t.Fatal("lifecycle context did not receive Runtime-bound Meta")
	}
}

func TestRuntimeProfileAndLocalOverride(t *testing.T) {
	r := NewRegistry()
	r.Register("api", Profile{Include: []string{"user.*", "http.*"}, Exclude: []string{"user.internal.*"}})
	r.Register("user.get", "global")
	r.Register("user.internal.debug", "excluded")
	r.Register("worker.run", "excluded")
	rt := NewRuntime()
	copyRegistry(rt, r)
	rt.identity.Profile = "api"
	rt.Register("user.get", "local")
	rt.Register("http.index", struct{}{})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := names(rt.Entries()); !reflect.DeepEqual(got, []string{"http.index", "user.get"}) {
		t.Fatalf("profile entries = %v", got)
	}
	v, ok := find(rt.Entries(), "user.get")
	if !ok || v != "local" {
		t.Fatalf("local override = %v, %v", v, ok)
	}
}

func TestRuntimeDispatchesRegistrationsToMountedModules(t *testing.T) {
	r := NewRegistry()
	m := testReceiverModule
	m.registered = nil
	r.Register("http.index", struct{}{})
	r.Register("http.filter", struct{}{})
	r.Register("user.get", struct{}{})

	rt := NewRuntime()
	copyRegistry(rt, r)
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := m.registered, []string{"http.filter", "http.index", "user.get"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered = %v, want %v", got, want)
	}
}

func TestProfileMatchesComponentScope(t *testing.T) {
	r := NewRegistry()
	r.Register("api", Profile{Include: []string{"web:router:*"}})
	r.Register("index", scopedRouter{})
	r.Register("debug", struct{}{})
	rt := NewRuntime()
	copyRegistry(rt, r)
	rt.identity.Profile = "api"
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := rt.Entries()
	if len(entries) != 1 || entries[0].Key() != "web:router:index" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestProfileKeepsFoundationRegistrations(t *testing.T) {
	rt := NewRuntime()
	rt.Register("web", Profile{Include: []string{"core:method:*"}})
	rt.Register("user.get", Method{})
	rt.Register("custom-type", Type{Name: "custom"})
	rt.Register("custom-codec", Codec{
		Encode: func(value any) (any, error) {
			return []byte("custom"), nil
		},
	})
	rt.identity.Profile = "web"

	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := find(rt.Entries(), "custom-type"); !ok {
		t.Fatalf("foundation type was filtered by profile: %#v", rt.Entries())
	}
	if _, ok := find(rt.Entries(), "custom-codec"); !ok {
		t.Fatalf("foundation codec was filtered by profile: %#v", rt.Entries())
	}
	if _, ok := rt.basic.types["custom-type"]; !ok {
		t.Fatalf("custom type was not registered in the Runtime basic instance: %#v", rt.basic.types)
	}
	encoded, err := rt.Marshal("custom-codec", "value")
	if err != nil || string(encoded) != "custom" {
		t.Fatalf("custom codec = %q, %v", encoded, err)
	}
}

func TestRuntimeManagerTracksMultipleRuntimes(t *testing.T) {
	first := NewRuntime()
	second := NewRuntime()
	ctx := context.Background()
	for _, rt := range []*Runtime{first, second} {
		if err := rt.Setup(ctx); err != nil {
			t.Fatal(err)
		}
		if err := rt.Open(ctx); err != nil {
			t.Fatal(err)
		}
		if err := rt.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if runtimes, _ := runtimeManager.snapshot(); len(runtimes) != 2 {
		t.Fatalf("active runtimes = %d, want 2", len(runtimes))
	}
	if err := first.End(); err != nil {
		t.Fatal(err)
	}
	if runtimes, _ := runtimeManager.snapshot(); len(runtimes) != 1 {
		t.Fatalf("active runtimes = %d, want 1", len(runtimes))
	}
	if err := second.End(); err != nil {
		t.Fatal(err)
	}
	if runtimes, _ := runtimeManager.snapshot(); len(runtimes) != 0 {
		t.Fatalf("active runtimes = %d, want 0", len(runtimes))
	}
}

func TestRuntimeCreatesIsolatedModuleInstances(t *testing.T) {
	testIsolatedModule.mu.Lock()
	start := len(testIsolatedModule.instances)
	testIsolatedModule.mu.Unlock()

	first := NewRuntime()
	second := NewRuntime()
	first.Register("instance.first", struct{}{})
	second.Register("instance.second", struct{}{})
	ctx := context.Background()
	for _, rt := range []*Runtime{first, second} {
		if err := rt.Setup(ctx); err != nil {
			t.Fatal(err)
		}
		if err := rt.Open(ctx); err != nil {
			t.Fatal(err)
		}
		if err := rt.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
	defer first.End()
	defer second.End()

	testIsolatedModule.mu.Lock()
	instances := append([]*isolatedInstance(nil), testIsolatedModule.instances[start:]...)
	testIsolatedModule.mu.Unlock()
	if len(instances) != 2 {
		t.Fatalf("instances = %d, want 2", len(instances))
	}
	if !contains(instanceNames(instances[0]), "instance.first") || contains(instanceNames(instances[0]), "instance.second") {
		t.Fatalf("first instance registrations = %v", instanceNames(instances[0]))
	}
	if !contains(instanceNames(instances[1]), "instance.second") || contains(instanceNames(instances[1]), "instance.first") {
		t.Fatalf("second instance registrations = %v", instanceNames(instances[1]))
	}
}

func TestRuntimeRunAndEnd(t *testing.T) {
	rt := NewRuntime()
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		runtimes, _ := runtimeManager.snapshot()
		if containsRuntime(runtimes, rt) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("runtime did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
}

func containsRuntime(runtimes []*Runtime, want *Runtime) bool {
	for _, rt := range runtimes {
		if rt == want {
			return true
		}
	}
	return false
}

func instanceNames(instance *isolatedInstance) []string {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	return append([]string(nil), instance.names...)
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func names(entries []Entry) []string {
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		result = append(result, entry.Name)
	}
	return result
}

func find(entries []Entry, name string) (any, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry.Value, true
		}
	}
	return nil, false
}

func copyRegistry(rt *Runtime, registry *Registry) {
	for _, entry := range registry.Entries() {
		rt.Register(entry.Name, entry.Value)
	}
}
