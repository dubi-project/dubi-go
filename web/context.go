package web

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	stdhttp "net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

var (
	resultNotFound = dubi.Result(1, "notfound", stdhttp.StatusText(stdhttp.StatusNotFound))
	resultError    = dubi.Result(1, "error", stdhttp.StatusText(stdhttp.StatusInternalServerError))
	resultFailed   = dubi.Result(1, "failed", stdhttp.StatusText(stdhttp.StatusBadRequest))
	resultUnsigned = dubi.Result(1, "unsigned", "Signature Required")
	resultUnauthed = dubi.Result(1, "unauthed", "Authentication Required")
	resultDenied   = dubi.Result(1, "denied", stdhttp.StatusText(stdhttp.StatusForbidden))
)

var contextPool = sync.Pool{
	New: func() any { return &Context{} },
}

// Context is the HTTP request context. Router actions write responses through
// this object and may explicitly invoke Dubi business operations when useful.
type Context struct {
	*dubi.Meta
	metaValue dubi.Meta
	Request   *stdhttp.Request
	Writer    stdhttp.ResponseWriter
	StartedAt time.Time
	Name      string

	Method   string
	Host     string
	Site     string
	Server   string
	Ajax     bool
	Path     string
	Uri      string
	Params   Map
	Query    Map
	Form     Map
	Value    Map
	Args     Map
	Files    map[string][]*multipart.FileHeader
	RemoteIP string

	Code    int
	Body    any
	Data    Map
	Setting Map
	Config  Router
	Locals  Map

	nexts        []func(*Context)
	index        int
	handling     string
	responseRan  bool
	wrote        bool
	output       *responseWriter
	responseVars Vars
	responsePlan *dubi.MappingPlan
	routeHandler stdhttp.Handler
	routePattern string
	streams      []io.Closer
	routes       *routeRegistry
}

func (ctx *Context) Header(name string, values ...string) string {
	if len(values) > 0 {
		ctx.Writer.Header().Set(name, values[0])
		return values[0]
	}
	return ctx.Request.Header.Get(name)
}

func (ctx *Context) setResponseHeader(name, value string) {
	if ctx == nil || ctx.Writer == nil {
		return
	}
	ctx.Writer.Header()[name] = []string{value}
}

// ContentType returns the media type without parameters.
func (ctx *Context) ContentType() string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	value, _, err := mime.ParseMediaType(ctx.Request.Header.Get("Content-Type"))
	if err != nil {
		return strings.TrimSpace(strings.SplitN(ctx.Request.Header.Get("Content-Type"), ";", 2)[0])
	}
	return value
}

// Accepts reports whether the request accepts one of the supplied media types.
// A wildcard in either side is treated as a match.
func (ctx *Context) Accepts(types ...string) bool {
	if ctx == nil || ctx.Request == nil {
		return false
	}
	if strings.TrimSpace(ctx.Request.Header.Get("Accept")) == "" {
		return true
	}
	accepted := strings.Split(ctx.Request.Header.Get("Accept"), ",")
	if len(types) == 0 {
		return true
	}
	for _, want := range types {
		want = strings.TrimSpace(strings.ToLower(want))
		for _, item := range accepted {
			parts := strings.Split(item, ";")
			item = strings.TrimSpace(strings.ToLower(parts[0]))
			quality := 1.0
			for _, parameter := range parts[1:] {
				name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
				if ok && strings.EqualFold(strings.TrimSpace(name), "q") {
					quality, _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
				}
			}
			if quality <= 0 {
				continue
			}
			if item == "" || item == "*/*" || item == want || (strings.HasSuffix(item, "/*") && strings.HasPrefix(want, strings.TrimSuffix(item, "*"))) {
				return true
			}
		}
	}
	return false
}

// ClientIP returns the address resolved by the Web server's trusted proxy
// policy. It is empty only when the underlying request has no peer address.
func (ctx *Context) ClientIP() string { return ctx.RemoteIP }

// Elapsed returns the time since this request Context was acquired.
func (ctx *Context) Elapsed() time.Duration {
	if ctx == nil || ctx.StartedAt.IsZero() {
		return 0
	}
	return time.Since(ctx.StartedAt)
}

