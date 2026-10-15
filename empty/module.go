package empty

import (
	"context"
	"errors"
	"sync"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

var _ dubi.Module = (*Module)(nil)

// Module is the singleton process module owned by this package.
type Module struct {
	mu       sync.RWMutex
	filters  map[string]Filter
	handlers map[string]Handler
	opened   bool
	started  bool
}

func (i *Module) Config(Map) error { return nil }

// Reset clears the singleton module state for test setup.
func (i *Module) Reset() {
	i.mu.Lock()
	i.filters = make(map[string]Filter)
	i.handlers = make(map[string]Handler)
	i.opened = false
	i.started = false
	i.mu.Unlock()
}

func (i *Module) Setup(*dubi.Context) error { return nil }

func (i *Module) Open(*dubi.Context) error {
	i.mu.Lock()
	i.opened = true
	i.mu.Unlock()
	return nil
}

func (i *Module) Start(*dubi.Context) error {
	i.mu.Lock()
	i.started = true
	i.mu.Unlock()
	return nil
}

func (i *Module) Stop(*dubi.Context) error {
	i.mu.Lock()
	i.started = false
	i.mu.Unlock()
	return nil
}

func (i *Module) Close(*dubi.Context) error {
	i.mu.Lock()
	i.opened = false
	i.mu.Unlock()
	return nil
}

// Counts reports the number of components in this process module.
func (i *Module) Counts() (filters, handlers int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.filters), len(i.handlers)
}

// Health demonstrates a module-owned health check.
func (i *Module) Health(context.Context) error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.opened || !i.started {
		return errors.New("empty module is not started")
	}
	return nil
}
