package empty

// Handler is a no-op handler component used to demonstrate a second
// component type managed by the same module.
type Handler struct {
	Name string
}

func (Handler) Scope() string { return "empty:handler" }

func (m *Module) RegisterHandler(name string, handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlers == nil {
		m.handlers = make(map[string]Handler)
	}
	m.handlers[name] = handler
}
