package dubi

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

type memoryBusOptions struct{ workers, capacity int }

func parseMemoryBusOptions(config Map) (memoryBusOptions, error) {
	options := memoryBusOptions{workers: runtime.GOMAXPROCS(0), capacity: 1024}
	section, exists := config["bus"]
	if !exists {
		return options, nil
	}
	busConfig, ok := section.(Map)
	if !ok {
		return options, fmt.Errorf("dubi: bus must be an object")
	}
	raw, exists := busConfig["options"]
	if !exists {
		return options, nil
	}
	settings, ok := raw.(Map)
	if !ok {
		return options, fmt.Errorf("dubi: bus.options must be an object")
	}
	for name, value := range settings {
		n, err := strconv.Atoi(fmt.Sprint(value))
		if err != nil || n < 1 {
			return options, fmt.Errorf("dubi: bus.options.%s must be a positive integer", name)
		}
		switch name {
		case "workers":
			options.workers = n
		case "capacity":
			options.capacity = n
		default:
			return options, fmt.Errorf("dubi: unknown bus option %q", name)
		}
	}
	return options, nil
}

type memoryBus struct {
	mu            sync.Mutex
	subscriptions map[string][]ServiceInfo
	cursors       map[string]uint64
	ctx           context.Context
	ready         bool
	active        sync.WaitGroup
	workers       chan struct{}
	capacity      int
	pending       int
	failures      []BusFailure
}

func newMemoryBus(services []ServiceInfo, options memoryBusOptions) *memoryBus {
	b := &memoryBus{
		subscriptions: make(map[string][]ServiceInfo), cursors: make(map[string]uint64),
		workers: make(chan struct{}, options.workers), capacity: options.capacity,
	}
	for _, service := range services {
		b.subscriptions[service.Subject] = append(b.subscriptions[service.Subject], service)
	}
	return b
}

func (b *memoryBus) start(ctx context.Context) {
	b.mu.Lock()
	b.ctx, b.ready = ctx, true
	b.mu.Unlock()
}

func (b *memoryBus) stop() {
	b.mu.Lock()
	b.ready = false
	b.mu.Unlock()
	b.active.Wait()
}

func (b *memoryBus) choose(key string, services []ServiceInfo) ServiceInfo {
	index := b.cursors[key] % uint64(len(services))
	b.cursors[key]++
	return services[index]
}

func (b *memoryBus) Request(meta *Meta, subject string, value Map) (Map, Res) {
	b.mu.Lock()
	if !b.ready || b.ctx.Err() != nil {
		b.mu.Unlock()
		return nil, ErrorResult(ErrBusNotReady)
	}
	services := b.subscriptions[subject]
	if len(services) == 0 {
		b.mu.Unlock()
		return nil, ErrorResult(fmt.Errorf("%w: %s", ErrBusNoService, subject))
	}
	service := b.choose("request:"+subject, services)
	b.mu.Unlock()
	if err := meta.Context().Err(); err != nil {
		return nil, ErrorResult(err)
	}
	return invokeBusService(meta, service.Name, value)
}

func (b *memoryBus) Broadcast(meta *Meta, subject string, value Map) error {
	return b.submit(meta, "broadcast", subject, value)
}
func (b *memoryBus) Dispatch(meta *Meta, subject string, value Map) error {
	return b.submit(meta, "dispatch", subject, value)
}
func (b *memoryBus) Enqueue(meta *Meta, subject string, value Map) error {
	return b.submit(meta, "enqueue", subject, value)
}

