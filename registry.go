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
	mu     sync.RWMutex
	items  map[string]any
	frozen bool
}

func NewRegistry() *Registry {
	return &Registry{items: make(map[string]any)}
}

// Register adds or replaces a named registration. Replacement is deliberate:
// it permits an application or test runtime to override a built-in entry.
func (r *Registry) Register(name string, value any) {
	if r == nil {
		panic("dubi: nil registry")
	}
	name = normalizeName(name)
	if value == nil {
		panic("dubi: cannot register nil value")
	}
	r.mu.Lock()
	if r.frozen {
		r.mu.Unlock()
		panic("dubi: registry is frozen")
	}
	if r.items == nil {
		r.items = make(map[string]any)
	}
	r.items[name] = value
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

func (r *Registry) Lookup(name string) (any, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	v, ok := r.items[name]
	r.mu.RUnlock()
	return v, ok
}

func (r *Registry) Entries() []Entry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	entries := make([]Entry, 0, len(r.items))
	for name, value := range r.items {
		entries = append(entries, Entry{Name: name, Value: value, Scope: registrationScope(value)})
	}
	r.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
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

// Register adds any registration, including Profile values, to the process
// registry. Profiles are metadata and are selected by Run.
func Register(name string, value any) {
	globalRegistry.Register(name, value)
}

func Lookup(name string) (any, bool) { return globalRegistry.Lookup(name) }

func (r *Registry) String() string {
	if r == nil {
		return "<nil>"
	}
	return fmt.Sprintf("Registry(%d entries)", len(r.Entries()))
}
