package dubi

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	. "github.com/dubi-project/dubi-go/base"
)

type lifecycle uint8

const (
	stateNew lifecycle = iota
	stateSetup
	stateOpen
	stateStarted
	stateStopped
	stateClosed
)

// Module is a process-level singleton module. It is mounted once and receives
// the process lifecycle directly; there is no per-runtime instance.
type Module interface {
	Register(string, any)
	Config(Map) error
	Setup(*Context) error
	Open(*Context) error
	Start(*Context) error
	Stop(*Context) error
	Close(*Context) error
}

// RuntimeOption is retained as a compatibility hook for code that builds the
// process runtime before Run. There is only one effective Runtime per process.
type RuntimeOption func(*Runtime)

type Context struct {
	context.Context
	*Meta
	Name    string
	Config  any
	Setting Map
	Value   Map
	Args    Map
}

type Runtime struct {
	mu           sync.Mutex
	identity     Identity
	nodeSet      bool
	profileSet   bool
	allEntries   []Entry
	entries      []Entry
	config       Map
	configLoaded bool
	bootstrap    Bootstrap
	state        lifecycle
	transition   string
}

func newRuntime() *Runtime {
	identity := defaultIdentity()
	rt := &Runtime{
		config:     Map{},
		identity:   identity,
		bootstrap:  defaultBootstrap(),
		nodeSet:    identity.Node != "",
		profileSet: identity.Profile != "",
	}
	return rt
}

// resetRuntime resets the process singleton for Reset and embedded test hosts.
func resetRuntime(rt *Runtime) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	state := rt.state
	transition := rt.transition
	rt.mu.Unlock()
	if transition != "" {
		panic(fmt.Sprintf("dubi: reset called while %s is running", transition))
	}
	if state == stateStarted || state == stateOpen || state == stateSetup || state == stateStopped {
		_ = rt.shutdown(context.Background())
	}
	// shutdown only closes modules. Clear the process wait state as well so a
	// reset after Start cannot leave Wait observing the old runtime handle.
	runtimeManager.remove(rt)
	unfreezeModules()
	for _, mounted := range globalModules() {
		if resettable, ok := mounted.(interface{ Reset() }); ok {
			resettable.Reset()
		}
	}
	globalRegistry = NewRegistry()
	fresh := newRuntime()
	rt.mu.Lock()
	rt.identity = fresh.identity
	rt.nodeSet = fresh.nodeSet
	rt.profileSet = fresh.profileSet
	rt.allEntries = fresh.allEntries
	rt.entries = fresh.entries
	rt.config = fresh.config
	rt.configLoaded = fresh.configLoaded
	rt.bootstrap = fresh.bootstrap
	rt.state = fresh.state
	rt.transition = fresh.transition
	rt.mu.Unlock()
}

// Configure applies process-level options before Run.
func Configure(options ...RuntimeOption) {
	rt := DefaultRuntime()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.transition != "" {
		panic(fmt.Sprintf("dubi: runtime is busy with %s", rt.transition))
	}
	if rt.state != stateNew {
		panic("dubi: runtime is already prepared")
	}
	for _, option := range options {
		if option != nil {
			option(rt)
		}
	}
}

// Reset stops and clears the process singleton. It is intended for embedded
// hosts that deliberately run the framework more than once in one process,
// and for test isolation. Normal applications only call Run once.
func Reset() { resetRuntime(DefaultRuntime()) }

// Identity returns a copy of this Runtime's identity scope.
func (rt *Runtime) Identity() Identity {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	identity := rt.identity
	if identity.Node == "" {
		identity.Node = defaultNodeID()
	}
	return identity
}

// Endpoints returns the configured Dubi endpoints for this Runtime.
func (rt *Runtime) Endpoints() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.bootstrap.Endpoints...)
}

func Endpoints() []string { return DefaultRuntime().Endpoints() }

// Auth returns a copy of the connection authentication settings.
func (rt *Runtime) Auth() Map {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.bootstrap.Auth.Clone()
}

func Auth() Map { return DefaultRuntime().Auth() }

