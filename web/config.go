package web

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

// Config is the HTTP module configuration. Direct fields describe the
// implicit default server; Servers describes the explicit multi-server form.
type Config struct {
	Host     string
	Port     int
	Domains  []string
	Static   string
	Require  bool
	Settings Map
	Cross    Map
	Options  ServerOptions
	TLS      TLSConfig
	Sites    map[string]SiteConfig
	Servers  map[string]ServerConfig
}

// ServerOptions holds server timeouts, request limits, and transport behavior.
// Root options are defaults; each server can override individual fields.
type ServerOptions struct {
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	RequestTimeout    time.Duration
	ShutdownTimeout   time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
	MaxConcurrency    int
	Compress          bool
	TrustedProxies    []string
}

// TLSConfig enables HTTPS for one Web server when both files are present.
type TLSConfig struct {
	Cert string
	Key  string
}

// ServerConfig describes one listener. Each server may host several Sites.
type ServerConfig struct {
	Name     string
	Host     string
	Port     int
	Domains  []string
	Static   string
	Settings Map
	Cross    Map
	Options  ServerOptions
	TLS      TLSConfig
	Sites    map[string]SiteConfig
}

// SiteConfig matches request domains inside one listener. Host in Config and
// ServerConfig is exclusively the listener binding address.
type SiteConfig struct {
	Name     string
	Static   string
	Domains  []string
	Aliases  []string
	Settings Map
	Cross    Map
}

func (c Config) Clone() Config {
	copyOf := c
	copyOf.Domains = append([]string(nil), c.Domains...)
	copyOf.Static = c.Static
	copyOf.Settings = c.Settings.Clone()
	copyOf.Cross = c.Cross.Clone()
	copyOf.Options = c.Options.Clone()
	copyOf.Sites = cloneSites(c.Sites)
	copyOf.Servers = cloneServers(c.Servers)
	return copyOf
}

func (c ServerConfig) Clone() ServerConfig {
	copyOf := c
	copyOf.Domains = append([]string(nil), c.Domains...)
	copyOf.Static = c.Static
	copyOf.Settings = c.Settings.Clone()
	copyOf.Cross = c.Cross.Clone()
	copyOf.Options = c.Options.Clone()
	copyOf.Sites = cloneSites(c.Sites)
	return copyOf
}

func (c SiteConfig) Clone() SiteConfig {
	copyOf := c
	copyOf.Static = c.Static
	copyOf.Domains = append([]string(nil), c.Domains...)
	copyOf.Aliases = append([]string(nil), c.Aliases...)
	copyOf.Settings = c.Settings.Clone()
	copyOf.Cross = c.Cross.Clone()
	return copyOf
}

func (o ServerOptions) Clone() ServerOptions {
	o.TrustedProxies = append([]string(nil), o.TrustedProxies...)
	return o
}

// configGroup merges legacy singular keys before canonical plural keys. A
// plural value wins for the same field; unrelated legacy fields are preserved.
func configGroup(section Map, plural, singular string) (Map, error) {
	var result Map
	for _, name := range []string{singular, plural} {
		if value, exists := section[name]; exists {
			fields, ok := asMap(value)
			if !ok {
				return nil, fmt.Errorf("web.%s must be an object", name)
			}
			result = mergeMap(result, fields)
		}
	}
	return result, nil
}

var serverOptionNames = []string{
	"read_timeout", "read_header_timeout", "write_timeout", "idle_timeout",
	"request_timeout", "shutdown_timeout", "max_header_bytes", "max_body_bytes",
	"max_concurrency", "compress", "trusted_proxies",
}

