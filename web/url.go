package web

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

type routeInfo struct {
	name   string
	uri    string
	server string
	site   string
}

type routeRegistry struct {
	config ServerConfig
	routes map[string]routeInfo
}

func newRouteRegistry(config ServerConfig) *routeRegistry {
	return &routeRegistry{config: config, routes: make(map[string]routeInfo)}
}

func (r *routeRegistry) add(original, compiled, route, uri, site string) {
	info := routeInfo{name: route, uri: uri, server: r.config.Name, site: site}
	for _, key := range []string{original, compiled, route} {
		key = strings.ToLower(strings.TrimSpace(key))
		if key != "" {
			if _, exists := r.routes[key]; exists {
				continue
			}
			r.routes[key] = info
		}
	}
	// Method-specific expansion should still allow the base route name.
	if dot := strings.LastIndex(compiled, "."); dot > 0 {
		base := compiled[:dot]
		if _, exists := r.routes[base]; !exists {
			r.routes[base] = info
		}
	}
}

func (r *routeRegistry) lookup(ctx *Context, name string, options Map) (routeInfo, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = strings.ToLower(ctx.Name)
	}
	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		return routeInfo{uri: name}, true
	}
	if !strings.Contains(name, ".") {
		if ctx.Site != "" {
			if _, ok := r.routes[ctx.Site+"."+name]; ok {
				name = ctx.Site + "." + name
			}
		}
	}
	if site, ok := options["[site]"].(string); ok && site != "" && !strings.Contains(name, ".") {
		name = strings.ToLower(site) + "." + name
	}
	info, ok := r.routes[name]
	return info, ok
}

func (r *routeRegistry) uri(ctx *Context, name string, values ...Map) string {
	if strings.HasPrefix(strings.ToLower(name), "http://") || strings.HasPrefix(strings.ToLower(name), "https://") {
		return name
	}
	options, params, query := splitURLValues(values...)
	info, ok := r.lookup(ctx, name, options)
	if !ok {
		return name
	}
	path := expandRoutePath(info.uri, params)
	return addURLQuery(path, query)
}

func (r *routeRegistry) url(ctx *Context, name string, values ...Map) string {
	options, params, query := splitURLValues(values...)
	info, ok := r.lookup(ctx, name, options)
	if !ok {
		return name
	}
	if strings.HasPrefix(strings.ToLower(info.uri), "http://") || strings.HasPrefix(strings.ToLower(info.uri), "https://") {
		return info.uri
	}
	path := addURLQuery(expandRoutePath(info.uri, params), query)
	return r.siteURLForInfo(ctx, info, path, options)
}

func (r *routeRegistry) siteURL(ctx *Context, site, path string, options ...Map) string {
	values := Map{}
	if len(options) > 0 && options[0] != nil {
		values = options[0]
	}
	info := routeInfo{server: r.config.Name, site: strings.ToLower(strings.TrimSpace(site))}
	return r.siteURLForInfo(ctx, info, path, values)
}

func (r *routeRegistry) siteURLForInfo(ctx *Context, info routeInfo, path string, options Map) string {
	scheme := "http"
	if ctx.Request != nil && ctx.Request.TLS != nil {
		scheme = "https"
	}
	if value, ok := options["[ssl]"].(bool); ok && value {
		scheme = "https"
	}
	if value, ok := options["[socket]"].(bool); ok && value {
		if scheme == "https" {
			scheme = "wss"
		} else {
			scheme = "ws"
		}
	}
	host := r.hostFor(ctx, info.site)
	if host == "" {
		host = "localhost"
	}
	if path == "" {
		return scheme + "://" + host
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

func (r *routeRegistry) hostFor(ctx *Context, site string) string {
	if site == "" || site == ctx.Site {
		if ctx.Host != "" {
			return normalizeURLHost(ctx.Host)
		}
	}
	if site != "" {
		if config, ok := r.config.Sites[site]; ok {
			for _, host := range append(append([]string{}, config.Hosts...), config.Aliases...) {
				if host != "" && !strings.Contains(host, "*") {
					return addPort(host, r.config.Port)
				}
			}
		}
	}
	host := r.config.Host
	if host == "" || host == "0.0.0.0" || host == "::" {
		if ctx.Host != "" {
			return normalizeURLHost(ctx.Host)
		}
		host = "localhost"
	}
	return addPort(host, r.config.Port)
}

func splitURLValues(values ...Map) (options, params, query Map) {
	options, params, query = Map{}, Map{}, Map{}
	if len(values) == 0 || values[0] == nil {
		return
	}
	for key, value := range values[0] {
		switch {
		case strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]"):
			options[key] = value
		case strings.HasPrefix(key, "{") && strings.HasSuffix(key, "}"):
			params[strings.TrimSuffix(strings.TrimPrefix(key, "{"), "}")] = value
		default:
			if strings.HasPrefix(key, "_") {
				params[strings.TrimPrefix(key, "_")] = value
			} else {
				query[key] = value
			}
		}
	}
	return
}

func expandRoutePath(pattern string, params Map) string {
	for key, value := range params {
		text := fmt.Sprint(value)
		pattern = strings.ReplaceAll(pattern, "{"+key+"}", url.PathEscape(text))
		pattern = strings.ReplaceAll(pattern, "{"+key+"...}", escapeMultiPath(text))
	}
	return pattern
}

func escapeMultiPath(value string) string {
	parts := strings.Split(value, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func addURLQuery(path string, values Map) string {
	if len(values) == 0 {
		return path
	}
	query := url.Values{}
	for key, value := range values {
		switch item := value.(type) {
		case []string:
			for _, part := range item {
				query.Add(key, part)
			}
		case []any:
			for _, part := range item {
				query.Add(key, fmt.Sprint(part))
			}
		default:
			query.Set(key, fmt.Sprint(value))
		}
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + query.Encode()
}

func normalizeURLHost(host string) string {
	host = strings.TrimSpace(host)
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		return net.JoinHostPort(parsed, strings.TrimSpace(fmt.Sprint(parsePort(host))))
	}
	return strings.TrimSuffix(host, ".")
}

func parsePort(host string) int {
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		return 0
	}
	var value int
	_, _ = fmt.Sscanf(port, "%d", &value)
	return value
}

func addPort(host string, port int) string {
	if port == 0 || strings.Contains(host, ":") {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}
