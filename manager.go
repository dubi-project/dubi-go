package dubi

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// runtimeManager tracks every Runtime that has been started in this process.
// The package-level Wait function uses it to coordinate one process-wide
// shutdown signal across all runtimes.
type runtimeRegistry struct {
	sync.Mutex
	items map[*Runtime]struct{}
	wake  chan struct{}
}

var runtimeManager = runtimeRegistry{
	items: make(map[*Runtime]struct{}),
	wake:  make(chan struct{}),
}

func (m *runtimeRegistry) add(rt *Runtime) {
	m.Lock()
	m.items[rt] = struct{}{}
	close(m.wake)
	m.wake = make(chan struct{})
	m.Unlock()
}

func (m *runtimeRegistry) remove(rt *Runtime) {
	m.Lock()
	delete(m.items, rt)
	close(m.wake)
	m.wake = make(chan struct{})
	m.Unlock()
}

func (m *runtimeRegistry) snapshot() ([]*Runtime, <-chan struct{}) {
	m.Lock()
	items := make([]*Runtime, 0, len(m.items))
	for rt := range m.items {
		items = append(items, rt)
	}
	wake := m.wake
	m.Unlock()
	return items, wake
}

// Wait blocks until all started runtimes have ended. SIGINT and SIGTERM end
// every active Runtime before Wait returns.
func Wait() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		runtimes, wake := runtimeManager.snapshot()
		if len(runtimes) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			var first error
			for _, rt := range runtimes {
				if err := rt.End(); err != nil && first == nil {
					first = err
				}
			}
			if first != nil {
				return first
			}
		case <-wake:
		}
	}
}
