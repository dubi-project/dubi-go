package dubi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

var (
	ErrBusNotReady          = errors.New("dubi: BUS is not running")
	ErrBusNoService         = errors.New("dubi: BUS service not found")
	ErrBusFull              = errors.New("dubi: memory BUS capacity exceeded")
	ErrBusServerUnavailable = errors.New("dubi: Dubi Server BUS transport is not implemented; mount a BusHook to connect an external BUS")
)

// BusHook is implemented by a mounted Module to replace the default BUS.
// Request/Broadcast/Dispatch/Enqueue address service registration names.
// Notify addresses registered Notification names. Async methods acknowledge submission, not
// business execution. Implementations must snapshot inputs before returning,
// honor Meta.Context for admission/Request, and join consumers during Stop.
// A module uses Host.Services in Setup and Host.InvokeLocalService on delivery.
type BusHook interface {
	Request(*Meta, string, Map) (Map, Res)
	Broadcast(*Meta, string, Map) error
	Dispatch(*Meta, string, Map) error
	Enqueue(*Meta, string, Map) error
	Notify(*Meta, string, Map) error
}

// ServiceInfo describes one loaded subscription, not its business Action.
type ServiceInfo struct {
	Name    string
	Listens []string
	Group   string
	Retry   []time.Duration
}

// BusDelivery lets a transport restore a request/task identity and queue attempt
// at the Host boundary without exposing writable Meta identity fields.
type BusDelivery struct {
	ID      string
	Attempt int
}

// BusFailure is an exhausted memory task. Memory failures, like memory tasks,
// do not survive process exit. Error is a captured result message.
type BusFailure struct {
	ID       string
	Service  string
	Attempts int
	Value    Map
	Error    string
	Time     time.Time
}

// BusFailures returns retained failures if the selected implementation exposes
// Failures() []BusFailure. The memory BUS retains up to its configured capacity.
func BusFailures() []BusFailure {
	bus.mu.Lock()
	hook := bus.hook
	bus.mu.Unlock()
	if source, ok := hook.(interface{ Failures() []BusFailure }); ok {
		return source.Failures()
	}
	return nil
}

func serviceGroup(group string, identity Identity) string {
	for _, value := range []string{group, identity.Role, identity.Tenant, "dubi"} {
		if value != "" {
			return value
		}
	}
	return "dubi"
}