func (b *memoryBus) submit(meta *Meta, mode, subject string, value Map) error {
	if err := meta.Context().Err(); err != nil {
		return err
	}
	b.mu.Lock()
	if !b.ready || b.ctx.Err() != nil {
		b.mu.Unlock()
		return ErrBusNotReady
	}
	services := b.subscriptions[subject]
	if len(services) == 0 {
		b.mu.Unlock()
		if mode == "enqueue" {
			return fmt.Errorf("%w: %s", ErrBusNoService, subject)
		}
		return nil // Fire-and-forget notifications without subscribers are dropped.
	}
	var selected []ServiceInfo
	switch mode {
	case "broadcast":
		selected = services
	case "enqueue":
		selected = []ServiceInfo{b.choose("enqueue:"+subject, services)}
	case "dispatch":
		groups := make(map[string][]ServiceInfo)
		for _, service := range services {
			groups[service.Group] = append(groups[service.Group], service)
		}
		names := make([]string, 0, len(groups))
		for group := range groups {
			names = append(names, group)
		}
		sort.Strings(names)
		for _, group := range names {
			// A length prefix prevents ambiguous subject/group cursor keys.
			key := fmt.Sprintf("dispatch:%d:%s%s", len(subject), subject, group)
			selected = append(selected, b.choose(key, groups[group]))
		}
	}
	if len(selected) > b.capacity-b.pending {
		b.mu.Unlock()
		return ErrBusFull // Fan-out admission is atomic; no partial send.
	}
	b.pending += len(selected)
	b.active.Add(len(selected))
	lifetime := b.ctx
	b.mu.Unlock()
	for _, service := range selected {
		// Detach from the entry request lifetime, including pooled Web Meta.
		snapshot := copyBusMeta(meta, lifetime, true)
		payload := cloneConfigMap(value)
		go b.run(snapshot, service, payload, mode == "enqueue")
	}
	return nil
}

func (b *memoryBus) run(meta *Meta, service ServiceInfo, payload Map, queue bool) {
	defer func() {
		b.mu.Lock()
		b.pending--
		b.mu.Unlock()
		b.active.Done()
	}()
	for attempt := 1; ; attempt++ {
		if meta.Context().Err() != nil {
			return
		}
		select {
		case <-meta.Context().Done():
			return
		case b.workers <- struct{}{}:
		}
		if meta.Context().Err() != nil {
			<-b.workers
			return
		}
		// A fresh Meta and payload on every attempt keep mutations from leaking
		// across retries; the immutable task ID remains the same.
		invocation := copyBusMeta(meta, meta.Context(), false)
		if queue {
			invocation.attempt = attempt
		}
		_, result := invokeBusService(invocation, service.Name, cloneConfigMap(payload))
		<-b.workers
		if result == nil || result.OK() {
			return
		}
		if meta.Context().Err() != nil {
			return
		}
		if !queue {
			log.Printf("dubi: BUS notification %q -> %q failed: %v", service.Subject, service.Name, result)
			return
		}
		if attempt > len(service.Retry) {
			b.recordFailure(BusFailure{ID: meta.Id(), Subject: service.Subject, Service: service.Name,
				Attempts: attempt, Value: payload, Error: result.Error(), Time: time.Now()})
			return
		}
		timer := time.NewTimer(service.Retry[attempt-1])
		select {
		case <-meta.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func invokeBusService(meta *Meta, name string, value Map) (Map, Res) {
	data, result, _ := (moduleHost{}).InvokeLocalService(meta, name, value)
	return data, result
}

func (b *memoryBus) recordFailure(failure BusFailure) {
	log.Printf("dubi: BUS task %s (%s -> %s) failed after %d attempts: %s", failure.ID, failure.Subject, failure.Service, failure.Attempts, failure.Error)
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.failures) == b.capacity {
		copy(b.failures, b.failures[1:])
		b.failures[len(b.failures)-1] = failure
	} else {
		b.failures = append(b.failures, failure)
	}
}

func (b *memoryBus) Failures() []BusFailure {
	b.mu.Lock()
	defer b.mu.Unlock()
	failures := append([]BusFailure(nil), b.failures...)
	for i := range failures {
		failures[i].Value = cloneConfigMap(failures[i].Value)
	}
	return failures
}
