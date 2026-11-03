package view

import (
	"fmt"
	"html/template"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

type templateKey struct {
	source  source
	helpers string
}

// Cached templates are never executed and contain no request-bound functions.
// Each render clones one, binds its own functions, then executes the clone.
type engine struct {
	config    Config
	fsys      fs.FS
	helpers   template.FuncMap
	mu        sync.RWMutex
	templates map[templateKey]*template.Template
}

func (e *engine) load(file source, functions template.FuncMap, signature string) (*template.Template, error) {
	key := templateKey{file, signature}
	var parsed *template.Template
	if e.config.Cache {
		e.mu.RLock()
		parsed = e.templates[key]
		e.mu.RUnlock()
	}
	if parsed == nil {
		data, err := e.read(file)
		if err != nil {
			return nil, fmt.Errorf("view: read %s: %w", file.name, err)
		}
		placeholders := template.FuncMap{}
		for name := range functions {
			placeholders[name] = func(...any) (any, error) { return nil, nil }
		}
		parsed, err = template.New(file.name).Delims(e.config.Left, e.config.Right).Funcs(placeholders).Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("view: parse %s: %w", file.name, err)
		}
		if e.config.Cache {
			e.mu.Lock()
			if existing := e.templates[key]; existing != nil {
				parsed = existing
			} else {
				e.templates[key] = parsed
			}
			e.mu.Unlock()
		}
	}
	clone, err := parsed.Clone()
	if err != nil {
		return nil, fmt.Errorf("view: clone %s: %w", file.name, err)
	}
	return clone.Funcs(functions), nil
}

func functionSignature(functions template.FuncMap) string {
	names := make([]string, 0, len(functions))
	for name := range functions {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
}