func serviceCatalog() []ServiceInfo {
	items := make([]ServiceInfo, 0, len(core.services))
	for name, service := range core.services {
		items = append(items, ServiceInfo{Name: name, Listens: append([]string(nil), service.Listens...), Group: service.Group, Retry: append([]time.Duration(nil), service.Retry...)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

type busController struct {
	mu     sync.Mutex
	hook   BusHook
	memory *memoryBus
	ctx    context.Context
	cancel context.CancelFunc
	ready  bool
	active sync.WaitGroup
}

var bus busController

// prepare runs after registration and before module Setup. Only the selected
// implementation is allocated. Server endpoints must never silently fall back.
func (b *busController) prepare(rt *Runtime, modules []Module) error {
	b.stop()
	for _, item := range serviceCatalog() {
		for _, delay := range item.Retry {
			if delay < 0 {
				return fmt.Errorf("dubi: service %q has a negative retry delay", item.Name)
			}
		}
	}
	var hook BusHook
	for _, module := range modules {
		if candidate, ok := module.(BusHook); ok {
			if hook != nil {
				return errors.New("dubi: multiple BUS hooks mounted")
			}
			hook = candidate
		}
	}
	var memory *memoryBus
	if hook == nil {
		if len(rt.Endpoints()) > 0 {
			return ErrBusServerUnavailable
		}
		options, err := parseMemoryBusOptions(rt.Config())
		if err != nil {
			return err
		}
		memory = newMemoryBus(serviceCatalog(), options)
		hook = memory
	}
	b.mu.Lock()
	b.hook, b.memory = hook, memory
	b.mu.Unlock()
	return nil
}

func (b *busController) start() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ctx, b.cancel = context.WithCancel(context.Background())
	if b.memory != nil {
		b.memory.start(b.ctx)
	}
	b.ready = true
}

// stop rejects admissions, cancels requests, and joins memory jobs before
// any module resources or the service catalog can be closed/reset.
func (b *busController) stop() {
	b.mu.Lock()
	b.ready = false
	if b.cancel != nil {
		b.cancel()
	}
	memory := b.memory
	b.mu.Unlock()
	b.active.Wait()
	if memory != nil {
		memory.stop()
	}
}

func (b *busController) acquire(meta *Meta) (BusHook, *Meta, func(), error) {
	b.mu.Lock()
	if !b.ready {
		b.mu.Unlock()
		return nil, nil, nil, ErrBusNotReady
	}
	hook, lifetime := b.hook, b.ctx
	b.active.Add(1)
	b.mu.Unlock()
	ctx, cancel := context.WithCancel(meta.Context())
	stop := context.AfterFunc(lifetime, cancel)
	if err := lifetime.Err(); err != nil {
		cancel()
	}
	done := func() { stop(); cancel(); b.active.Done() }
	if err := ctx.Err(); err != nil {
		done()
		return nil, nil, nil, err
	}
	return hook, copyBusMeta(meta, ctx, false), done, nil
}

// Request always uses the selected BUS, even if a local Method has this name.
func Request(name string, values ...Map) (Map, Res) {
	return requestBus(nil, name, busValue(values))
}

func requestBus(meta *Meta, name string, value Map) (Map, Res) {
	hook, snapshot, done, err := bus.acquire(meta)
	if err != nil {
		return nil, ErrorResult(err)
	}
	defer done()
	return hook.Request(snapshot, name, cloneConfigMap(value))
}

func (m *Meta) Request(name string, values ...Map) Map {
	data, result := requestBus(m, name, busValue(values))
	m.result = result
	return data
}

func Broadcast(name string, values ...Map) error {
	return sendBus(nil, "broadcast", name, busValue(values))
}
func Dispatch(name string, values ...Map) error {
	return sendBus(nil, "dispatch", name, busValue(values))
}
func Enqueue(name string, values ...Map) error {
	return sendBus(nil, "enqueue", name, busValue(values))
}

func (m *Meta) Broadcast(name string, values ...Map) error {
	return sendBus(m, "broadcast", name, busValue(values))
}
func (m *Meta) Dispatch(name string, values ...Map) error {
	return sendBus(m, "dispatch", name, busValue(values))
}
func (m *Meta) Enqueue(name string, values ...Map) error {
	return sendBus(m, "enqueue", name, busValue(values))
}

func sendBus(meta *Meta, mode, name string, value Map) error {
	hook, snapshot, done, err := bus.acquire(meta)
	if err != nil {
		return err
	}
	defer done()
	value = cloneConfigMap(value)
	switch mode {
	case "broadcast":
		return hook.Broadcast(snapshot, name, value)
	case "dispatch":
		return hook.Dispatch(snapshot, name, value)
	default:
		return hook.Enqueue(snapshot, name, value)
	}
}

func busValue(values []Map) Map {
	if len(values) > 0 {
		return values[0]
	}
	return nil
}

// Attempt is 1-based for Enqueue execution, and zero for other calls.
func (m *Meta) Attempt() int {
	if m == nil {
		return 0
	}
	return m.attempt
}

func copyBusMeta(meta *Meta, ctx context.Context, newID bool) *Meta {
	if meta == nil {
		return NewMeta().WithContext(ctx)
	}
	copy := *meta
	copy.ctx, copy.result = ctx, nil
	copy.tokenData.Payload = cloneConfigMap(meta.tokenData.Payload)
	if newID || copy.id == "" {
		copy.id = newRequestID()
	}
	copy.attempt = 0
	return &copy
}
