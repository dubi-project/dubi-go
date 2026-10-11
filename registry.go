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
// registrations. A Registry can be shared by multiple Runtime instances.
type Registry struct {
	mu    sync.RWMutex
	items map[string]any
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
	if r.items == nil {
		r.items = make(map[string]any)
	}
	r.items[name] = value
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

// Register adds any registration, including Profile values, to the
// process-wide registry. Profiles are metadata and are separated by Runtime
// when it builds its component entries.
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
