package empty

import (
	"context"
	"errors"
	"sync"

	dubi "github.com/dubi-project/dubi-go"
)

var _ dubi.Instance = (*Instance)(nil)

// Instance owns the empty module state for one Runtime.
type Instance struct {
	mu       sync.RWMutex
	filters  map[string]Filter
	handlers map[string]Handler
	opened   bool
	started  bool
}

func (i *Instance) init() {
	i.mu.Lock()
	i.filters = make(map[string]Filter)
	i.handlers = make(map[string]Handler)
	i.mu.Unlock()
}

func (i *Instance) Setup(*dubi.Context) error { return nil }

func (i *Instance) Open(*dubi.Context) error {
	i.mu.Lock()
	i.opened = true
	i.mu.Unlock()
	return nil
}

func (i *Instance) Start(*dubi.Context) error {
	i.mu.Lock()
	i.started = true
	i.mu.Unlock()
	return nil
}

func (i *Instance) Stop(*dubi.Context) error {
	i.mu.Lock()
	i.started = false
	i.mu.Unlock()
	return nil
}

func (i *Instance) Close(*dubi.Context) error {
	i.mu.Lock()
	i.opened = false
	i.mu.Unlock()
	return nil
}

// Counts reports the number of components in this Runtime instance.
func (i *Instance) Counts() (filters, handlers int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.filters), len(i.handlers)
}

// Health demonstrates an instance-owned health check.
func (i *Instance) Health(context.Context) error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if !i.opened || !i.started {
		return errors.New("empty module is not started")
	}
	return nil
}
