package base

// Var describes one dynamic value at a framework boundary. It is deliberately
// transport-neutral so the same definition can be used by HTTP, Method,
// Service, Queue, and Event modules.
type Var struct {
	Name     string
	Desc     string
	Type     string
	Alias    []string
	Default  Any
	Required bool
	Nullable bool
	Encode   string
	Decode   string
	Children Vars
	Valid    func(Any, Var) bool
	Value    func(Any, Var) Any
	Empty    Any
	Error    Any
}

// Vars is a named dynamic parameter definition map.
type Vars map[string]Var

func (v Var) Clone() Var {
	v.Alias = append([]string(nil), v.Alias...)
	v.Children = v.Children.Clone()
	return v
}

func (v Var) IsZero() bool {
	return v.Name == "" && v.Desc == "" && v.Type == "" && len(v.Alias) == 0 &&
		v.Default == nil && !v.Required && !v.Nullable && v.Encode == "" &&
		v.Decode == "" && len(v.Children) == 0 && v.Valid == nil && v.Value == nil &&
		v.Empty == nil && v.Error == nil
}

func (v Vars) Clone() Vars {
	if v == nil {
		return nil
	}
	copyOf := make(Vars, len(v))
	for key, value := range v {
		copyOf[key] = value.Clone()
	}
	return copyOf
}
