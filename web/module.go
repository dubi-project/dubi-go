// Package web provides the HTTP entry module for dubi-go.
package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	stdhttp "net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

// Module is the process-level HTTP module. It owns one set of configuration
// and registrations for the process.
type Module struct {
	mu sync.RWMutex

	enabled      bool
	config       Config
	servers      map[string]ServerConfig
	routers      map[string]Router
	filters      map[string]Filter
	handlers     map[string]Handler
	serverStates map[string]*serverState
}

var (
	module = &Module{
		servers:      make(map[string]ServerConfig),
		routers:      make(map[string]Router),
		filters:      make(map[string]Filter),
		handlers:     make(map[string]Handler),
		serverStates: make(map[string]*serverState),
	}
	host dubi.Host
)

func init() {
	host = dubi.Mount(module)
}

var _ dubi.Module = (*Module)(nil)

func (i *Module) initMaps() {
	if i.servers == nil {
		i.servers = make(map[string]ServerConfig)
	}
	if i.routers == nil {
		i.routers = make(map[string]Router)
	}
	if i.filters == nil {
		i.filters = make(map[string]Filter)
	}
	if i.handlers == nil {
		i.handlers = make(map[string]Handler)
	}
	if i.serverStates == nil {
		i.serverStates = make(map[string]*serverState)
	}
}

// Reset clears process module state for the deprecated test constructor.
func (i *Module) Reset() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, server := range i.serverStates {
		_ = server.close(context.Background())
	}
	i.enabled = false
	i.config = Config{}
	i.servers = make(map[string]ServerConfig)
	i.routers = make(map[string]Router)
	i.filters = make(map[string]Filter)
	i.handlers = make(map[string]Handler)
	i.serverStates = make(map[string]*serverState)
}

// Config loads the HTTP section before registrations are delivered.
func (i *Module) Config(global Map) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.initMaps()
	value, ok := global["web"]
	if !ok {
		return nil
	}
	section, ok := asMap(value)
	if !ok {
		return fmt.Errorf("web config must be an object")
	}
	config, err := parseConfig(section)
	if err != nil {
		return err
	}
	i.enabled = config.Require || config.Static != "" || configHasStatic(config.Servers, config.Sites)
	i.config = config
	i.servers = config.Servers
	return nil
}

// Register receives the selected dynamic components from the process registry.
func (i *Module) Register(name string, value any) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.initMaps()
	switch item := value.(type) {
	case Router:
		i.routers[name] = item
		i.enabled = true
	case *Router:
		if item != nil {
			i.routers[name] = *item
			i.enabled = true
		}
	case Filter:
		i.filters[name] = item
		i.enabled = true
	case *Filter:
		if item != nil {
			i.filters[name] = *item
			i.enabled = true
		}
	case ServeFilter:
		i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Serve: item.Action}
		i.enabled = true
	case *ServeFilter:
		if item != nil {
			i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Serve: item.Action}
			i.enabled = true
		}
	case RequestFilter:
		i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Request: item.Action}
		i.enabled = true
	case *RequestFilter:
		if item != nil {
			i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Request: item.Action}
			i.enabled = true
		}
	case ExecuteFilter:
		i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Execute: item.Action}
		i.enabled = true
	case *ExecuteFilter:
		if item != nil {
			i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Execute: item.Action}
			i.enabled = true
		}
	case ResponseFilter:
		i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Response: item.Action}
		i.enabled = true
	case *ResponseFilter:
		if item != nil {
			i.filters[name] = Filter{Name: item.Name, Desc: item.Desc, Response: item.Action}
			i.enabled = true
		}
	case Handler:
		i.handlers[name] = item
		i.enabled = true
	case *Handler:
		if item != nil {
			i.handlers[name] = *item
			i.enabled = true
		}
	case NotFoundHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, NotFound: item.Action}
		i.enabled = true
	case *NotFoundHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, NotFound: item.Action}
			i.enabled = true
		}
	case ErrorHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Error: item.Action}
		i.enabled = true
	case *ErrorHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Error: item.Action}
			i.enabled = true
		}
	case FailedHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Failed: item.Action}
		i.enabled = true
	case *FailedHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Failed: item.Action}
			i.enabled = true
		}
	case UnsignedHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Unsigned: item.Action}
		i.enabled = true
	case *UnsignedHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Unsigned: item.Action}
			i.enabled = true
		}
	case UnauthedHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Unauthed: item.Action}
		i.enabled = true
	case *UnauthedHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Unauthed: item.Action}
			i.enabled = true
		}
	case DeniedHandler:
		i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Denied: item.Action}
		i.enabled = true
	case *DeniedHandler:
		if item != nil {
			i.handlers[name] = Handler{Name: item.Name, Desc: item.Desc, Denied: item.Action}
			i.enabled = true
		}
	}
}