// Run prepares and starts this Runtime, then returns. Process-level waiting is
// provided by the package-level Run or Wait functions.
func (rt *Runtime) Run(profile ...string) error {
	if len(profile) > 1 {
		return fmt.Errorf("dubi: Run accepts at most one profile")
	}
	rt.mu.Lock()
	if rt.state != stateNew {
		state := rt.state
		rt.mu.Unlock()
		return fmt.Errorf("dubi: run called in state %s", state)
	}
	if rt.transition != "" {
		transition := rt.transition
		rt.mu.Unlock()
		return fmt.Errorf("dubi: run called while %s is running", transition)
	}
	if len(profile) == 1 {
		rt.identity.Profile = strings.TrimSpace(profile[0])
		rt.profileSet = true
	}
	rt.mu.Unlock()
	ctx := context.Background()
	if err := rt.Setup(ctx); err != nil {
		return err
	}
	if err := rt.Open(ctx); err != nil {
		return err
	}
	if err := rt.Start(ctx); err != nil {
		return err
	}
	return nil
}

// Run starts the process-wide default runtime.
func Run(profile ...string) error {
	if err := DefaultRuntime().Run(profile...); err != nil {
		return err
	}
	return Wait()
}

// End gracefully stops and closes the runtime. It is safe to call after a
// partially completed startup, and is a no-op for a new or already closed
// runtime.
func (rt *Runtime) End() error {
	err := rt.shutdown(context.Background())
	if err == nil {
		runtimeManager.remove(rt)
	}
	return err
}

func (rt *Runtime) shutdown(ctx context.Context) error {
	rt.mu.Lock()
	state := rt.state
	transition := rt.transition
	rt.mu.Unlock()
	if transition != "" {
		return fmt.Errorf("dubi: end called while %s is running", transition)
	}

	var first error
	if state == stateStarted {
		if err := rt.Stop(ctx); err != nil {
			first = err
		}
		state = stateStopped
	}
	if state == stateSetup || state == stateOpen || state == stateStopped {
		if err := rt.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// End closes the process-wide default runtime.
func End() error { return DefaultRuntime().End() }

// Setup, Open, Start, Stop, and Close expose the lifecycle for tests and
// embedded tooling. Normal applications should use Run and End.
func Setup(ctx context.Context) error { return DefaultRuntime().Setup(ctx) }
func Open(ctx context.Context) error  { return DefaultRuntime().Open(ctx) }
func Start(ctx context.Context) error { return DefaultRuntime().Start(ctx) }
func Stop(ctx context.Context) error  { return DefaultRuntime().Stop(ctx) }
func Close(ctx context.Context) error { return DefaultRuntime().Close(ctx) }

func Entries() []Entry    { return DefaultRuntime().Entries() }
func AllEntries() []Entry { return DefaultRuntime().AllEntries() }

// DefaultRuntime returns the process-wide runtime. It is created lazily and
// uses all global registrations unless a profile is selected. The returned
// handle is shared by the entire process.
func DefaultRuntime() *Runtime {
	defaultRuntimeMu.Lock()
	defer defaultRuntimeMu.Unlock()
	if defaultRuntime == nil {
		defaultRuntime = newRuntime()
	}
	return defaultRuntime
}

var (
	defaultRuntimeMu sync.Mutex
	defaultRuntime   *Runtime
)

// Register adds a process registration through the compatibility Runtime
// handle. It must happen before Setup; package-level Register is preferred.
func (rt *Runtime) Register(name string, value any) {
	// The process Registry is the source of truth and performs the setup-time
	// freeze check. Avoid taking Runtime.mu here: a module callback may invoke
	// this compatibility method while Setup is dispatching lifecycle hooks.
	Register(name, value)
}
func (rt *Runtime) Entries() []Entry {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]Entry(nil), rt.entries...)
}

// AllEntries returns all merged component entries before Profile filtering.
func (rt *Runtime) AllEntries() []Entry {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]Entry(nil), rt.allEntries...)
}