func parseOptions(section Map, defaults ServerOptions) (ServerOptions, error) {
	// Old flat keys remain valid, but nested option/options fields take priority.
	fields := Map{}
	for _, name := range serverOptionNames {
		if value, exists := section[name]; exists {
			fields[name] = value
		}
	}
	if value, exists := section["trusted_proxy"]; exists {
		if _, present := fields["trusted_proxies"]; !present {
			fields["trusted_proxies"] = value
		}
	}
	for _, name := range []string{"option", "options"} {
		if value, exists := section[name]; exists {
			overlay, ok := asMap(value)
			if !ok {
				return ServerOptions{}, fmt.Errorf("web.%s must be an object", name)
			}
			for key, value := range overlay {
				if key == "trusted_proxy" {
					if _, present := overlay["trusted_proxies"]; present {
						continue
					}
					key = "trusted_proxies"
				}
				known := false
				for _, candidate := range serverOptionNames {
					if key == candidate {
						known = true
						break
					}
				}
				if !known {
					return ServerOptions{}, fmt.Errorf("web.%s: unknown field %q", name, key)
				}
				fields[key] = value
			}
		}
	}
	return ServerOptions{
		ReadTimeout:       firstDuration(fields["read_timeout"], defaults.ReadTimeout),
		ReadHeaderTimeout: firstDuration(fields["read_header_timeout"], defaults.ReadHeaderTimeout),
		WriteTimeout:      firstDuration(fields["write_timeout"], defaults.WriteTimeout),
		IdleTimeout:       firstDuration(fields["idle_timeout"], defaults.IdleTimeout),
		RequestTimeout:    firstDuration(fields["request_timeout"], defaults.RequestTimeout),
		ShutdownTimeout:   firstDuration(fields["shutdown_timeout"], defaults.ShutdownTimeout),
		MaxHeaderBytes:    int(firstInt64(fields["max_header_bytes"], int64(defaults.MaxHeaderBytes))),
		MaxBodyBytes:      firstInt64(fields["max_body_bytes"], defaults.MaxBodyBytes),
		MaxConcurrency:    int(firstInt64(fields["max_concurrency"], int64(defaults.MaxConcurrency))),
		Compress:          firstBool(fields["compress"], defaults.Compress),
		TrustedProxies:    firstStringList(fields["trusted_proxies"], defaults.TrustedProxies),
	}, nil
}

func parseConfig(section Map) (Config, error) {
	domains, _, err := configuredDomains(section, "domains", "domain")
	if err != nil {
		return Config{}, fmt.Errorf("web: %w", err)
	}
	options, err := parseOptions(section, ServerOptions{})
	if err != nil {
		return Config{}, err
	}
	settings, err := configGroup(section, "settings", "setting")
	if err != nil {
		return Config{}, err
	}
	config := Config{
		Host:     stringValue(section["host"]),
		Port:     intValue(section["port"]),
		Domains:  domains,
		Static:   stringValue(section["static"]),
		Require:  boolValue(section["require"]),
		Settings: settings,
		Cross:    mapValue(section["cross"]),
		Options:  options,
		TLS:      parseTLS(section["tls"]),
	}
	if raw, ok := section["sites"]; ok {
		sites, err := parseSites(raw)
		if err != nil {
			return Config{}, fmt.Errorf("web sites: %w", err)
		}
		config.Sites = sites
	}
	if raw, ok := section["servers"]; ok {
		if len(config.Sites) > 0 {
			return Config{}, fmt.Errorf("web.sites cannot be combined with web.servers; put sites under a server")
		}
		servers, err := parseServers(raw, config)
		if err != nil {
			return Config{}, err
		}
		config.Servers = servers
	}
	return config, nil
}

