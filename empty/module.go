package empty

import (
	"sync"

	dubi "github.com/dubi-project/dubi-go"
)

var _ dubi.Module = (*Module)(nil)

var (
	last *Instance
	mu   sync.RWMutex
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