func (rt *Runtime) beginLifecycle(action string, expected ...lifecycle) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.transition != "" {
		return fmt.Errorf("dubi: %s called while %s is running", action, rt.transition)
	}
	for _, state := range expected {
		if rt.state == state {
			rt.transition = action
			return nil
		}
	}
	return fmt.Errorf("dubi: %s called in state %s", action, rt.state)
}

func (rt *Runtime) lifecycleTransition() string {
	rt.mu.Lock()
	transition := rt.transition
	rt.mu.Unlock()
	return transition
}

func (rt *Runtime) finishLifecycle(state lifecycle) {
	rt.mu.Lock()
	rt.state = state
	rt.transition = ""
	rt.mu.Unlock()
}

func (rt *Runtime) resetLifecycle(state lifecycle) {
	rt.finishLifecycle(state)
}

func (rt *Runtime) Setup(ctx context.Context) error {
	if err := rt.beginLifecycle("setup", stateNew); err != nil {
		return err
	}
	setupOK := false
	defer func() {
		if !setupOK {
			globalRegistry.unfreeze()
			unfreezeModules()
			rt.resetLifecycle(stateNew)
		}
	}()
	rt.mu.Lock()
	if !rt.configLoaded {
		if err := rt.loadConfig(); err != nil {
			rt.mu.Unlock()
			return err
		}
	}
	config := rt.config.Clone()
	profileName := rt.identity.Profile
	rt.mu.Unlock()
	merged := make(map[string]any)
	profiles := make(map[string]Profile)
	registered := globalRegistry.Entries()
	globalRegistry.Freeze()
	freezeModules()
	for _, entry := range registered {
		if profile, ok := asProfile(entry.Value); ok {
			profiles[entry.Name] = profile
			continue
		}
		merged[entry.Name] = entry.Value
	}
	configured, err := configuredProfiles(config)
	if err != nil {
		return fmt.Errorf("dubi: load profiles: %w", err)
	}
	for name, profile := range configured {
		profiles[name] = profile
	}
	all := make([]Entry, 0, len(merged))
	for name, value := range merged {
		all = append(all, Entry{Name: name, Value: value, Scope: registrationScope(value)})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	rt.mu.Lock()
	rt.allEntries = all
	rt.mu.Unlock()
	var active *Profile
	if profileName != "" {
		profile, err := resolveProfiles(profiles, profileName)
		if err != nil {
			return err
		}
		active = &profile
	}
	selected := make([]Entry, 0, len(all))
	for _, entry := range all {
		// Basic and codec registrations describe the process foundation. They
		// are always delivered to their module, even when a Profile selects a
		// smaller set of business components. A Profile chooses entry modules;
		// it does not make JSON, types, MIME values, status text, or custom
		// codecs disappear from the process.
		if active == nil || isFoundationEntry(entry) || active.matches(entry.Key()) {
			selected = append(selected, entry)
		}
	}
	rt.mu.Lock()
	rt.entries = selected
	rt.mu.Unlock()
	modules := globalModules()
	base := rt.lifecycleContext(ctx)
	for _, module := range modules {
		name := moduleName(module)
		if err := module.Config(config.Clone()); err != nil {
			return fmt.Errorf("dubi: config %q: %w", name, err)
		}
	}
	for _, module := range modules {
		for _, registration := range selected {
			module.Register(registration.Name, registration.Value)
		}
	}
	for _, module := range modules {
		base.Name = moduleName(module)
		if err := module.Setup(base); err != nil {
			return fmt.Errorf("dubi: setup %q: %w", base.Name, err)
		}
	}
	setupOK = true
	rt.finishLifecycle(stateSetup)
	return nil
}

// isFoundationEntry reports whether an entry configures one of the process
// foundation modules. Foundation registrations are independent of the
// business Profile selected for the process.
func isFoundationEntry(entry Entry) bool {
	if entry.Scope == "codec" || entry.Scope == "basic" || strings.HasPrefix(entry.Scope, "basic:") {
		return true
	}
	// Keep the type check as a compatibility guard for foundation values that
	// predate Scope methods, including the short []string MIME form.
	switch entry.Value.(type) {
	case Type, *Type, Language, *Language, Mime, []string,
		Status, *Status, Statuses, *Statuses, Codec, *Codec:
		return true
	default:
		return false
	}
}

// Config returns a copy of the process configuration delivered to modules.
func (rt *Runtime) Config() Map {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.config.Clone()
}

func Config() Map { return DefaultRuntime().Config() }

func asProfile(value any) (Profile, bool) {
	switch profile := value.(type) {
	case Profile:
		return profile, true
	case *Profile:
		if profile != nil {
			return *profile, true
		}
	}
	return Profile{}, false
}

func (rt *Runtime) Open(ctx context.Context) error {
	if err := rt.beginLifecycle("open", stateSetup); err != nil {
		return err
	}
	base := rt.lifecycleContext(ctx)
	modules := globalModules()
	opened := 0
	for _, module := range modules {
		base.Name = moduleName(module)
		if err := module.Open(base); err != nil {
			for j := opened - 1; j >= 0; j-- {
				_ = modules[j].Close(base)
			}
			rt.finishLifecycle(stateSetup)
			return fmt.Errorf("dubi: open %q: %w", base.Name, err)
		}
		opened++
	}
	rt.finishLifecycle(stateOpen)
	return nil
}

// Start starts all mounted process modules and returns immediately. The caller
// owns process-level waiting and can stop the process with End.
func (rt *Runtime) Start(ctx context.Context) error {
	return rt.start(ctx)
}

func (rt *Runtime) start(ctx context.Context) error {
	if err := rt.beginLifecycle("start", stateOpen); err != nil {
		return err
	}
	base := rt.lifecycleContext(ctx)
	modules := globalModules()
	started := 0
	for _, module := range modules {
		base.Name = moduleName(module)
		if err := module.Start(base); err != nil {
			for j := started - 1; j >= 0; j-- {
				_ = modules[j].Stop(base)
			}
			rt.finishLifecycle(stateOpen)
			return fmt.Errorf("dubi: start %q: %w", base.Name, err)
		}
		started++
	}
	// Register the running process before releasing the lifecycle transition.
	// A concurrent End must either wait for this transition to finish or see
	// the active process; it must never observe a started runtime that is absent
	// from the process wait manager.
	runtimeManager.add(rt)
	rt.finishLifecycle(stateStarted)
	return nil
}

func (rt *Runtime) Stop(ctx context.Context) error {
	if err := rt.beginLifecycle("stop", stateStarted); err != nil {
		return err
	}
	base := rt.lifecycleContext(ctx)
	modules := globalModules()
	var first error
	for i := len(modules) - 1; i >= 0; i-- {
		base.Name = moduleName(modules[i])
		if err := modules[i].Stop(base); err != nil && first == nil {
			first = fmt.Errorf("dubi: stop %q: %w", base.Name, err)
		}
	}
	rt.finishLifecycle(stateStopped)
	runtimeManager.remove(rt)
	return first
}

func (rt *Runtime) Close(ctx context.Context) error {
	if err := rt.beginLifecycle("close", stateStopped, stateOpen, stateSetup); err != nil {
		return err
	}
	base := rt.lifecycleContext(ctx)
	modules := globalModules()
	var first error
	for i := len(modules) - 1; i >= 0; i-- {
		base.Name = moduleName(modules[i])
		if err := modules[i].Close(base); err != nil && first == nil {
			first = fmt.Errorf("dubi: close %q: %w", base.Name, err)
		}
	}
	rt.finishLifecycle(stateClosed)
	return first
}

func normalizeScope(scope string) string {
	return strings.Trim(strings.ToLower(scope), ". ")
}

// lifecycleContext gives every module lifecycle callback the same process
// Meta facade. Modules can use ctx.Marshal, ctx.Mapping, ctx.String and
// ctx.Invoke without reaching into Runtime directly.
func (rt *Runtime) lifecycleContext(ctx context.Context) *Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Context{
		Context: ctx,
		Meta:    NewMeta().WithContext(ctx),
	}
}

func moduleName(module Module) string {
	return fmt.Sprintf("%T", module)
}