func parseServers(raw any, defaults Config) (map[string]ServerConfig, error) {
	section, ok := asMap(raw)
	if !ok {
		return nil, fmt.Errorf("web.servers must be an object")
	}
	servers := make(map[string]ServerConfig, len(section))
	for rawName, rawValue := range section {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("web server name is empty")
		}
		serverMap, ok := asMap(rawValue)
		if !ok {
			return nil, fmt.Errorf("web server %q must be an object", name)
		}
		domains, present, err := configuredDomains(serverMap, "domains", "domain")
		if err != nil {
			return nil, fmt.Errorf("web server %q: %w", name, err)
		}
		if !present {
			domains = append([]string(nil), defaults.Domains...)
		}
		options, err := parseOptions(serverMap, defaults.Options)
		if err != nil {
			return nil, fmt.Errorf("web server %q: %w", name, err)
		}
		settings, err := configGroup(serverMap, "settings", "setting")
		if err != nil {
			return nil, fmt.Errorf("web server %q: %w", name, err)
		}
		server := ServerConfig{
			Name:     name,
			Host:     firstString(serverMap["host"], defaults.Host),
			Port:     firstInt(serverMap["port"], defaults.Port),
			Domains:  domains,
			Static:   firstString(serverMap["static"], defaults.Static),
			Settings: mergeMap(defaults.Settings, settings),
			Cross:    mergeMap(defaults.Cross, mapValue(serverMap["cross"])),
			Options:  options,
			TLS:      mergeTLS(defaults.TLS, parseTLS(serverMap["tls"])),
		}
		if rawSites, exists := serverMap["sites"]; exists {
			sites, err := parseSites(rawSites)
			if err != nil {
				return nil, fmt.Errorf("web server %q sites: %w", name, err)
			}
			server.Sites = sites
		}
		servers[name] = server
	}
	return servers, nil
}

func parseSites(raw any) (map[string]SiteConfig, error) {
	section, ok := asMap(raw)
	if !ok {
		return nil, fmt.Errorf("sites must be an object")
	}
	sites := make(map[string]SiteConfig, len(section))
	for rawName, rawValue := range section {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("site name is empty")
		}
		site := SiteConfig{Name: name}
		if rawValue != nil {
			siteMap, ok := asMap(rawValue)
			if !ok {
				return nil, fmt.Errorf("site %q must be an object", name)
			}
			domains, _, err := configuredDomains(siteMap, "domains", "domain", "hosts")
			if err != nil {
				return nil, fmt.Errorf("site %q: %w", name, err)
			}
			site.Domains = domains
			site.Static = stringValue(siteMap["static"])
			site.Aliases = stringList(firstPresent(siteMap, "aliases", "alias"))
			settings, err := configGroup(siteMap, "settings", "setting")
			if err != nil {
				return nil, fmt.Errorf("site %q: %w", name, err)
			}
			site.Settings = settings
			site.Cross = mapValue(siteMap["cross"])
		}
		sites[name] = site
	}
	return sites, nil
}

func defaultServerConfig(config Config) ServerConfig {
	return ServerConfig{
		Name:     defaultServerName,
		Host:     config.Host,
		Port:     config.Port,
		Domains:  append([]string(nil), config.Domains...),
		Static:   config.Static,
		Settings: config.Settings.Clone(),
		Cross:    config.Cross.Clone(),
		Options:  config.Options.Clone(),
		TLS:      config.TLS,
		Sites:    cloneSites(config.Sites),
	}
}

func validateServers(servers map[string]ServerConfig) error {
	seen := make(map[string]string, len(servers))
	for rawName, server := range servers {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return fmt.Errorf("web server name is empty")
		}
		if server.Port < 0 || server.Port > 65535 {
			return fmt.Errorf("web server %q has invalid port %d", name, server.Port)
		}
		if server.Options.ReadTimeout < 0 || server.Options.ReadHeaderTimeout < 0 || server.Options.WriteTimeout < 0 || server.Options.IdleTimeout < 0 || server.Options.RequestTimeout < 0 || server.Options.ShutdownTimeout < 0 {
			return fmt.Errorf("web server %q has a negative timeout", name)
		}
		if server.Options.MaxHeaderBytes < 0 || server.Options.MaxBodyBytes < 0 || server.Options.MaxConcurrency < 0 {
			return fmt.Errorf("web server %q has a negative request limit", name)
		}
		if (server.TLS.Cert == "") != (server.TLS.Key == "") {
			return fmt.Errorf("web server %q must configure both tls.cert and tls.key", name)
		}
		host := server.Host
		if host == "" {
			host = "0.0.0.0"
		}
		address := net.JoinHostPort(host, strconv.Itoa(server.Port))
		if previous, exists := seen[address]; exists {
			return fmt.Errorf("web servers %q and %q use the same address %s", previous, name, address)
		}
		seen[address] = name
	}
	return nil
}

