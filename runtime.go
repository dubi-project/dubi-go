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

// Module is the process-level module definition. It is mounted once and
// creates an isolated Instance for each Runtime.
type Module interface {
	New() Instance
}

// Instance owns one Runtime's registrations and lifecycle state.
type Instance interface {
	Register(string, any)
	Config(Map) error
	Setup(*Context) error
	Open(*Context) error
	Start(*Context) error
	Stop(*Context) error
	Close(*Context) error
}

// RuntimeOption configures one Runtime before its lifecycle starts.
type RuntimeOption func(*Runtime)

type Context struct {
	context.Context
	*Meta
	Runtime *Runtime
	Name    string
	Config  any
	Setting Map
	Value   Map
	Args    Map
}

type Runtime struct {
	mu           sync.Mutex
	registry     *Registry
	profiles     map[string]Profile
	identity     Identity
	nodeSet      bool
	profileSet   bool
	allEntries   []Entry
	entries      []Entry
	instances    []Instance
	moduleNames  []string
	core         *coreInstance
	basic        *basicInstance
	codec        *codecInstance
	config       Map
	configLoaded bool
	bootstrap    Bootstrap
	state        lifecycle
}

func NewRuntime(options ...RuntimeOption) *Runtime {
	identity := defaultIdentity()
	rt := &Runtime{
		registry:   NewRegistry(),
		profiles:   make(map[string]Profile),
		config:     Map{},
		identity:   identity,
		bootstrap:  defaultBootstrap(),
		nodeSet:    identity.Node != "",
		profileSet: identity.Profile != "",
	}
	for _, option := range options {
		if option != nil {
			option(rt)
		}
	}
	return rt
}

// New is the concise constructor used by applications.
func New(options ...RuntimeOption) *Runtime { return NewRuntime(options...) }

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

// Auth returns a copy of the connection authentication settings.
func (rt *Runtime) Auth() Map {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.bootstrap.Auth.Clone()
}

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
	runtimeManager.remove(rt)
	return err
}

func (rt *Runtime) shutdown(ctx context.Context) error {
	rt.mu.Lock()
	state := rt.state
	rt.mu.Unlock()

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

// DefaultRuntime returns the process-wide runtime. It is created lazily and
// uses all global registrations unless a profile is selected.
func DefaultRuntime() *Runtime {
	defaultRuntimeMu.Lock()
	defer defaultRuntimeMu.Unlock()
	if defaultRuntime == nil {
		defaultRuntime = NewRuntime()
	}
	return defaultRuntime
}

var (
	defaultRuntimeMu sync.Mutex
	defaultRuntime   *Runtime
)

// Register adds an instance-local component or Profile. It must happen before Setup.
func (rt *Runtime) Register(name string, value any) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateNew {
		panic("dubi: runtime is already prepared")
	}
	if profile, ok := asProfile(value); ok {
		rt.profiles[normalizeName(name)] = profile
		return
	}
	rt.registry.Register(name, value)
}
func (rt *Runtime) Entries() []Entry { return append([]Entry(nil), rt.entries...) }

// AllEntries returns all merged component entries before Profile filtering.
func (rt *Runtime) AllEntries() []Entry { return append([]Entry(nil), rt.allEntries...) }

