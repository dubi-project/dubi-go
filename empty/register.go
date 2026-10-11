package empty

// Register receives selected Registry entries from the dubi Runtime.
func (m *Instance) Register(name string, value any) {
	switch item := value.(type) {
	case Filter:
		m.RegisterFilter(name, item)
	case Handler:
		m.RegisterHandler(name, item)
	}
}
