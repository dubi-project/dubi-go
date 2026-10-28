package dubi

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Entry is one named object in a Registry. The value is intentionally
// dynamic: its concrete type describes how the runtime should use it.
type Entry struct {
	Name  string
	Value any
	Scope string
}

// Key is the profile-matching name. Registration scopes are supplied by the
// registered value itself; ordinary registrations keep their original name.
func (e Entry) Key() string {
	if e.Scope == "" {
		return e.Name
	}
	return e.Scope + ":" + e.Name
}

// Registry stores modules, services, routes, profiles, and application
// registrations. The package-level Registry is the process registry.
type Registry struct {
	mu       sync.RWMutex
	items    map[registryKey]any
	latest   map[string]registryKey
	triggers map[string][]any
	frozen   bool
}

type registryKey struct{ scope, name string }

func NewRegistry() *Registry {
	return &Registry{items: make(map[registryKey]any), latest: make(map[string]registryKey)}
}

// Register adds or replaces an ordinary registration with the same scope and
// name. Registrations with different scopes coexist. Trigger registrations
// append under their logical name so independent packages can listen together.
func (r *Registry) Register(name string, value any) {
	if r == nil {
		panic("dubi: nil registry")
	}
	name = normalizeName(name)
	if value == nil {
		panic("dubi: cannot register nil value")
	}
	if trigger, ok := value.(*Trigger); ok && trigger == nil {
		panic("dubi: cannot register nil trigger")
	}
	key := registryKey{scope: registrationScope(value), name: name}
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		panic("dubi: registry is frozen")
	}
	if r.items == nil {
		r.items = make(map[registryKey]any)
	}
	if r.latest == nil {
		r.latest = make(map[string]registryKey)
	}
	if isTrigger(value) {
		if r.triggers == nil {
			r.triggers = make(map[string][]any)
		}
		r.triggers[name] = append(r.triggers[name], value)
	} else {
		r.items[key] = value
		r.latest[name] = key
	}
	r.mu.Unlock()
}

// Freeze prevents registrations from changing after process setup has begun.
// A frozen registry remains readable and can be replaced by the process reset
// path used by embedded hosts and tests.
func (r *Registry) Freeze() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.frozen = true
	r.mu.Unlock()
}

// Frozen reports whether this registry accepts new registrations.
func (r *Registry) Frozen() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	frozen := r.frozen
	r.mu.RUnlock()
	return frozen
}

func (r *Registry) unfreeze() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.frozen = false
	r.mu.Unlock()
}

// Lookup returns the latest ordinary registration with this logical name,
// or the latest Trigger if there is no ordinary registration. Use LookupScope
// when a name may belong to more than one component.
func (r *Registry) Lookup(name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	key, ok := r.latest[name]
	var v any
	if ok {
		v, ok = r.items[key]
	}
	if !ok {
		if listeners := r.triggers[name]; len(listeners) > 0 {
			v, ok = listeners[len(listeners)-1], true
		}
	}
	r.mu.RUnlock()
	return v, ok
}

// LookupScope returns the registration for one component scope and logical
// name. For Triggers it returns the latest listener; Entries returns all of them.
func (r *Registry) LookupScope(scope, name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	scope = normalizeScope(scope)
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.items[registryKey{scope: scope, name: name}]
	if !ok && scope == "core:trigger" {
		if listeners := r.triggers[name]; len(listeners) > 0 {
			return listeners[len(listeners)-1], true
		}
	}
	return v, ok
}

func (r *Registry) Entries() []Entry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	entries := make([]Entry, 0, len(r.items))
	for key, value := range r.items {
		entries = append(entries, Entry{Name: key.name, Value: value, Scope: key.scope})
	}
	for name, listeners := range r.triggers {
		for _, value := range listeners {
			entries = append(entries, Entry{Name: name, Value: value, Scope: registrationScope(value)})
		}
	}
	r.mu.RUnlock()
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Scope < entries[j].Scope
	})
	return entries
}

type scoped interface {
	Scope() string
}

func registrationScope(value any) string {
	if item, ok := value.(scoped); ok {
		return normalizeScope(item.Scope())
	}
	return ""
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("dubi: registration name is empty")
	}
	return name
}

var globalRegistry = NewRegistry()

// RegistrationSet expands a collection into individual named registrations.
type RegistrationSet interface {
	Registrations() map[string]any
}

// Register adds any registration, including Profile values, to the process
// registry. A RegistrationSet can be passed alone to register its entries.
// Profiles are metadata and are selected by Run.
func Register(nameOrSet any, values ...any) {
	if len(values) == 0 {
		if set, ok := nameOrSet.(RegistrationSet); ok {
			entries := set.Registrations()
			names := make([]string, 0, len(entries))
			for name := range entries {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				globalRegistry.Register(name, entries[name])
			}
			return
		}
		panic("dubi: Register requires a name and value or a registration set")
	}
	name, ok := nameOrSet.(string)
	if !ok || len(values) != 1 {
		panic("dubi: Register requires a name and value or a registration set")
	}
	globalRegistry.Register(name, values[0])
}

func Lookup(name string) (any, bool) { return globalRegistry.Lookup(name) }

// LookupScope finds a registration by component scope and logical name.
func LookupScope(scope, name string) (any, bool) { return globalRegistry.LookupScope(scope, name) }

func (r *Registry) String() string {
	if r == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Registry(%d entries)", len(r.Entries()))
}