func (rt *Runtime) Setup(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateNew {
		return fmt.Errorf("dubi: setup called in state %s", rt.state)
	}
	if !rt.configLoaded {
		if err := rt.loadConfig(); err != nil {
			return err
		}
	}
	merged := make(map[string]any)
	profiles := make(map[string]Profile)
	for _, entry := range globalRegistry.Entries() {
		if profile, ok := asProfile(entry.Value); ok {
			profiles[entry.Name] = profile
			continue
		}
		merged[entry.Name] = entry.Value
	}
	for _, entry := range rt.registry.Entries() {
		merged[entry.Name] = entry.Value
	}
	configured, err := configuredProfiles(rt.config)
	if err != nil {
		return fmt.Errorf("dubi: load profiles: %w", err)
	}
	for name, profile := range configured {
		profiles[name] = profile
	}
	for name, profile := range rt.profiles {
		profiles[name] = profile
	}
	all := make([]Entry, 0, len(merged))
	for name, value := range merged {
		all = append(all, Entry{Name: name, Value: value, Scope: registrationScope(value)})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	rt.allEntries = all
	var active *Profile
	if rt.identity.Profile != "" {
		profile, err := resolveProfiles(profiles, rt.identity.Profile)
		if err != nil {
			return err
		}
		active = &profile
	}
	selected := make([]Entry, 0, len(all))
	for _, entry := range all {
		// Basic and codec registrations describe the Runtime's foundation. They
		// are always delivered to their module, even when a Profile selects a
		// smaller set of business components. A Profile chooses entry modules;
		// it does not make JSON, types, MIME values, status text, or custom
		// codecs disappear from the Runtime.
		if active == nil || isFoundationEntry(entry) || active.matches(entry.Key()) {
			selected = append(selected, entry)
		}
	}
	rt.entries = selected
	modules := globalModules()
	instances := make([]Instance, len(modules))
	names := make([]string, len(modules))
	for i, module := range modules {
		instance := module.New()
		if instance == nil {
			return fmt.Errorf("dubi: module %q returned nil instance", moduleName(module))
		}
		instances[i] = instance
		names[i] = moduleName(module)
		if core, ok := instance.(*coreInstance); ok {
			rt.core = core
		}
		if basic, ok := instance.(*basicInstance); ok {
			rt.basic = basic
		}
		if codec, ok := instance.(*codecInstance); ok {
			rt.codec = codec
		}
	}
	if rt.basic != nil {
		rt.basic.codec = rt.codec
	}
	base := rt.lifecycleContext(ctx)
	for i, instance := range instances {
		if err := instance.Config(rt.config.Clone()); err != nil {
			return fmt.Errorf("dubi: config %q: %w", names[i], err)
		}
	}
	for _, instance := range instances {
		for _, registration := range selected {
			instance.Register(registration.Name, registration.Value)
		}
	}
	for i, instance := range instances {
		base.Name = names[i]
		if err := instance.Setup(base); err != nil {
			return fmt.Errorf("dubi: setup %q: %w", base.Name, err)
		}
	}
	rt.instances = instances
	rt.moduleNames = names
	rt.state = stateSetup
	return nil
}

// isFoundationEntry reports whether an entry configures one of the process
// foundation modules. Foundation registrations are scoped per Runtime, but
// are independent of the business Profile selected for that Runtime.
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

// Config returns a copy of the Runtime configuration delivered to modules.
func (rt *Runtime) Config() Map {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.config.Clone()
}

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
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateSetup {
		return fmt.Errorf("dubi: open called in state %s", rt.state)
	}
	base := rt.lifecycleContext(ctx)
	instances := rt.instances
	opened := 0
	for i, instance := range instances {
		base.Name = rt.moduleNames[i]
		if err := instance.Open(base); err != nil {
			for j := opened - 1; j >= 0; j-- {
				_ = instances[j].Close(base)
			}
			return fmt.Errorf("dubi: open %q: %w", base.Name, err)
		}
		opened++
	}
	rt.state = stateOpen
	return nil
}

// Start starts all module instances and returns immediately. The caller owns
// process-level waiting and can stop this Runtime with End.
func (rt *Runtime) Start(ctx context.Context) error {
	if err := rt.start(ctx); err != nil {
		return err
	}
	runtimeManager.add(rt)
	return nil
}

func (rt *Runtime) start(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateOpen {
		return fmt.Errorf("dubi: start called in state %s", rt.state)
	}
	base := rt.lifecycleContext(ctx)
	instances := rt.instances
	started := 0
	for i, instance := range instances {
		base.Name = rt.moduleNames[i]
		if err := instance.Start(base); err != nil {
			for j := started - 1; j >= 0; j-- {
				_ = instances[j].Stop(base)
			}
			return fmt.Errorf("dubi: start %q: %w", base.Name, err)
		}
		started++
	}
	rt.state = stateStarted
	return nil
}

func (rt *Runtime) Stop(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateStarted {
		return fmt.Errorf("dubi: stop called in state %s", rt.state)
	}
	base := rt.lifecycleContext(ctx)
	instances := rt.instances
	var first error
	for i := len(instances) - 1; i >= 0; i-- {
		base.Name = rt.moduleNames[i]
		if err := instances[i].Stop(base); err != nil && first == nil {
			first = fmt.Errorf("dubi: stop %q: %w", base.Name, err)
		}
	}
	rt.state = stateStopped
	return first
}

func (rt *Runtime) Close(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.state != stateStopped && rt.state != stateOpen && rt.state != stateSetup {
		return fmt.Errorf("dubi: close called in state %s", rt.state)
	}
	base := rt.lifecycleContext(ctx)
	instances := rt.instances
	var first error
	for i := len(instances) - 1; i >= 0; i-- {
		base.Name = rt.moduleNames[i]
		if err := instances[i].Close(base); err != nil && first == nil {
			first = fmt.Errorf("dubi: close %q: %w", base.Name, err)
		}
	}
	rt.state = stateClosed
	return first
}

func normalizeScope(scope string) string {
	return strings.Trim(strings.ToLower(scope), ". ")
}

// lifecycleContext gives every module lifecycle callback the same Runtime
// bound Meta facade. Modules can use ctx.Marshal, ctx.Mapping, ctx.String and
// ctx.Invoke without reaching into Runtime directly.
func (rt *Runtime) lifecycleContext(ctx context.Context) *Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Context{
		Context: ctx,
		Meta:    NewMeta(rt).WithContext(ctx),
		Runtime: rt,
	}
}

func moduleName(module Module) string {
	return fmt.Sprintf("%T", module)
}
