package dubi

import (
	"context"
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
// Wait returns. If startup or shutdown is in progress, Wait follows the
// lifecycle transition instead of reporting a transient state error.
func Wait() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		rt, wake := runtimeManager.snapshot()
		if rt == nil {
			return nil
		}
		// A Runtime may be visible in the manager before Run has completed its
		// setup. Wait for that transition to publish either a started runtime or
		// a removed runtime instead of returning a transient error.
		if transition := rt.lifecycleTransition(); transition != "" {
			select {
			case <-ctx.Done():
				// A lifecycle callback may be blocked and cannot be safely
				// interrupted by End while its transition lock is held.
				return ctx.Err()
			case <-wake:
				continue
			}
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
