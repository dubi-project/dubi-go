package dubi

import (
	"sort"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// Methods names the local operations contained in a Library.
type Methods map[string]Method

// Library is a group of local Methods with a shared name prefix and Settings.
// Like Type and Codec, registered Libraries are available in every Profile.
type Library struct {
	Name     string
	Desc     string
	Settings Map
	Methods  Methods
}

func (Library) Scope() string { return "core:library" }

// LibraryInvoker binds one Library's settings to a request Meta. Create a new
// invoker per request; the invoker and its Meta retain the latest call result.
type LibraryInvoker struct {
	meta     *Meta
	name     string
	settings Map
	result   Res
}

func (c *coreModule) registerLibrary(name string, definition Library) {
	name = normalizeLibraryName(name)
	if name == "" {
		return
	}
	// A failed startup can be retried with a replacement Library definition.
	// Remove the old expansion before installing its new set of members.
	if previous, exists := c.libraries[name]; exists {
		for member := range previous.Methods {
			delete(c.methods, joinLibraryName(name, member))
		}
	}
	if definition.Name == "" {
		definition.Name = name
	}
	definition.Settings = cloneConfigMap(definition.Settings)
	methods := make(Methods, len(definition.Methods))
	names := make([]string, 0, len(definition.Methods))
	for key := range definition.Methods {
		names = append(names, key)
	}
	sort.Strings(names)
	for _, key := range names {
		methodName := normalizeLibraryName(key)
		if methodName == "" {
			continue
		}
		method := definition.Methods[key]
		method.Settings = cloneConfigMap(method.Settings)
		method.Args = method.Args.Clone()
		method.Data = method.Data.Clone()
		methods[methodName] = method
		c.methods[joinLibraryName(name, methodName)] = method
	}
	definition.Methods = methods
	c.libraries[name] = definition
}

// Library creates a local invoker sharing this request's identity, context,
// authentication, language, and result. Settings are merged in order:
// Method defaults, Library defaults, then the supplied overrides.
func (m *Meta) Library(name string, settings ...Map) *LibraryInvoker {
	if m == nil {
		m = NewMeta()
	}
	name = normalizeLibraryName(name)
	values := Map{}
	if definition, ok := core.libraries[name]; ok {
		for key, value := range definition.Settings {
			values[key] = cloneConfigValue(value)
		}
	}
	for _, overlay := range settings {
		for key, value := range overlay {
			values[key] = cloneConfigValue(value)
		}
	}
	return &LibraryInvoker{meta: m, name: name, settings: values}
}

// Invoke calls a local Method under the Library prefix. It never dispatches
// a Service or a remote Bus request. Arguments and returned data use the same
// mapping and dynamic action signatures as regular Method calls.
func (l *LibraryInvoker) Invoke(name string, values ...Map) Map {
	if l == nil {
		return nil
	}
	if l.meta == nil {
		l.meta = NewMeta()
	}
	fullName := joinLibraryName(l.name, normalizeLibraryName(name))
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	var data Map
	if _, exists := core.methods[fullName]; !exists {
		l.result = ResultOf(1, "library method not found: "+fullName)
	} else {
		var result Res
		data, result = DefaultRuntime().invoke(l.meta, fullName, value, true, l.settings)
		l.result = defaultRes(result)
	}
	l.meta.Result(l.result)
	return data
}

// Result returns the last result without clearing it, matching Meta.Result.
// Before the first call, the invoker has the standard successful result.
func (l *LibraryInvoker) Result() Res {
	if l == nil || l.result == nil {
		return OK
	}
	return l.result
}

func (l *LibraryInvoker) InvokeOK(name string, values ...Map) bool {
	if l == nil {
		return false
	}
	l.Invoke(name, values...)
	return l.Result().OK()
}

func (l *LibraryInvoker) InvokeFail(name string, values ...Map) bool {
	return !l.InvokeOK(name, values...)
}

func normalizeLibraryName(name string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(name)), ".")
}

func joinLibraryName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}