// Upload returns the first uploaded file for a form field.
func (ctx *Context) Upload(name string) (*multipart.FileHeader, bool) {
	files := ctx.Uploads(name)
	if len(files) == 0 {
		return nil, false
	}
	return files[0], true
}

// Uploads returns all uploaded files for a form field.
func (ctx *Context) Uploads(name string) []*multipart.FileHeader {
	if ctx == nil || ctx.Files == nil {
		return nil
	}
	return append([]*multipart.FileHeader(nil), ctx.Files[name]...)
}

// Cookie reads a request cookie or writes one when a value is supplied.
// This keeps the common infrago-style getter/setter call in one method.
func (ctx *Context) Cookie(name string, values ...any) string {
	if len(values) > 0 {
		switch cookie := values[0].(type) {
		case stdhttp.Cookie:
			ctx.SetCookie(&cookie)
			return cookie.Value
		case *stdhttp.Cookie:
			ctx.SetCookie(cookie)
			if cookie != nil {
				return cookie.Value
			}
			return ""
		}
		value := fmt.Sprint(values[0])
		ctx.SetCookieValue(name, value)
		return value
	}
	cookie, err := ctx.Request.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// CookieObject returns the complete request cookie.
func (ctx *Context) CookieObject(name string) (*stdhttp.Cookie, error) {
	return ctx.Request.Cookie(name)
}

// SetCookie adds a response cookie.
func (ctx *Context) SetCookie(cookie *stdhttp.Cookie) {
	if cookie != nil {
		stdhttp.SetCookie(ctx.Writer, cookie)
	}
}

// CookieValue returns a cookie value or an empty string when it is absent.
func (ctx *Context) CookieValue(name string) string {
	return ctx.Cookie(name)
}

// SetCookieValue writes a convenient session cookie with an optional lifetime.
func (ctx *Context) SetCookieValue(name, value string, maxAge ...time.Duration) {
	seconds := 0
	if len(maxAge) > 0 {
		seconds = int(maxAge[0] / time.Second)
	}
	ctx.SetCookie(&stdhttp.Cookie{Name: name, Value: value, Path: "/", MaxAge: seconds})
}

// Auth issues an authenticated Token using the current Router.Kind and writes
// it to the process module's configured auth cookie. The token is also returned for
// clients that prefer an Authorization header or a response body.
func (ctx *Context) Auth(payload ...Map) string {
	var value Map
	if len(payload) > 0 {
		value = payload[0]
	}
	token, err := ctx.SignToken(dubi.Token{
		Auth:    true,
		Kind:    ctx.Config.Kind,
		Payload: value,
	})
	if err != nil {
		ctx.Error(dubi.ErrorResult(err))
		return ""
	}
	if ctx.Request != nil && ctx.Writer != nil {
		cookie := &stdhttp.Cookie{
			Name:     dubi.AuthCookieName(),
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: stdhttp.SameSiteLaxMode,
			MaxAge:   int(dubi.AuthTTL() / time.Second),
			Secure:   ctx.Request.TLS != nil,
		}
		ctx.SetCookie(cookie)
	}
	return token
}

// Sign issues a signed but unauthenticated Token using the current
// Router.Kind. It is useful for callbacks and other routes that require
// integrity without a logged-in identity.
func (ctx *Context) Sign(payload ...Map) string {
	var value Map
	if len(payload) > 0 {
		value = payload[0]
	}
	token, err := ctx.SignToken(dubi.Token{
		Kind:    ctx.Config.Kind,
		Payload: value,
	})
	if err != nil {
		ctx.Error(dubi.ErrorResult(err))
		return ""
	}
	return token
}

// String resolves a translation using the request language.
func (ctx *Context) String(key string, args ...any) string {
	return dubi.String(ctx.Language(), key, args...)
}

func (ctx *Context) Status(code int) {
	ctx.Code = code
}

// StatusCode returns the response status visible to the current pipeline.
// Before the response is committed it predicts net/http's default status so
// Response filters can emit consistent access logs and metrics.
func (ctx *Context) StatusCode() int {
	if ctx == nil {
		return 0
	}
	if ctx.output != nil && ctx.output.code != 0 {
		return ctx.output.code
	}
	if ctx.Code != 0 {
		return ctx.Code
	}
	if ctx.Body == nil {
		return stdhttp.StatusNoContent
	}
	return stdhttp.StatusOK
}

// RouteUri builds a relative URI from a registered route name.
func (ctx *Context) RouteUri(name string, values ...Map) string {
	if ctx.routes == nil {
		return name
	}
	return ctx.routes.uri(ctx, name, values...)
}

// RouteUrl builds an absolute URL from a registered route name.
func (ctx *Context) RouteUrl(name string, values ...Map) string {
	if ctx.routes == nil {
		return name
	}
	return ctx.routes.url(ctx, name, values...)
}

// SiteUrl builds an absolute URL for a site and path.
func (ctx *Context) SiteUrl(site, path string, options ...Map) string {
	if ctx.routes == nil {
		return path
	}
	return ctx.routes.siteURL(ctx, site, path, options...)
}

// Next continues the current HTTP filter/action chain.
func (ctx *Context) Next() {
	if ctx.index >= len(ctx.nexts) {
		return
	}
	next := ctx.nexts[ctx.index]
	ctx.index++
	if next != nil {
		next(ctx)
	}
}

// Abort stops the current HTTP filter/action chain.
func (ctx *Context) Abort() { ctx.index = len(ctx.nexts) }

func (ctx *Context) Error(results ...Res) {
	ctx.outcome("error", stdhttp.StatusInternalServerError, "error", results...)
}

func (ctx *Context) NotFound() {
	ctx.outcome("notfound", stdhttp.StatusNotFound, "notfound", resultNotFound)
}

func (ctx *Context) Failed(results ...Res) {
	ctx.outcome("failed", stdhttp.StatusBadRequest, "failed", results...)
}

func (ctx *Context) Unauthed(results ...Res) {
	ctx.outcome("unauthed", stdhttp.StatusUnauthorized, "unauthed", results...)
}

func (ctx *Context) Unsigned(results ...Res) {
	ctx.outcome("unsigned", stdhttp.StatusUnauthorized, "unsigned", results...)
}

func (ctx *Context) Denied(results ...Res) {
	ctx.outcome("denied", stdhttp.StatusForbidden, "denied", results...)
}

func (ctx *Context) outcome(kind string, httpCode int, status string, results ...Res) {
	result := defaultOutcomeResult(status)
	if len(results) > 0 && results[0] != nil {
		result = results[0]
	}
	ctx.Result(result)
	ctx.Code = httpCode
	ctx.Body = ctx.String(result.Status(), result.Args()...)
	ctx.handling = kind
	ctx.Abort()
}

func defaultOutcomeResult(status string) Res {
	switch status {
	case "notfound":
		return resultNotFound
	case "error":
		return resultError
	case "failed":
		return resultFailed
	case "unsigned":
		return resultUnsigned
	case "unauthed":
		return resultUnauthed
	case "denied":
		return resultDenied
	default:
		return dubi.ResultOf(1, status)
	}
}

// NewContext creates a Context for an existing HTTP request using the process
// runtime.
func NewContext(writer stdhttp.ResponseWriter, request *stdhttp.Request) *Context {
	return initContext(nil, writer, request)
}

func acquireContext(writer stdhttp.ResponseWriter, request *stdhttp.Request) *Context {
	return initContext(contextPool.Get().(*Context), writer, request)
}

func initContext(ctx *Context, writer stdhttp.ResponseWriter, request *stdhttp.Request) *Context {
	if ctx == nil {
		ctx = &Context{}
	}
	language := ""
	ajax := request.Header.Get("X-Requested-With") != "" || request.Header.Get("Ajax") != ""
	language = dubi.DetectLanguage(request.Header.Get("Accept-Language"))
	meta := (&ctx.metaValue).Reset(request.Context())
	meta.Language(language)
	output := responseWriterPool.Get().(*responseWriter)
	output.ResponseWriter = writer
	output.code = 0
	output.Header()["X-Request-ID"] = []string{meta.Id()}
	ctx.Meta = meta
	ctx.Request = request
	ctx.Writer = output
	ctx.StartedAt = time.Now()
	ctx.output = output
	ctx.Method = request.Method
	ctx.Host = request.Host
	ctx.Site = ""
	ctx.Server = ""
	ctx.Ajax = ajax
	ctx.Path = request.URL.Path
	ctx.Uri = request.RequestURI
	ctx.RemoteIP = requestPeerIP(request.RemoteAddr)
	ctx.Params = resetMap(ctx.Params)
	ctx.Query = resetMap(ctx.Query)
	ctx.Form = resetMap(ctx.Form)
	ctx.Value = resetMap(ctx.Value)
	ctx.Args = resetMap(ctx.Args)
	if ctx.Files == nil {
		ctx.Files = make(map[string][]*multipart.FileHeader)
	} else {
		clear(ctx.Files)
	}
	ctx.Data = resetMap(ctx.Data)
	ctx.Setting = resetMap(ctx.Setting)
	ctx.Config = Router{}
	ctx.Locals = resetMap(ctx.Locals)
	ctx.nexts = nil
	ctx.index = 0
	ctx.handling = ""
	ctx.responseRan = false
	ctx.wrote = false
	ctx.responseVars = nil
	ctx.responsePlan = nil
	ctx.routeHandler = nil
	ctx.routePattern = ""
	ctx.streams = nil
	ctx.routes = nil
	ctx.Body = nil
	ctx.Code = 0
	return ctx
}

func resetMap(value Map) Map {
	if value == nil {
		return Map{}
	}
	clear(value)
	return value
}

func assignMap(target, source Map) Map {
	target = resetMap(target)
	for key, value := range source {
		target[key] = value
	}
	return target
}

func releaseContext(ctx *Context) {
	if ctx == nil {
		return
	}
	ctx.closeStreams()
	if ctx.output != nil {
		output := ctx.output
		output.ResponseWriter = nil
		output.code = 0
		responseWriterPool.Put(output)
	}
	if ctx.Meta != nil {
		ctx.Meta.WithContext(context.Background())
		ctx.Meta.TraceId("")
		ctx.Meta.SpanId("")
		ctx.Meta.Language("")
		ctx.Meta.Token("")
		ctx.Meta.Result(nil)
	}
	ctx.Meta = nil
	ctx.Request = nil
	ctx.Writer = nil
	ctx.StartedAt = time.Time{}
	ctx.output = nil
	ctx.Config = Router{}
	ctx.Body = nil
	ctx.Data = resetMap(ctx.Data)
	ctx.Setting = resetMap(ctx.Setting)
	ctx.Locals = resetMap(ctx.Locals)
	ctx.Value = resetMap(ctx.Value)
	ctx.Args = resetMap(ctx.Args)
	ctx.Params = resetMap(ctx.Params)
	ctx.Query = resetMap(ctx.Query)
	ctx.Form = resetMap(ctx.Form)
	clear(ctx.Files)
	ctx.responseVars = nil
	ctx.responsePlan = nil
	ctx.routeHandler = nil
	ctx.routePattern = ""
	ctx.routes = nil
	ctx.nexts = nil
	contextPool.Put(ctx)
}

var uriParameterPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.)?\}`)

func pathParams(request *stdhttp.Request, uri string) Map {
	params := Map{}
	pathParamsInto(params, request, compilePathParameters(uri))
	return params
}

type pathParameter struct {
	name    string
	segment int
	multi   bool
}

func compilePathParameters(uri string) []pathParameter {
	if !strings.Contains(uri, "{") {
		return nil
	}
	segments := strings.Split(strings.TrimPrefix(uri, "/"), "/")
	parameters := make([]pathParameter, 0, strings.Count(uri, "{"))
	pathIndex := 0
	for _, segment := range segments {
		matches := uriParameterPattern.FindStringSubmatch(segment)
		if len(matches) == 2 {
			multi := strings.HasSuffix(segment, "...}")
			parameters = append(parameters, pathParameter{
				name:    matches[1],
				segment: pathIndex,
				multi:   multi,
			})
			if !multi {
				pathIndex++
			}
			continue
		}
		if segment != "" {
			pathIndex++
		}
	}
	return parameters
}

// pathParamsInto applies wildcard values compiled during route registration.
// It mirrors ServeMux path values while avoiding a regular-expression scan of
// the route pattern on each request.
func pathParamsInto(params Map, request *stdhttp.Request, parameters []pathParameter) {
	if request == nil || request.URL == nil {
		return
	}
	path := request.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	path = strings.TrimPrefix(path, "/")
	cursor, segmentIndex := 0, 0
	for _, parameter := range parameters {
		for segmentIndex < parameter.segment && cursor <= len(path) {
			slash := strings.IndexByte(path[cursor:], '/')
			if slash < 0 {
				cursor = len(path) + 1
				break
			}
			cursor += slash + 1
			segmentIndex++
		}
		if cursor > len(path) {
			continue
		}
		var segment string
		if parameter.multi {
			segment = path[cursor:]
			cursor = len(path) + 1
		} else {
			slash := strings.IndexByte(path[cursor:], '/')
			if slash < 0 {
				segment = path[cursor:]
				cursor = len(path) + 1
			} else {
				segment = path[cursor : cursor+slash]
				cursor += slash + 1
			}
			segmentIndex++
		}
		value := unescapePathValue(segment)
		request.SetPathValue(parameter.name, value)
		if value != "" {
			params[parameter.name] = value
		}
	}
}

func unescapePathValue(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return unescaped
}

func queryValues(request *stdhttp.Request) Map {
	values := Map{}
	queryValuesInto(values, request)
	return values
}

func queryValuesInto(values Map, request *stdhttp.Request) {
	if request == nil || request.URL == nil || request.URL.RawQuery == "" {
		return
	}
	for key, items := range request.URL.Query() {
		if len(items) == 1 {
			assignInputValue(values, key, items[0])
		} else if len(items) > 1 {
			assignInputValue(values, key, append([]string(nil), items...))
		}
	}
}

func requestPeerIP(remote string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(remote)); err == nil {
		return host
	}
	return strings.TrimSpace(remote)
}

func assignInputValue(target Map, key string, value any) {
	tokens := inputPathTokens(key)
	if len(tokens) == 0 {
		return
	}
	assignInputTokens(target, tokens, value)
}

func inputPathTokens(key string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	tokens := make([]string, 0, 4)
	start := 0
	for index := 0; index < len(key); index++ {
		switch key[index] {
		case '.':
			if start < index {
				tokens = append(tokens, key[start:index])
			}
			start = index + 1
		case '[':
			if start < index {
				tokens = append(tokens, key[start:index])
			}
			end := strings.IndexByte(key[index+1:], ']')
			if end < 0 {
				return append(tokens, key[index:])
			}
			tokens = append(tokens, key[index+1:index+1+end])
			index += end + 1
			start = index + 1
		}
	}
	if start < len(key) {
		tokens = append(tokens, key[start:])
	}
	return tokens
}

func assignInputTokens(target Map, tokens []string, value any) {
	if len(tokens) == 0 {
		return
	}
	key := tokens[0]
	if len(tokens) == 1 {
		if key == "" {
			return
		}
		target[key] = value
		return
	}
	rest := tokens[1:]
	if key == "" {
		return
	}
	if isArrayToken(rest[0]) {
		items, _ := target[key].([]any)
		if items == nil {
			items = []any{}
		}
		if rest[0] == "" {
			if len(rest) == 1 {
				items = appendInputArrayValue(items, value)
			} else {
				child := Map{}
				assignInputTokens(child, rest[1:], value)
				items = append(items, child)
			}
		} else if index, err := strconv.Atoi(rest[0]); err == nil && index >= 0 {
			for len(items) <= index {
				items = append(items, nil)
			}
			if len(rest) == 1 {
				items[index] = value
			} else {
				child, _ := items[index].(Map)
				if child == nil {
					child = Map{}
				}
				assignInputTokens(child, rest[1:], value)
				items[index] = child
			}
		}
		target[key] = items
		return
	}
	child, _ := target[key].(Map)
	if child == nil {
		child = Map{}
	}
	assignInputTokens(child, rest, value)
	target[key] = child
}

func isArrayToken(token string) bool {
	if token == "" {
		return true
	}
	_, err := strconv.Atoi(token)
	return err == nil
}

func appendInputArrayValue(items []any, value any) []any {
	switch values := value.(type) {
	case []string:
		for _, item := range values {
			items = append(items, item)
		}
	case []any:
		items = append(items, values...)
	default:
		items = append(items, value)
	}
	return items
}