func (i *Module) Setup(hostContext *dubi.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.enabled {
		return nil
	}
	if len(i.servers) == 0 {
		i.servers = map[string]ServerConfig{defaultServerName: defaultServerConfig(i.config)}
	}
	if err := validateServers(i.servers); err != nil {
		return err
	}
	i.serverStates = make(map[string]*serverState, len(i.servers))
	serverNames := make(map[string]struct{}, len(i.servers))
	for _, name := range sortedKeys(i.servers) {
		serverNames[name] = struct{}{}
	}
	for _, name := range sortedKeys(i.servers) {
		config := i.servers[name]
		server := newServerState(config, serverNames)
		if err := server.register(i.routers, i.filters, i.handlers); err != nil {
			return fmt.Errorf("web server %q: %w", name, err)
		}
		i.serverStates[name] = server
	}
	return nil
}

// Open creates every configured listener. A partial open is rolled back when
// any listener cannot bind its address.
func (i *Module) Open(*dubi.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	opened := make([]*serverState, 0, len(i.serverStates))
	for _, name := range sortedKeys(i.serverStates) {
		server := i.serverStates[name]
		if err := server.open(); err != nil {
			for _, previous := range opened {
				_ = previous.close(context.Background())
			}
			return fmt.Errorf("web server %q: %w", name, err)
		}
		opened = append(opened, server)
	}
	return nil
}

// Start serves every configured listener in its own goroutine.
func (i *Module) Start(*dubi.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	for _, name := range sortedKeys(i.serverStates) {
		i.serverStates[name].start()
	}
	return nil
}

func (i *Module) Stop(*dubi.Context) error { return nil }

func (i *Module) Close(hostContext *dubi.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	shutdownContext := context.Background()
	if hostContext != nil {
		shutdownContext = hostContext.Context
	}
	var first error
	for _, name := range sortedKeys(i.serverStates) {
		server := i.serverStates[name]
		if err := server.close(shutdownContext); err != nil && first == nil {
			first = fmt.Errorf("web server %q: %w", name, err)
		}
	}
	return first
}

// ConfiguredServers returns a copy of the process module's resolved HTTP servers.
func (i *Module) ConfiguredServers() map[string]ServerConfig {
	i.mu.RLock()
	defer i.mu.RUnlock()
	result := make(map[string]ServerConfig, len(i.servers))
	for name, server := range i.servers {
		result[name] = server.Clone()
	}
	return result
}

const defaultServerName = "default"

type serverState struct {
	config        ServerConfig
	serverNames   map[string]struct{}
	listener      net.Listener
	server        *stdhttp.Server
	started       bool
	muxes         map[string]*stdhttp.ServeMux
	pathMuxes     map[string]*stdhttp.ServeMux
	pathPatterns  map[string]map[string]struct{}
	filters       map[string][]Filter
	handlers      map[string][]Handler
	hasRoutes     bool
	hasStatic     bool
	pipelines     map[string]*sitePipeline
	routes        *routeRegistry
	exactSites    map[string]string
	wildcardSites []siteHostPattern
	cors          map[string]corsPolicy
	concurrency   chan struct{}
}

type siteHostPattern struct {
	suffix string
	site   string
}

type corsPolicy struct {
	origins     []string
	methods     []string
	headers     []string
	allow       bool
	credentials bool
	maxAge      int
}

type sitePipeline struct {
	mux           *stdhttp.ServeMux
	pathMux       *stdhttp.ServeMux
	staticRoot    string
	staticRootAbs string
	staticPrefix  string
	setting       Map
	handlers      []Handler
	chain         []func(*Context)
	requestChain  []func(*Context)
	executeChain  []func(*Context)
	response      []func(*Context)
	outcomes      map[string][]func(*Context)
}

func newServerState(config ServerConfig, serverNames map[string]struct{}) *serverState {
	muxes := make(map[string]*stdhttp.ServeMux)
	pathMuxes := make(map[string]*stdhttp.ServeMux)
	pathPatterns := make(map[string]map[string]struct{})
	if len(config.Sites) == 0 {
		muxes[""] = stdhttp.NewServeMux()
		pathMuxes[""] = stdhttp.NewServeMux()
		pathPatterns[""] = make(map[string]struct{})
	} else {
		for name := range config.Sites {
			muxes[name] = stdhttp.NewServeMux()
			pathMuxes[name] = stdhttp.NewServeMux()
			pathPatterns[name] = make(map[string]struct{})
		}
	}
	exactSites, wildcardSites := compileSiteHosts(config.Sites)
	cors := make(map[string]corsPolicy, len(config.Sites)+1)
	cors[""] = compileCORSPolicy(config.Cross)
	for name, site := range config.Sites {
		cors[name] = compileCORSPolicy(mergeMap(config.Cross, site.Cross))
	}
	return &serverState{
		config: config, serverNames: serverNames, muxes: muxes,
		pathMuxes: pathMuxes, pathPatterns: pathPatterns,
		filters: make(map[string][]Filter), handlers: make(map[string][]Handler),
		hasStatic:     config.Static != "" || sitesHaveStatic(config.Sites),
		pipelines:     make(map[string]*sitePipeline),
		routes:        newRouteRegistry(config),
		exactSites:    exactSites,
		wildcardSites: wildcardSites,
		cors:          cors,
		concurrency:   newConcurrencyGate(config.MaxConcurrency),
	}
}

