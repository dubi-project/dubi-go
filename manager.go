package dubi

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// runtimeManager tracks the one process runtime. It remains separate from the
// lifecycle implementation so Wait and signal handling stay small.
type runtimeRegistry struct {
	sync.Mutex
	active *Runtime
	wake   chan struct{}
}

var runtimeManager = runtimeRegistry{
	wake: make(chan struct{}),
}

func (m *runtimeRegistry) add(rt *Runtime) {
	m.Lock()
	m.active = rt
	close(m.wake)
	m.wake = make(chan struct{})
	m.Unlock()
}

func (m *runtimeRegistry) remove(rt *Runtime) {
	m.Lock()
	if m.active == rt {
		m.active = nil
	}
	close(m.wake)
	m.wake = make(chan struct{})
	m.Unlock()
}

func (m *runtimeRegistry) snapshot() (*Runtime, <-chan struct{}) {
	m.Lock()
	active := m.active
	wake := m.wake
	m.Unlock()
	return active, wake
}

// Wait blocks until the process runtime ends. SIGINT and SIGTERM end it before
// Wait returns.
func Wait() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		rt, wake := runtimeManager.snapshot()
		if rt == nil {
			return nil
		}
		if transition := rt.lifecycleTransition(); transition != "" {
			return fmt.Errorf("dubi: wait called while %s is running", transition)
		}
		select {
		case <-ctx.Done():
			if err := rt.End(); err != nil {
				return err
			}
			return nil
		case <-wake:
		}
	}
}
