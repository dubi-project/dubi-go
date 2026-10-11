// Package empty is a small example of a dubi-go module package.
// Importing the package mounts its singleton Module and registers its
// components automatically.
package empty

import (
	"sync"

	dubi "github.com/dubi-project/dubi-go"
)

var (
	module = &Module{}
	host   dubi.Host
	last   *Instance
)

// Module is the singleton runtime module definition owned by this package.
type Module struct{}

// New creates isolated state for one dubi Runtime.
func (*Module) New() dubi.Instance {
	instance := &Instance{}
	instance.init()
	mu.Lock()
	last = instance
	mu.Unlock()
	return instance
}

var mu sync.RWMutex

func init() {
	host = dubi.Mount(module)
}

// Host returns the host created when this package was mounted.
func Host() dubi.Host { return host }

// Counts reports the number of registered example components.
func Counts() (filters, handlers int) {
	mu.RLock()
	instance := last
	mu.RUnlock()
	if instance == nil {
		return 0, 0
	}
	return instance.Counts()
}