func newConcurrencyGate(limit int) chan struct{} {
	if limit <= 0 {
		return nil
	}
	return make(chan struct{}, limit)
}

func (s *serverState) settingForSite(site string) Map {
	setting := s.config.Setting.Clone()
	if s.config.Compress {
		if setting == nil {
			setting = Map{}
		}
		setting["compress"] = true
	}
	if site != "" {
		setting = mergeMap(setting, s.config.Sites[site].Setting)
	}
	return setting
}

func (s *serverState) resolveClientIP(request *stdhttp.Request) string {
	if request == nil {
		return ""
	}
	peer := requestPeerIP(request.RemoteAddr)
	if len(s.config.TrustedProxies) == 0 {
		return peer
	}
	if !s.isTrustedProxy(peer) {
		return peer
	}
	var chain []string
	for _, value := range strings.Split(request.Header.Get("X-Forwarded-For"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			chain = append(chain, requestPeerIP(value))
		}
	}
	if real := strings.TrimSpace(request.Header.Get("X-Real-IP")); real != "" {
		chain = append(chain, requestPeerIP(real))
	}
	for index := len(chain) - 1; index >= 0; index-- {
		if !s.isTrustedProxy(chain[index]) {
			return chain[index]
		}
	}
	if len(chain) > 0 {
		return chain[0]
	}
	return peer
}

func (s *serverState) isTrustedProxy(address string) bool {
	address = strings.TrimSpace(address)
	parsed := net.ParseIP(address)
	for _, pattern := range s.config.TrustedProxies {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" {
			return true
		}
		if ip := net.ParseIP(pattern); ip != nil && parsed != nil && ip.Equal(parsed) {
			return true
		}
		if _, network, err := net.ParseCIDR(pattern); err == nil && parsed != nil && network.Contains(parsed) {
			return true
		}
	}
	return false
}

func (s *serverState) register(routers map[string]Router, filters map[string]Filter, handlers map[string]Handler) error {
	for _, name := range sortedKeys(routers) {
		router := routers[name]
		compiledRouters := expandRouters(name, router)
		for _, compiledName := range sortedKeys(compiledRouters) {
			compiled := compiledRouters[compiledName]
			sites, route, ok := routeSites(s.config, s.serverNames, compiledName)
			if !ok {
				continue
			}
			uris := append([]string(nil), compiled.Uris...)
			if compiled.Uri != "" {
				uris = append(uris, compiled.Uri)
			}
			if len(uris) == 0 {
				uris = []string{"/"}
			}
			for _, uri := range uris {
				pattern := uri
				if compiled.Method != "" {
					pattern = compiled.Method + " " + pattern
				}
				for _, site := range sites {
					mux, exists := s.muxes[site]
					if !exists {
						continue
					}
					baseSetting := s.settingForSite(site)
					s.routes.add(name, compiledName, route, uri, site)
					handler := compiled
					routeSetting := mergeMap(baseSetting, handler.Setting)
					var argumentPlan *dubi.MappingPlan
					if len(handler.Args) > 0 {
						argumentPlan = dubi.PrepareMapping(handler.Args)
					}
					var responsePlan *dubi.MappingPlan
					if len(handler.Data) > 0 {
						responsePlan = dubi.PrepareMapping(handler.Data)
					}
					actions := append([]func(*Context){}, handler.Actions...)
					if handler.Action != nil {
						actions = append(actions, handler.Action)
					}
					registeredPattern := pattern
					pathParameters := compilePathParameters(uri)
					registered := &routeHandler{
						serve: func(ctx *Context, writer stdhttp.ResponseWriter, request *stdhttp.Request) {
							request.Pattern = registeredPattern
							ctx.Request = request
							ctx.Writer = writer
							ctx.Name = route
							ctx.Config = handler
							ctx.Setting = assignMap(ctx.Setting, routeSetting)
							ctx.Locals = resetMap(ctx.Locals)
							ctx.responseVars = handler.Data
							ctx.responsePlan = responsePlan
							ctx.Params = resetMap(ctx.Params)
							pathParamsInto(ctx.Params, request, pathParameters)
							if !authorizeRouter(ctx, handler) {
								runRouterOutcome(ctx, handler)
								return
							}
							ctx.Query = resetMap(ctx.Query)
							queryValuesInto(ctx.Query, request)
							values := assignMap(ctx.Value, ctx.Params)
							for key, value := range ctx.Query {
								values[key] = value
							}
							form, files, err := requestFormValuesInto(request, ctx.Form)
							if err != nil {
								ctx.Failed(dubi.ErrorResult(err))
								var tooLarge *stdhttp.MaxBytesError
								if errors.As(err, &tooLarge) {
									ctx.Code = stdhttp.StatusRequestEntityTooLarge
								}
								runRouterOutcome(ctx, handler)
								return
							}
							ctx.Form = form
							ctx.Files = assignFiles(ctx.Files, files)
							for key, value := range form {
								values[key] = value
							}
							ctx.Value = values
							ctx.Args = assignMap(ctx.Args, values)
							if len(handler.Args) > 0 {
								args, result := dubi.ApplyMapping(argumentPlan, values)
								if result != nil && result.Fail() {
									ctx.Failed(result)
									runRouterOutcome(ctx, handler)
									return
								}
								ctx.Args = args
							}
							if !runLoading(ctx, handler.Loading) {
								runRouterOutcome(ctx, handler)
								return
							}
							if len(actions) > 0 {
								runChain(ctx, actions)
								runRouterOutcome(ctx, handler)
								return
							}
							ctx.NotFound()
							runRouterOutcome(ctx, handler)
						},
					}
					if err := registerHandler(mux, registeredPattern, registered); err != nil {
						return fmt.Errorf("web route %q: %w", compiledName, err)
					}
					// Keep a methodless path index for fast 404/405 detection.
					// Path conflicts are harmless here: any matching path is
					// enough to distinguish a method mismatch from a 404.
					if patterns := s.pathPatterns[site]; patterns != nil {
						if _, exists := patterns[uri]; !exists {
							patterns[uri] = struct{}{}
							_ = registerHandler(s.pathMuxes[site], uri, stdhttp.NotFoundHandler())
						}
					}
					s.hasRoutes = true
				}
			}
		}
	}
	for _, name := range sortedKeys(filters) {
		filter := filters[name]
		sites, _, ok := routeSites(s.config, s.serverNames, name)
		if !ok {
			continue
		}
		for _, site := range sites {
			s.filters[site] = append(s.filters[site], filter)
		}
	}
	for _, name := range sortedKeys(handlers) {
		handler := handlers[name]
		sites, _, ok := routeSites(s.config, s.serverNames, name)
		if !ok {
			continue
		}
		for _, site := range sites {
			s.handlers[site] = append(s.handlers[site], handler)
		}
	}
	s.compilePipelines()
	return nil
}

