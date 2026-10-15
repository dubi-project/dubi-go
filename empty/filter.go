package empty

// Filter is a no-op filter component used to demonstrate component
// registration and Scope-based Profile matching.
type Filter struct {
	Name string
}

func (Filter) Scope() string { return "empty:filter" }

func (m *Module) RegisterFilter(name string, filter Filter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.filters == nil {
		m.filters = make(map[string]Filter)
	}
	m.filters[name] = filter
}
