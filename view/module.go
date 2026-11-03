package view

import (
	"fmt"
	"html/template"
	"reflect"
	"sort"
	"sync"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

// Module owns process-wide template configuration and helper registrations.
type Module struct {
	mu      sync.RWMutex
	config  Config
	helpers map[string]Helper
	engine  *engine
}

func (m *Module) Register(name string, value any) {
	switch value := value.(type) {
	case Helper:
		m.registerHelper(name, value)
	case *Helper:
		if value != nil {
			m.registerHelper(name, *value)
		}
	}
}

func (m *Module) registerHelper(name string, helper Helper) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.helpers == nil {
		m.helpers = make(map[string]Helper)
	}
	helper.Alias = append([]string(nil), helper.Alias...)
	m.helpers[name] = helper
}

func (m *Module) Config(global Map) error {
	config, err := parseConfig(global)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}

func (m *Module) Setup(*dubi.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	functions := template.FuncMap{}
	names := make([]string, 0, len(m.helpers))
	for name := range m.helpers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		helper := m.helpers[name]
		for _, key := range append([]string{name}, helper.Alias...) {
			if _, reserved := builtinNames[key]; reserved {
				return fmt.Errorf("view: helper %q is reserved", key)
			}
			functions[key] = helper.Action
		}
	}
	if err := validateFunctions(functions); err != nil {
		return err
	}
	m.engine = &engine{config: m.config, fsys: dubi.AssetFS(), helpers: functions, templates: make(map[templateKey]*template.Template)}
	return nil
}

func (*Module) Open(*dubi.Context) error  { return nil }
func (*Module) Start(*dubi.Context) error { return nil }
func (*Module) Stop(*dubi.Context) error  { return nil }
func (m *Module) Close(*dubi.Context) error {
	m.mu.Lock()
	m.engine = nil
	m.mu.Unlock()
	return nil
}

func (m *Module) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = defaultConfig()
	m.helpers = make(map[string]Helper)
	m.engine = nil
}

func validateFunctions(functions template.FuncMap) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("view: invalid helper: %v", failure)
		}
	}()
	for name, action := range functions {
		value := reflect.ValueOf(action)
		if !value.IsValid() || value.Kind() != reflect.Func || value.IsNil() {
			return fmt.Errorf("view: helper %q must be a non-nil function", name)
		}
	}
	template.New("validation").Funcs(functions)
	return nil
}