func registerHandler(mux *stdhttp.ServeMux, pattern string, handler stdhttp.Handler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("invalid or conflicting pattern %q: %v", pattern, recovered)
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}

// routeHandler is the internal route implementation registered in ServeMux.
// ServeHTTP keeps direct net/http compatibility, while ServeContext lets the
// Web pipeline pass its already-acquired Context without a request context
// allocation on every request.
type routeHandler struct {
	serve func(*Context, stdhttp.ResponseWriter, *stdhttp.Request)
}

func (h *routeHandler) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	ctx, _ := request.Context().Value(httpContextKey{}).(*Context)
	if ctx == nil {
		ctx = NewContext(writer, request)
	}
	h.ServeContext(ctx, writer, request)
}

func (h *routeHandler) ServeContext(ctx *Context, writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if h == nil || h.serve == nil {
		return
	}
	h.serve(ctx, writer, request)
}

// sortedKeys gives component registration a stable order after the dynamic
// registry has been copied into maps. Registry entries are already sorted by
// name, so using the same order here keeps Filter and Handler chains
// reproducible across processes.
func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// compilePipelines moves the stable part of request dispatch out of the hot
// path. The process module is configured once, while its request Context is
// created for each request.
func (s *serverState) compilePipelines() {
	for site, mux := range s.muxes {
		items := append([]Filter{}, s.filters[""]...)
		if site != "" {
			items = append(items, s.filters[site]...)
		}
		staticRoot := s.config.Static
		if site != "" && s.config.Sites[site].Static != "" {
			staticRoot = s.config.Sites[site].Static
		}
		staticRootAbs := ""
		staticPrefix := ""
		if staticRoot != "" {
			staticRootAbs, _ = filepath.Abs(staticRoot)
			staticPrefix = staticRootAbs + string(filepath.Separator)
		}
		p := &sitePipeline{
			mux:           mux,
			pathMux:       s.pathMuxes[site],
			staticRoot:    staticRoot,
			staticRootAbs: staticRootAbs,
			staticPrefix:  staticPrefix,
			setting:       s.settingForSite(site),
			handlers:      append([]Handler{}, s.handlers[""]...),
			outcomes:      make(map[string][]func(*Context)),
		}
		if site != "" {
			p.handlers = append(p.handlers, s.handlers[site]...)
		}
		for _, item := range p.handlers {
			if item.NotFound != nil {
				p.outcomes["notfound"] = append(p.outcomes["notfound"], item.NotFound)
			}
			if item.Error != nil {
				p.outcomes["error"] = append(p.outcomes["error"], item.Error)
			}
			if item.Failed != nil {
				p.outcomes["failed"] = append(p.outcomes["failed"], item.Failed)
			}
			if item.Unsigned != nil {
				p.outcomes["unsigned"] = append(p.outcomes["unsigned"], item.Unsigned)
			}
			if item.Unauthed != nil {
				p.outcomes["unauthed"] = append(p.outcomes["unauthed"], item.Unauthed)
			}
			if item.Denied != nil {
				p.outcomes["denied"] = append(p.outcomes["denied"], item.Denied)
			}
		}
		for _, item := range items {
			if item.Serve != nil {
				p.chain = append(p.chain, item.Serve)
			}
			if item.Request != nil {
				p.requestChain = append(p.requestChain, item.Request)
			}
			if item.Execute != nil {
				p.executeChain = append(p.executeChain, item.Execute)
			}
			if item.Response != nil {
				p.response = append(p.response, item.Response)
			}
		}
		p.executeChain = append(p.executeChain, func(ctx *Context) {
			if selected, ok := ctx.routeHandler.(*routeHandler); ok {
				selected.ServeContext(ctx, ctx.Writer, ctx.Request)
				return
			}
			if ctx.routeHandler != nil {
				ctx.Request.Pattern = ctx.routePattern
				ctx.routeHandler.ServeHTTP(ctx.Writer, ctx.Request)
				return
			}
			p.mux.ServeHTTP(ctx.Writer, ctx.Request)
		})
		p.requestChain = append(p.requestChain, func(ctx *Context) {
			ctx.routeHandler = nil
			routeHandler, pattern := p.mux.Handler(ctx.Request)
			ctx.routePattern = pattern
			if pattern == "" && !p.hasPathRoute(ctx.Request) {
				if p.serveStatic(ctx) {
					defer ctx.closeStreams()
					p.finish(ctx)
					return
				}
				ctx.NotFound()
				return
			}
			// Keep the selected handler for both a normal match and a method
			// mismatch. The latter is the native ServeMux 405 handler, including
			// its Allow header; invoking it directly avoids a second tree lookup.
			ctx.routeHandler = routeHandler
			runChain(ctx, p.executeChain)
		})
		p.chain = append(p.chain, func(ctx *Context) {
			runChain(ctx, p.requestChain)
			if ctx.handling != "" {
				p.handleOutcome(ctx)
			}
		})
		p.chain = append(p.chain, func(ctx *Context) {
			ctx.responseRan = true
			runChain(ctx, p.response)
		})

		s.pipelines[site] = p
	}
}

