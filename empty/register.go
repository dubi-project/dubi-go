package empty

// Register receives selected Registry entries from the process registry.
func (i *Module) Register(name string, value any) {
	switch item := value.(type) {
	case Filter:
		i.RegisterFilter(name, item)
	case Handler:
		i.RegisterHandler(name, item)
	}
}