func asMap(value any) (Map, bool) {
	switch item := value.(type) {
	case Map:
		return item, true
	case map[string]any:
		result := Map{}
		for key, value := range item {
			result[key] = value
		}
		return result, true
	default:
		return nil, false
	}
}

func mapValue(value any) Map {
	mapped, ok := asMap(value)
	if !ok {
		return nil
	}
	return mapped.Clone()
}

func mergeMap(base, overlay Map) Map {
	if base == nil && overlay == nil {
		return nil
	}
	result := base.Clone()
	if result == nil {
		result = Map{}
	}
	for key, value := range overlay {
		result[key] = value
	}
	return result
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intValue(value any) int {
	switch item := value.(type) {
	case int:
		return item
	case int64:
		return int(item)
	case float64:
		return int(item)
	case string:
		value, _ := strconv.Atoi(strings.TrimSpace(item))
		return value
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	switch item := value.(type) {
	case int:
		return int64(item)
	case int8:
		return int64(item)
	case int16:
		return int64(item)
	case int32:
		return int64(item)
	case int64:
		return item
	case uint:
		return int64(item)
	case uint64:
		return int64(item)
	case float64:
		return int64(item)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
		return parsed
	default:
		return 0
	}
}

func durationValue(value any) time.Duration {
	switch item := value.(type) {
	case time.Duration:
		return item
	case int:
		return time.Duration(item) * time.Second
	case int64:
		return time.Duration(item) * time.Second
	case float64:
		return time.Duration(item * float64(time.Second))
	case string:
		parsed, err := time.ParseDuration(strings.TrimSpace(item))
		if err == nil {
			return parsed
		}
	}
	return 0
}

func firstDuration(value any, fallback time.Duration) time.Duration {
	if value == nil {
		return fallback
	}
	return durationValue(value)
}

func firstInt64(value any, fallback int64) int64 {
	if value == nil {
		return fallback
	}
	return int64Value(value)
}

func firstBool(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return boolValue(value)
}

func firstStringList(value any, fallback []string) []string {
	if value == nil {
		return append([]string(nil), fallback...)
	}
	return stringList(value)
}

func parseTLS(value any) TLSConfig {
	section, ok := asMap(value)
	if !ok {
		return TLSConfig{}
	}
	return TLSConfig{
		Cert: firstString(section["cert"], stringValue(section["certificate"])),
		Key:  firstString(section["key"], stringValue(section["private_key"])),
	}
}

func mergeTLS(base, overlay TLSConfig) TLSConfig {
	if overlay.Cert != "" {
		base.Cert = overlay.Cert
	}
	if overlay.Key != "" {
		base.Key = overlay.Key
	}
	return base
}

func boolValue(value any) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(item))
		return parsed
	default:
		return false
	}
}

func firstString(value any, fallback string) string {
	if text := stringValue(value); text != "" {
		return text
	}
	return fallback
}

func firstInt(value any, fallback int) int {
	if parsed := intValue(value); parsed != 0 {
		return parsed
	}
	return fallback
}

func firstPresent(section Map, names ...string) any {
	for _, name := range names {
		if value, ok := section[name]; ok {
			return value
		}
	}
	return nil
}

func stringList(value any) []string {
	switch item := value.(type) {
	case string:
		if item = strings.TrimSpace(item); item != "" {
			return []string{item}
		}
	case []string:
		return cleanStrings(item)
	case []any:
		result := make([]string, 0, len(item))
		for _, value := range item {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
		return result
	}
	return nil
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func cloneSites(values map[string]SiteConfig) map[string]SiteConfig {
	if values == nil {
		return nil
	}
	result := make(map[string]SiteConfig, len(values))
	for name, value := range values {
		result[name] = value.Clone()
	}
	return result
}

func cloneServers(values map[string]ServerConfig) map[string]ServerConfig {
	if values == nil {
		return nil
	}
	result := make(map[string]ServerConfig, len(values))
	for name, value := range values {
		result[name] = value.Clone()
	}
	return result
}