func (p *sitePipeline) hasPathRoute(request *stdhttp.Request) bool {
	if p == nil || p.pathMux == nil {
		return false
	}
	_, pattern := p.pathMux.Handler(request)
	return pattern != ""
}

func (p *sitePipeline) serveStatic(ctx *Context) bool {
	root := p.staticRootAbs
	if root == "" {
		return false
	}
	clean := filepath.Clean("/" + ctx.Request.URL.Path)
	if clean == "/" {
		clean = "/index.html"
	}
	fileAbs := filepath.Join(root, clean)
	if root != string(filepath.Separator) && fileAbs != root && !strings.HasPrefix(fileAbs, p.staticPrefix) {
		return false
	}
	info, err := os.Stat(fileAbs)
	if err != nil || info.IsDir() {
		return false
	}
	if cache, ok := p.setting["cache_control"].(string); ok && strings.TrimSpace(cache) != "" {
		ctx.CacheControl(cache)
	}
	ctx.fileWithInfo(fileAbs, info)
	return true
}

func requestFormValues(request *stdhttp.Request) (Map, map[string][]*multipart.FileHeader, error) {
	return requestFormValuesInto(request, Map{})
}

func requestFormValuesInto(request *stdhttp.Request, result Map) (Map, map[string][]*multipart.FileHeader, error) {
	result = resetMap(result)
	if request.Body == nil || request.Body == stdhttp.NoBody || request.ContentLength == 0 || request.Method == stdhttp.MethodGet || request.Method == stdhttp.MethodHead {
		return result, nil, nil
	}
	files := map[string][]*multipart.FileHeader{}
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		contentType = strings.TrimSpace(strings.SplitN(request.Header.Get("Content-Type"), ";", 2)[0])
	}
	contentType = strings.ToLower(contentType)
	if contentType == "application/json" || strings.HasSuffix(contentType, "+json") {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, nil, err
		}
		request.Body.Close()
		request.Body = io.NopCloser(bytes.NewReader(body))
		if len(bytes.TrimSpace(body)) == 0 {
			return result, files, nil
		}
		value, err := dubi.Decode("json", body, nil)
		if err != nil {
			return nil, nil, err
		}
		decoded, ok := value.(Map)
		if !ok {
			return nil, nil, fmt.Errorf("json request body must be an object")
		}
		return assignMap(result, decoded), files, nil
	}
	if contentType != "application/x-www-form-urlencoded" && contentType != "multipart/form-data" {
		return result, files, nil
	}
	if contentType == "application/x-www-form-urlencoded" {
		if err := request.ParseForm(); err != nil {
			return nil, nil, err
		}
	} else if err := request.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, err
	}
	for key, values := range request.PostForm {
		if len(values) == 1 {
			assignInputValue(result, key, values[0])
		} else if len(values) > 1 {
			assignInputValue(result, key, append([]string(nil), values...))
		}
	}
	if request.MultipartForm != nil {
		for key, values := range request.MultipartForm.File {
			files[key] = append([]*multipart.FileHeader(nil), values...)
		}
	}
	return result, files, nil
}

