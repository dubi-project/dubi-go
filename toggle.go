package dubi

import (
	"context"
	. "github.com/dubi-project/dubi-go/base"
	"log"
	"sync"
)

type triggerDispatcher struct {
	mu        sync.Mutex
	methods   map[string][]string
	accepting bool
	active    sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

var triggers triggerDispatcher

// Toggle starts each selected listener in its own goroutine and returns.
func Toggle(name string, values ...Map) { triggers.toggle(name, false, values...) }

// SyncToggle runs each listener in registration order, returning after all of
// them finish. Each callback receives its own Meta and payload snapshot.
func SyncToggle(name string, values ...Map) { triggers.toggle(name, true, values...) }

func (d *triggerDispatcher) toggle(name string, synchronous bool, values ...Map) {
	d.mu.Lock()
	if !d.accepting {
		d.mu.Unlock()
		return
	}
	methods := append([]string(nil), d.methods[name]...)
	ctx := d.ctx
	d.active.Add(len(methods))
	d.mu.Unlock()
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	for _, method := range methods {
		payload := cloneConfigMap(value)
		run := func() { defer d.active.Done(); invokeTrigger(ctx, name, method, payload) }
		if synchronous {
			run()
		} else {
			go run()
		}
	}
}

func invokeTrigger(ctx context.Context, name, method string, value Map) {
	defer func() {
		if value := recover(); value != nil {
			log.Printf("dubi: trigger %q panic: %v", name, value)
		}
	}()
	_, result := DefaultRuntime().Execute(NewMeta().WithContext(ctx), method, value)
	if result != nil && result.Fail() {
		log.Printf("dubi: trigger %q failed: %v", name, result)
	}
}

// shutdown rejects new work before joining callbacks, so WaitGroup.Add cannot
// race with Wait. Runtime uses it before Close/Reset change shared resources.
func (d *triggerDispatcher) shutdown() {
	d.mu.Lock()
	d.accepting = false
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()
	d.active.Wait()
}

func (d *triggerDispatcher) drain() {
	d.mu.Lock()
	wasAccepting := d.accepting
	d.accepting = false
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()
	d.active.Wait()
	d.mu.Lock()
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.accepting = wasAccepting
	d.mu.Unlock()
}
