package dubi

import (
	"fmt"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

// Method is a local business operation. Action uses the dynamic Context and
// may return Map, (Map, Res), (Map, error), any, (any, Res), (any, error),
// error, Res, or no value.
type Method struct {
	Name     string
	Desc     string
	Nullable bool
	Args     Vars
	Data     Vars
	Action   any
	Settings Map
}

func (Method) Scope() string { return "core:method" }

// Service is a business operation callable locally by registration name or
// through BUS by Subject. The default BUS is process-local and in-memory.
type Service struct {
	Name string
	Desc string
	// Subject is the BUS subscription name. Empty uses the registration name.
	// Multiple services may share a subject without sharing a registration key.
	Subject string
	// Group controls Dispatch. Empty falls back to Role, Tenant, then "dubi".
	Group string
	// Retry contains the delays before each additional Enqueue attempt.
	Retry    []time.Duration
	Nullable bool
	Args     Vars
	Data     Vars
	Action   any
	Settings Map
}

func (Service) Scope() string { return "core:service" }

type coreModule struct {
	methods         map[string]Method
	services        map[string]Service
	libraries       map[string]Library
	triggers        map[string][]Trigger
	triggerMethods  []string
	triggerSequence uint64
}

var core = &coreModule{
	methods:   make(map[string]Method),
	services:  make(map[string]Service),
	libraries: make(map[string]Library),
	triggers:  make(map[string][]Trigger),
}

func init() {
	Mount(core)
}

func (c *coreModule) Register(name string, value any) {
	switch item := value.(type) {
	case Method:
		c.methods[name] = item
	case *Method:
		if item != nil {
			c.methods[name] = *item
		}
	case Service:
		c.registerService(name, item)
	case *Service:
		if item != nil {
			c.registerService(name, *item)
		}
	case Library:
		c.registerLibrary(name, item)
	case *Library:
		if item != nil {
			c.registerLibrary(name, *item)
		}
	case Trigger:
		c.registerTrigger(name, item)
	case *Trigger:
		if item != nil {
			c.registerTrigger(name, *item)
		}
	}
}

func (c *coreModule) registerService(name string, service Service) {
	if service.Subject == "" {
		service.Subject = name
	}
	service.Group = serviceGroup(service.Group, DefaultRuntime().Identity())
	service.Retry = append([]time.Duration(nil), service.Retry...)
	service.Settings = cloneConfigMap(service.Settings)
	service.Args = cloneTriggerVars(service.Args, false)
	service.Data = cloneTriggerVars(service.Data, false)
	c.services[name] = service
}

func (c *coreModule) Reset() {
	triggers.shutdown()
	triggers.mu.Lock()
	triggers.methods, triggers.ctx, triggers.cancel = nil, nil, nil
	triggers.mu.Unlock()
	c.methods = make(map[string]Method)
	c.services = make(map[string]Service)
	c.libraries = make(map[string]Library)
	c.triggers = make(map[string][]Trigger)
	c.triggerMethods = nil
	c.triggerSequence = 0
}

func (c *coreModule) Config(Map) error {
	triggers.shutdown()
	c.methods = make(map[string]Method)
	c.services = make(map[string]Service)
	c.libraries = make(map[string]Library)
	c.triggerMethods = nil
	c.triggers = make(map[string][]Trigger)
	return nil
}
func (c *coreModule) Setup(*Context) error { c.setupTriggers(); return nil }
func (*coreModule) Open(*Context) error    { return nil }
func (*coreModule) Start(*Context) error   { return nil }
func (*coreModule) Stop(*Context) error    { return nil }
func (*coreModule) Close(*Context) error   { return nil }

// Invoke calls a local Method or Service, falling back to BUS Request if absent.
func (rt *Runtime) Invoke(meta *Meta, name string, value Map) (Map, Res) {
	if _, ok := core.methods[name]; !ok {
		if _, ok := core.services[name]; !ok {
			return requestBus(meta, name, value)
		}
	}
	return rt.invoke(meta, name, value, false)
}

// Execute calls a local Method only.
func (rt *Runtime) Execute(meta *Meta, name string, value Map) (Map, Res) {
	return rt.invoke(meta, name, value, true)
}

// Invoke calls a Method or Service on the default Runtime.
func Invoke(name string, values ...Map) (Map, Res) {
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	return DefaultRuntime().Invoke(nil, name, value)
}

// Execute calls a Method on the default Runtime.
func Execute(name string, values ...Map) (Map, Res) {
	var value Map
	if len(values) > 0 {
		value = values[0]
	}
	return DefaultRuntime().Execute(nil, name, value)
}

func invokeRuntime(meta *Meta, name string, value Map, methodOnly bool) (Map, Res) {
	rt := DefaultRuntime()
	if methodOnly {
		return rt.invoke(meta, name, value, true)
	}
	return rt.Invoke(meta, name, value)
}

func (rt *Runtime) invoke(meta *Meta, name string, value Map, methodOnly bool, overrides ...Map) (Map, Res) {
	if meta == nil {
		meta = NewMeta()
	}
	if rt == nil || core == nil {
		return nil, ResultOf(1, "runtime is not ready")
	}

	var action Method
	var settings Map
	var dataDefinition Vars
	var config any
	if method, ok := core.methods[name]; ok {
		action = method
		settings = method.Settings
		dataDefinition = method.Data
		config = method
	} else if !methodOnly {
		if service, ok := core.services[name]; ok {
			action = Method{
				Name:     service.Name,
				Desc:     service.Desc,
				Nullable: service.Nullable,
				Args:     service.Args,
				Data:     service.Data,
				Action:   service.Action,
				Settings: service.Settings,
			}
			settings = service.Settings
			dataDefinition = service.Data
			config = service
		} else {
			return nil, ResultOf(1, "operation not found: "+name)
		}
	} else {
		return nil, ResultOf(1, "method not found: "+name)
	}
	return rt.invokeDefinition(meta, name, value, action, settings, dataDefinition, config, overrides...)
}

// invokeLocalService bypasses Method lookup and BUS routing, including when a
// Method and Service share the same registration name.
func (rt *Runtime) invokeLocalService(meta *Meta, name string, value Map) (Map, Res, bool) {
	service, ok := core.services[name]
	if !ok {
		return nil, ErrorResult(fmt.Errorf("%w: %s", ErrBusNoService, name)), false
	}
	if meta == nil {
		meta = NewMeta()
	}
	action := Method{Action: service.Action, Args: service.Args}
	data, result := rt.invokeDefinition(meta, name, value, action, service.Settings, service.Data, service)
	return data, result, true
}

func (rt *Runtime) invokeDefinition(meta *Meta, name string, value Map, action Method, settings Map, dataDefinition Vars, config any, overrides ...Map) (Map, Res) {
	settings = cloneConfigMap(settings)
	for _, overlay := range overrides {
		if overlay == nil {
			continue
		}
		if settings == nil {
			settings = Map{}
		}
		for key, value := range overlay {
			settings[key] = cloneConfigValue(value)
		}
	}

	ctx := &Context{
		Context:  meta.Context(),
		Meta:     meta,
		Name:     name,
		Config:   config,
		Settings: settings,
		Value:    value,
		Args:     value.Clone(),
	}
	if len(action.Args) > 0 {
		args, result := rt.Mapping(action.Args, value)
		if result != nil && result.Fail() {
			return nil, result
		}
		ctx.Args = args
	}
	data, result := invokeAction(action.Action, ctx)
	if len(dataDefinition) > 0 && (result == nil || !result.Fail()) && data != nil {
		mapped, mappingResult := rt.Mapping(dataDefinition, data)
		if mappingResult != nil && mappingResult.Fail() {
			return nil, mappingResult
		}
		data = mapped
	}
	return data, result
}

func invokeAction(action any, ctx *Context) (Map, Res) {
	switch fn := action.(type) {
	case func(*Context):
		fn(ctx)
		return Map{}, OK
	case func(*Context) Map:
		return fn(ctx), OK
	case func(*Context) (Map, Res):
		data, result := fn(ctx)
		return data, defaultRes(result)
	case func(*Context) (Map, error):
		data, err := fn(ctx)
		if err != nil {
			return nil, ErrorResult(err)
		}
		return data, OK
	case func(*Context) error:
		if err := fn(ctx); err != nil {
			return nil, ErrorResult(err)
		}
		return Map{}, OK
	case func(*Context) Res:
		return Map{}, defaultRes(fn(ctx))
	case func(*Context) []Map:
		return Map{"items": fn(ctx)}, OK
	case func(*Context) ([]Map, Res):
		items, result := fn(ctx)
		return Map{"items": items}, defaultRes(result)
	case func(*Context) bool:
		return Map{"ok": fn(ctx)}, OK
	case func(*Context) any:
		return normalizeActionData(fn(ctx)), OK
	case func(*Context) (any, Res):
		data, result := fn(ctx)
		return normalizeActionData(data), defaultRes(result)
	case func(*Context) (any, error):
		data, err := fn(ctx)
		if err != nil {
			return nil, ErrorResult(err)
		}
		return normalizeActionData(data), OK
	case func(*Context) (Map, []Map):
		data, items := fn(ctx)
		return packInvokeListData(data, items), OK
	case func(*Context) (Map, []Map, Res):
		data, items, result := fn(ctx)
		return packInvokeListData(data, items), defaultRes(result)
	case func(*Context) (int64, []Map):
		total, items := fn(ctx)
		return Map{"total": total, "items": items}, OK
	case func(*Context) (int64, []Map, Res):
		total, items, result := fn(ctx)
		return Map{"total": total, "items": items}, defaultRes(result)
	default:
		return nil, ResultOf(1, fmt.Sprintf("invalid action signature: %T", action))
	}
}

func packInvokeListData(item Map, items []Map) Map {
	return Map{"item": item, "items": items}
}

func defaultRes(result Res) Res {
	if result == nil {
		return OK
	}
	return result
}

func normalizeActionData(value any) Map {
	switch item := value.(type) {
	case nil:
		return nil
	case Map:
		return item
	case map[string]any:
		return Map(item)
	default:
		return Map{"value": item}
	}
}