func assignFiles(target, source map[string][]*multipart.FileHeader) map[string][]*multipart.FileHeader {
	if target == nil {
		target = make(map[string][]*multipart.FileHeader)
	}
	clear(target)
	for key, values := range source {
		target[key] = values
	}
	return target
}

func sitesHaveStatic(sites map[string]SiteConfig) bool {
	for _, site := range sites {
		if site.Static != "" {
			return true
		}
	}
	return false
}

func configHasStatic(servers map[string]ServerConfig, sites map[string]SiteConfig) bool {
	if sitesHaveStatic(sites) {
		return true
	}
	for _, server := range servers {
		if server.Static != "" || sitesHaveStatic(server.Sites) {
			return true
		}
	}
	return false
}

func (s *serverState) open() error {
	host := s.config.Host
	if host == "" {
		host = "0.0.0.0"
	}
	port := s.config.Port
	if port == 0 {
		port = 8080
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	if s.config.TLS.Cert != "" {
		certificate, loadErr := tls.LoadX509KeyPair(s.config.TLS.Cert, s.config.TLS.Key)
		if loadErr != nil {
			_ = listener.Close()
			return fmt.Errorf("load tls certificate: %w", loadErr)
		}
		listener = tls.NewListener(listener, &tls.Config{
			Certificates: []tls.Certificate{certificate},
			MinVersion:   tls.VersionTLS12,
		})
	}
	s.listener = listener
	s.server = &stdhttp.Server{
		Handler:           stdhttp.HandlerFunc(s.serveHTTP),
		ReadTimeout:       s.config.ReadTimeout,
		ReadHeaderTimeout: s.config.ReadHeaderTimeout,
		WriteTimeout:      s.config.WriteTimeout,
		IdleTimeout:       s.config.IdleTimeout,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}
	return nil
}

func (s *serverState) serveHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	var ctx *Context
	defer func() {
		if recovered := recover(); recovered != nil {
			s.recoverPanic(writer, ctx, recovered)
		}
		if ctx != nil {
			releaseContext(ctx)
		}
	}()
	if s.config.RequestTimeout > 0 {
		requestContext, cancel := context.WithTimeout(request.Context(), s.config.RequestTimeout)
		defer cancel()
		request = request.WithContext(requestContext)
	}
	if s.config.MaxBodyBytes > 0 && request.Body != nil {
		request.Body = stdhttp.MaxBytesReader(writer, request.Body, s.config.MaxBodyBytes)
	}
	site, ok := s.siteForHost(request.Host)
	if !ok {
		stdhttp.NotFound(writer, request)
		return
	}
	p := s.pipelines[site]
	if p == nil {
		stdhttp.NotFound(writer, request)
		return
	}
	if s.applyCORS(writer, request, site) {
		return
	}
	if s.concurrency != nil {
		select {
		case s.concurrency <- struct{}{}:
			defer func() { <-s.concurrency }()
		default:
			writer.Header().Set("Retry-After", "1")
			stdhttp.Error(writer, stdhttp.StatusText(stdhttp.StatusServiceUnavailable), stdhttp.StatusServiceUnavailable)
			return
		}
	}
	if !s.hasRoutes && !s.hasStatic && len(s.filters) == 0 && len(s.handlers) == 0 {
		stdhttp.NotFound(writer, request)
		return
	}
	noPipeline := len(s.filters) == 0 && len(s.handlers) == 0
	var matchedHandler stdhttp.Handler
	var matchedPattern string
	if noPipeline && p.staticRoot != "" {
		var pattern string
		matchedHandler, pattern = p.mux.Handler(request)
		matchedPattern = pattern
		if pattern == "" && !p.hasPathRoute(request) {
			if s.hasStatic {
				ctx = acquireContext(writer, request)
				ctx.Site = site
				ctx.Server = s.config.Name
				ctx.Setting = assignMap(ctx.Setting, p.setting)
				ctx.routes = s.routes
				if p.serveStatic(ctx) {
					p.finish(ctx)
					return
				}
			}
			stdhttp.NotFound(writer, request)
			return
		}
	}
	if noPipeline && matchedHandler == nil {
		matchedHandler, matchedPattern = p.mux.Handler(request)
	}
	ctx = acquireContext(writer, request)
	ctx.Site = site
	ctx.Server = s.config.Name
	ctx.RemoteIP = s.resolveClientIP(request)
	ctx.Setting = assignMap(ctx.Setting, p.setting)
	ctx.routes = s.routes
	ctx.Request = request
	if noPipeline {
		if selected, ok := matchedHandler.(*routeHandler); ok {
			selected.ServeContext(ctx, ctx.Writer, request)
		} else if matchedHandler != nil {
			request.Pattern = matchedPattern
			matchedHandler.ServeHTTP(ctx.Writer, request)
		} else {
			p.mux.ServeHTTP(ctx.Writer, request)
		}
	} else {
		runChain(ctx, p.chain)
	}
	if ctx.handling != "" {
		p.handleOutcome(ctx)
	}
	if !ctx.responseRan {
		ctx.responseRan = true
		runChain(ctx, p.response)
	}
	if ctx.handling != "" {
		p.handleOutcome(ctx)
	}
	p.finish(ctx)
}

