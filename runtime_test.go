package dubi

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

import . "github.com/dubi-project/dubi-go/base"

type testModule struct {
	state          []string
	metaGood       bool
	setupReentrant bool
	reentrantError error
}

func (m *testModule) Reset() {
	m.state = nil
	m.metaGood = false
	m.setupReentrant = false
	m.reentrantError = nil
}
func (m *testModule) Register(string, any) {}
func (m *testModule) Config(Map) error {
	m.state = append(m.state, "config")
	return nil
}
func (m *testModule) Setup(ctx *Context) error {
	m.metaGood = ctx != nil && ctx.Meta != nil && ctx.Meta.Context() != nil
	if m.setupReentrant {
		m.reentrantError = DefaultRuntime().Open(context.Background())
	}
	m.state = append(m.state, "setup")
	return nil
}
func (m *testModule) Open(*Context) error {
	m.state = append(m.state, "open")
	return nil
}
func (m *testModule) Start(*Context) error {
	m.state = append(m.state, "start")
	return nil
}
func (m *testModule) Stop(*Context) error {
	m.state = append(m.state, "stop")
	return nil
}
func (m *testModule) Close(*Context) error {
	m.state = append(m.state, "close")
	return nil
}

type receivingModule struct {
	testModule
	registered []string
}

func (m *receivingModule) Reset() {
	m.testModule.Reset()
	m.registered = nil
}
func (m *receivingModule) Register(name string, _ any) {
	m.registered = append(m.registered, name)
}

type scopedRouter struct{}

func (scopedRouter) Scope() string { return "web:router" }

var (
	testLifecycleModule = &testModule{}
	testReceiverModule  = &receivingModule{}
)

func init() {
	Mount(testLifecycleModule)
	Mount(testReceiverModule)
}

func testRuntime(options ...RuntimeOption) *Runtime {
	Reset()
	Configure(options...)
	return DefaultRuntime()
}

func TestRuntimeLoadsAllAndRunsLifecycle(t *testing.T) {
	rt := testRuntime()
	rt.Register("user.get", struct{}{})
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
	if got, want := testLifecycleModule.state, []string{"config", "setup", "open", "start", "stop", "close"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lifecycle = %v, want %v", got, want)
	}
	if !testLifecycleModule.metaGood {
		t.Fatal("lifecycle context did not receive the process runtime")
	}
}

func TestRuntimeIsProcessSingleton(t *testing.T) {
	Reset()
	first := DefaultRuntime()
	second := DefaultRuntime()
	if first != second || first != DefaultRuntime() {
		t.Fatalf("runtime handles are not process singleton: first=%p second=%p default=%p", first, second, DefaultRuntime())
	}
}

func TestMountIsIdempotentForSameModule(t *testing.T) {
	Reset()
	before := len(globalModules())
	module := &testModule{}
	Mount(module)
	once := len(globalModules())
	Mount(module)
	if got := len(globalModules()); got != once || once != before+1 {
		t.Fatalf("mount count = %d after duplicate mount, want %d", got, once)
	}
}

func TestMountRejectsAfterSetup(t *testing.T) {
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	defer func() {
		if recover() == nil {
			t.Fatal("mount after setup did not panic")
		}
	}()
	Mount(&testModule{})
}

func TestLifecycleRejectsReentrantCalls(t *testing.T) {
	rt := testRuntime()
	testLifecycleModule.setupReentrant = true
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	if err := testLifecycleModule.reentrantError; err == nil || !strings.Contains(err.Error(), "while setup is running") {
		t.Fatalf("reentrant lifecycle error = %v", err)
	}
}

func TestSetupFreezesProcessRegistry(t *testing.T) {
	rt := testRuntime()
	Register("before", struct{}{})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	defer func() {
		if recover() == nil {
			t.Fatal("registration after Setup did not panic")
		}
	}()
	Register("after", struct{}{})
}

func TestRuntimeProfileAndRegistry(t *testing.T) {
	rt := testRuntime()
	rt.Register("api", Profile{Include: []string{"user.*", "http.*"}, Exclude: []string{"user.internal.*"}})
	rt.Register("user.get", "global")
	rt.Register("user.internal.debug", "excluded")
	rt.Register("worker.run", "excluded")
	rt.Register("http.index", struct{}{})
	identity := rt.identity
	identity.Profile = "api"
	rt.identity = identity
	rt.Register("user.get", "local")
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
	rt := testRuntime()
	rt.Register("http.index", struct{}{})
	rt.Register("http.filter", struct{}{})
	rt.Register("user.get", struct{}{})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := testReceiverModule.registered, []string{"http.filter", "http.index", "user.get"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("registered = %v, want %v", got, want)
	}
}

func TestProfileMatchesComponentScope(t *testing.T) {
	rt := testRuntime()
	rt.Register("api", Profile{Include: []string{"web:router:*"}})
	rt.Register("index", scopedRouter{})
	rt.Register("debug", struct{}{})
	rt.identity.Profile = "api"
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries := rt.Entries()
	if len(entries) != 1 || entries[0].Key() != "web:router:index" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestRuntimeRunAndEnd(t *testing.T) {
	rt := testRuntime()
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
}

func TestResetClearsProcessWaitState(t *testing.T) {
	rt := testRuntime()
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	Reset()
	if active, _ := runtimeManager.snapshot(); active != nil {
		t.Fatalf("reset left an active runtime: %p", active)
	}
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
