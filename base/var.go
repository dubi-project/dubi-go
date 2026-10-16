package base

// Var describes one dynamic value at a framework boundary. It is deliberately
// transport-neutral so the same definition can be used by HTTP, Method,
// Service, Queue, and Event modules.
type Var struct {
	nilValue  bool
	Name      string
	Desc      string
	Type      string
	Alias     []string
	Default   Any
	Unique    bool
	Check     string
	Collation string
	Comment   string
	Setting   Map
	Options   Map
	Required  bool
	Nullable  bool
	Encode    string
	Decode    string
	Children  Vars
	Valid     func(Any, Var) bool
	Value     func(Any, Var) Any
	Empty     Any
	Error     Any
}

// Vars is a named dynamic parameter definition map.
type Vars map[string]Var

func (v Var) Clone() Var {
	v.Alias = append([]string(nil), v.Alias...)
	v.Setting = v.Setting.Clone()
	v.Options = v.Options.Clone()
	v.Children = v.Children.Clone()
	return v
}

func (v Var) IsZero() bool {
	return !v.nilValue && v.Name == "" && v.Desc == "" && v.Type == "" && len(v.Alias) == 0 &&
		v.Default == nil && !v.Unique && v.Check == "" && v.Collation == "" && v.Comment == "" &&
		len(v.Setting) == 0 && len(v.Options) == 0 && !v.Required && !v.Nullable && v.Encode == "" &&
		v.Decode == "" && len(v.Children) == 0 && v.Valid == nil && v.Value == nil &&
		v.Empty == nil && v.Error == nil
}

// Nil is the dynamic schema marker returned when a field cannot be found.
var Nil = Var{nilValue: true}

func (v Var) Nil() bool { return v.nilValue }

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