func (s *serverState) recoverPanic(writer stdhttp.ResponseWriter, ctx *Context, recovered any) {
	log.Printf("web: recovered panic: %v", recovered)
	if ctx == nil {
		stdhttp.Error(writer, stdhttp.StatusText(stdhttp.StatusInternalServerError), stdhttp.StatusInternalServerError)
		return
	}
	if ctx.output != nil && ctx.output.code != 0 {
		return
	}
	ctx.Error()
	p := s.pipelines[ctx.Site]
	if p != nil {
		func() {
			defer func() { _ = recover() }()
			p.handleOutcome(ctx)
		}()
		if !ctx.responseRan {
			ctx.responseRan = true
			func() {
				defer func() { _ = recover() }()
				runChain(ctx, p.response)
			}()
		}
		if ctx.output == nil || ctx.output.code == 0 {
			func() {
				defer func() { _ = recover() }()
				p.finish(ctx)
			}()
		}
	}
}

func (s *serverState) applyCORS(writer stdhttp.ResponseWriter, request *stdhttp.Request, site string) bool {
	policy, ok := s.cors[site]
	if !ok || !policy.allow {
		return false
	}
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	allowed := len(policy.origins) == 0 || corsOriginAllowed(origin, policy.origins)
	if !allowed {
		return false
	}
	writer.Header().Add("Vary", "Origin")
	if policy.credentials || len(policy.origins) > 0 {
		writer.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
	}
	if policy.credentials {
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	methods := policy.methods
	if len(methods) == 0 {
		methods = defaultCORMethods
	}
	writer.Header().Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
	headers := policy.headers
	if len(headers) == 0 {
		headers = stringList(request.Header.Get("Access-Control-Request-Headers"))
	}
	if len(headers) > 0 {
		writer.Header().Set("Access-Control-Allow-Headers", strings.Join(headers, ", "))
	}
	if policy.maxAge > 0 {
		writer.Header().Set("Access-Control-Max-Age", fmt.Sprint(policy.maxAge))
	}
	if request.Method == stdhttp.MethodOptions {
		writer.WriteHeader(stdhttp.StatusNoContent)
		return true
	}
	return false
}

var defaultCORMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

func compileCORSPolicy(cross Map) corsPolicy {
	if len(cross) == 0 {
		return corsPolicy{}
	}
	policy := corsPolicy{
		origins:     stringList(firstPresent(cross, "origins", "origin")),
		methods:     stringList(firstPresent(cross, "methods", "method")),
		headers:     stringList(firstPresent(cross, "headers", "header")),
		credentials: boolValue(firstPresent(cross, "credentials", "credential")),
		maxAge:      intValue(firstPresent(cross, "maxage", "max_age")),
	}
	policy.allow = boolValue(cross["allow"]) || len(policy.origins) > 0 || len(policy.methods) > 0 || len(policy.headers) > 0
	return policy
}

func corsOriginAllowed(origin string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "*" || strings.EqualFold(pattern, origin) {
			return true
		}
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(strings.ToLower(origin), strings.ToLower(pattern[1:])) {
			return true
		}
	}
	return false
}

func (p *sitePipeline) handleOutcome(ctx *Context) {
	kind := ctx.handling
	ctx.handling = ""
	hooks := p.outcomes[kind]
	if len(hooks) > 0 {
		runChain(ctx, hooks)
	}
}

func runRouterOutcome(ctx *Context, router Router) {
	if ctx.handling == "" {
		return
	}
	var handler func(*Context)
	switch ctx.handling {
	case "notfound":
		handler = router.NotFound
	case "error":
		handler = router.Error
	case "failed":
		handler = router.Failed
	case "unsigned":
		handler = router.Unsigned
	case "unauthed":
		handler = router.Unauthed
	case "denied":
		handler = router.Denied
	}
	if handler == nil {
		return
	}
	ctx.handling = ""
	handler(ctx)
}

func authorizeRouter(ctx *Context, router Router) bool {
	raw := requestToken(ctx)
	if raw != "" {
		if _, err := ctx.VerifyToken(raw); err != nil {
			if router.Sign {
				ctx.Unsigned()
			} else if router.Auth {
				ctx.Unauthed()
			}
			return !router.Sign && !router.Auth
		}
	}
	if router.Sign && !ctx.Signed() {
		ctx.Unsigned()
		return false
	}
	if router.Auth && !ctx.Authed() {
		ctx.Unauthed()
		return false
	}
	if (router.Sign || router.Auth) && router.Kind != "" && ctx.Kind() != router.Kind {
		ctx.Denied()
		return false
	}
	return true
}

