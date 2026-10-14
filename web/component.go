package web

import (
	"fmt"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// Router is an HTTP route. Its Action writes the HTTP response directly.
type Router struct {
	Method   string
	Uri      string
	Uris     []string
	Name     string
	Desc     string
	Args     Vars
	Data     Vars
	Setting  Map
	Loading  Loading
	Routing  Routing
	Action   func(*Context)
	Actions  []func(*Context)
	Nullable bool
	NotFound func(*Context)
	Error    func(*Context)
	Failed   func(*Context)
	Unsigned func(*Context)
	Unauthed func(*Context)
	Denied   func(*Context)
}

func (Router) Scope() string { return "web:router" }

// Routing contains method-specific overrides for one Router. Its keys are
// HTTP methods such as GET, POST, and DELETE.
type Routing map[string]Router

// Loading invokes a Method or Service before a route action and stores the
// returned data in Context.Locals.
type Loading map[string]Loader

// Loader describes one named business value loaded for a route.
type Loader struct {
	Required bool
	Invoke   string
	Value    string
	Args     string
	Empty    Res
	Error    Res
}

// expandRouters compiles one inherited Router into concrete method routes.
// Without Routing, the original route remains method-agnostic. With Routing,
// every method receives the parent defaults and then its own overrides.
func expandRouters(name string, config Router) map[string]Router {
	result := make(map[string]Router)
	if len(config.Routing) > 0 {
		for method, override := range config.Routing {
			method = strings.ToUpper(strings.TrimSpace(method))
			if method == "" {
				continue
			}
			compiled := inheritRouter(config)
			compiled.Method = method
			compiled.Routing = nil
			compiled.Action = nil
			compiled.Actions = nil
			mergeRouter(&compiled, override)
			compiled.Method = method
			result[fmt.Sprintf("%s.%s", name, strings.ToLower(method))] = compiled
		}
		if config.Action != nil || len(config.Actions) > 0 {
			fallback := config
			fallback.Routing = nil
			result[name+".*"] = fallback
		}
		return result
	}
	config.Routing = nil
	config.Method = strings.ToUpper(strings.TrimSpace(config.Method))
	result[name] = config
	return result
}

func inheritRouter(config Router) Router {
	copyOf := config
	copyOf.Args = config.Args.Clone()
	copyOf.Data = config.Data.Clone()
	copyOf.Setting = config.Setting.Clone()
	copyOf.Loading = cloneLoading(config.Loading)
	copyOf.Uris = append([]string(nil), config.Uris...)
	copyOf.Routing = nil
	return copyOf
}

func mergeRouter(target *Router, override Router) {
	if override.Uri != "" {
		target.Uri = override.Uri
	}
	if override.Uris != nil {
		target.Uris = append([]string(nil), override.Uris...)
	}
	if override.Name != "" {
		target.Name = override.Name
	}
	if override.Desc != "" {
		target.Desc = override.Desc
	}
	if override.Args != nil {
		if target.Args == nil {
			target.Args = Vars{}
		}
		for key, value := range override.Args {
			target.Args[key] = value.Clone()
		}
	}
	if override.Data != nil {
		if target.Data == nil {
			target.Data = Vars{}
		}
		for key, value := range override.Data {
			target.Data[key] = value.Clone()
		}
	}
	if override.Setting != nil {
		if target.Setting == nil {
			target.Setting = Map{}
		}
		for key, value := range override.Setting {
			target.Setting[key] = value
		}
	}
	if override.Loading != nil {
		if target.Loading == nil {
			target.Loading = Loading{}
		}
		for key, value := range override.Loading {
			target.Loading[key] = value
		}
	}
	target.Nullable = override.Nullable
	if override.Action != nil {
		target.Action = override.Action
	}
	if override.Actions != nil {
		target.Actions = append([]func(*Context){}, override.Actions...)
	}
	if override.NotFound != nil {
		target.NotFound = override.NotFound
	}
	if override.Error != nil {
		target.Error = override.Error
	}
	if override.Failed != nil {
		target.Failed = override.Failed
	}
	if override.Unsigned != nil {
		target.Unsigned = override.Unsigned
	}
	if override.Unauthed != nil {
		target.Unauthed = override.Unauthed
	}
	if override.Denied != nil {
		target.Denied = override.Denied
	}
}

func cloneLoading(values Loading) Loading {
	if values == nil {
		return nil
	}
	copyOf := make(Loading, len(values))
	for key, value := range values {
		copyOf[key] = value
	}
	return copyOf
}

// Filter contains request pipeline hooks. A hook may call Context.Next.
type Filter struct {
	Serve    func(*Context)
	Request  func(*Context)
	Execute  func(*Context)
	Response func(*Context)
}

func (Filter) Scope() string { return "web:filter" }

// Handler handles common HTTP outcomes such as not found and errors.
type Handler struct {
	NotFound func(*Context)
	Error    func(*Context)
	Failed   func(*Context)
	Unsigned func(*Context)
	Unauthed func(*Context)
	Denied   func(*Context)
}

func (Handler) Scope() string { return "web:handler" }
