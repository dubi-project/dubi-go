package dubi

import (
	"fmt"

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
	Setting  Map
}

func (Method) Scope() string { return "core:method" }

// Service is a business operation with a transport boundary. Until a Bus is
// mounted, a Service uses the same local invocation path as a Method.
type Service struct {
	Name     string
	Desc     string
	Nullable bool
	Args     Vars
	Data     Vars
	Action   any
	Setting  Map
}

func (Service) Scope() string { return "core:service" }

type coreModule struct {
	methods  map[string]Method
	services map[string]Service
}

var core = &coreModule{
	methods:  make(map[string]Method),
	services: make(map[string]Service),
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
		c.services[name] = item
	case *Service:
		if item != nil {
			c.services[name] = *item
		}
	}
}

func (c *coreModule) Reset() {
	c.methods = make(map[string]Method)
	c.services = make(map[string]Service)
}

func (*coreModule) Config(Map) error     { return nil }
func (*coreModule) Setup(*Context) error { return nil }
func (*coreModule) Open(*Context) error  { return nil }
func (*coreModule) Start(*Context) error { return nil }
func (*coreModule) Stop(*Context) error  { return nil }
func (*coreModule) Close(*Context) error { return nil }

// Invoke calls a local Method or Service on the Runtime.
func (rt *Runtime) Invoke(meta *Meta, name string, value Map) (Map, Res) {
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
	return rt.invoke(meta, name, value, false)
}

func (rt *Runtime) invoke(meta *Meta, name string, value Map, methodOnly bool) (Map, Res) {
	if meta == nil {
		meta = NewMeta()
	}
	if rt == nil || core == nil {
		return nil, ResultOf(1, "runtime is not ready")
	}

	var action Method
	var setting Map
	var dataDefinition Vars
	var config any
	if method, ok := core.methods[name]; ok {
		action = method
		setting = method.Setting
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
				Setting:  service.Setting,
			}
			setting = service.Setting
			dataDefinition = service.Data
			config = service
		} else {
			return nil, ResultOf(1, "operation not found: "+name)
		}
	} else {
		return nil, ResultOf(1, "method not found: "+name)
	}

	ctx := &Context{
		Context: meta.Context(),
		Meta:    meta,
		Name:    name,
		Config:  config,
		Setting: setting.Clone(),
		Value:   value,
		Args:    value.Clone(),
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