func requestToken(ctx *Context) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	if value := strings.TrimSpace(ctx.Request.Header.Get("Authorization")); value != "" {
		parts := strings.Fields(value)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return parts[1]
		}
		return value
	}
	if cookie, err := ctx.Request.Cookie(dubi.AuthCookieName()); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	return ""
}

func runChain(ctx *Context, chain []func(*Context)) {
	previousNexts, previousIndex := ctx.nexts, ctx.index
	ctx.nexts, ctx.index = chain, 0
	ctx.Next()
	ctx.nexts, ctx.index = previousNexts, previousIndex
}

func (s *serverState) start() {
	if s.listener == nil || s.server == nil {
		return
	}
	s.started = true
	go func() { _ = s.server.Serve(s.listener) }()
}

func (s *serverState) close(parent context.Context) error {
	if s.server != nil {
		var err error
		if s.started {
			shutdownContext := parent
			if shutdownContext == nil {
				shutdownContext = context.Background()
			}
			timeout := s.config.ShutdownTimeout
			if timeout <= 0 {
				timeout = 5 * time.Second
			}
			var cancel context.CancelFunc
			shutdownContext, cancel = context.WithTimeout(shutdownContext, timeout)
			defer cancel()
			err = s.server.Shutdown(shutdownContext)
			if err != nil {
				_ = s.server.Close()
			}
		} else {
			err = s.server.Close()
		}
		s.server = nil
		s.listener = nil
		s.started = false
		return err
	}
	if s.listener != nil {
		err := s.listener.Close()
		s.listener = nil
		return err
	}
	return nil
}

func routeSites(config ServerConfig, serverNames map[string]struct{}, name string) (sites []string, route string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", false
	}
	if strings.HasPrefix(name, "*.") {
		route = strings.TrimPrefix(name, "*.")
		if len(config.Sites) == 0 {
			return []string{""}, route, true
		}
		for site := range config.Sites {
			sites = append(sites, site)
		}
		return sites, route, true
	}
	parts := strings.SplitN(name, ".", 3)
	if len(config.Sites) == 0 {
		if len(parts) > 1 {
			if _, isServer := serverNames[parts[0]]; isServer {
				if parts[0] != config.Name {
					return nil, "", false
				}
				return []string{""}, strings.Join(parts[1:], "."), true
			}
			if config.Name != defaultServerName {
				return nil, "", false
			}
		}
		return []string{""}, name, true
	}
	if len(parts) == 3 && parts[0] == config.Name {
		if _, exists := config.Sites[parts[1]]; exists {
			return []string{parts[1]}, parts[2], true
		}
	}
	if len(parts) == 3 {
		if _, isServer := serverNames[parts[0]]; isServer {
			return nil, "", false
		}
	}
	if len(parts) >= 2 {
		if _, exists := config.Sites[parts[0]]; exists {
			return []string{parts[0]}, strings.Join(parts[1:], "."), true
		}
	}
	if config.Name != defaultServerName {
		return nil, "", false
	}
	return []string{""}, name, true
}

func compileSiteHosts(sites map[string]SiteConfig) (map[string]string, []siteHostPattern) {
	exact := make(map[string]string)
	wildcards := make([]siteHostPattern, 0)
	for _, name := range sortedKeys(sites) {
		site := sites[name]
		candidates := append(append([]string{}, site.Hosts...), site.Aliases...)
		for _, candidate := range candidates {
			candidate = normalizeHostPattern(candidate)
			if candidate == "" {
				continue
			}
			if candidate == "*" {
				wildcards = append(wildcards, siteHostPattern{site: name})
				continue
			}
			if strings.HasPrefix(candidate, "*.") {
				wildcards = append(wildcards, siteHostPattern{suffix: candidate[1:], site: name})
				continue
			}
			exact[candidate] = name
		}
	}
	sort.SliceStable(wildcards, func(left, right int) bool {
		if len(wildcards[left].suffix) != len(wildcards[right].suffix) {
			return len(wildcards[left].suffix) > len(wildcards[right].suffix)
		}
		return wildcards[left].site < wildcards[right].site
	})
	return exact, wildcards
}

func normalizeHostPattern(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" || host == "*" {
		return host
	}
	if strings.HasPrefix(host, "*.") {
		return "*." + strings.TrimSuffix(strings.TrimSpace(host[2:]), ".")
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return strings.TrimSuffix(host, ".")
}

func (s *serverState) siteForHost(rawHost string) (string, bool) {
	if len(s.config.Sites) == 0 {
		return "", true
	}
	host := normalizeHostPattern(rawHost)
	if site, ok := s.exactSites[host]; ok {
		return site, true
	}
	for _, pattern := range s.wildcardSites {
		if pattern.suffix == "" || strings.HasSuffix(host, pattern.suffix) {
			return pattern.site, true
		}
	}
	return "", false
}

type httpContextKey struct{}
