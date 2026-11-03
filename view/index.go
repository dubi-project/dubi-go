// Package view renders HTML pages, layouts, and partials using html/template.
package view

import (
	"fmt"

	dubi "github.com/dubi-project/dubi-go"
)

var (
	module = &Module{config: defaultConfig(), helpers: make(map[string]Helper)}
	host   dubi.Host
)

func init() { host = dubi.Mount(module) }

// Parse renders one page after the process Setup phase, with or without Web.
// Rendering is buffered: an error never returns a partially rendered page.
func Parse(body Body) (string, error) {
	module.mu.RLock()
	engine := module.engine
	module.mu.RUnlock()
	if engine == nil {
		return "", fmt.Errorf("view: module is not set up")
	}
	return engine.render(body)
}
