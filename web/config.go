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
	Host              string
	Port              int
	Static            string
	Require           bool
	Setting           Map
	Cross             Map
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
	TLS               TLSConfig
	TrustedProxies    []string
	Sites             map[string]SiteConfig
	Servers           map[string]ServerConfig
}

// TLSConfig enables HTTPS for one Web server when both files are present.
type TLSConfig struct {
	Cert string
	Key  string
}

// ServerConfig describes one listener. Each server may host several Sites.
type ServerConfig struct {
	Name              string
	Host              string
	Port              int
	Static            string
	Setting           Map
	Cross             Map
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
	TLS               TLSConfig
	TrustedProxies    []string
	Sites             map[string]SiteConfig
}

// SiteConfig describes one virtual host group inside a ServerConfig.
type SiteConfig struct {
	Name    string
	Static  string
	Hosts   []string
	Aliases []string
	Setting Map
	Cross   Map
}

func (c Config) Clone() Config {
	copyOf := c
	copyOf.Static = c.Static
	copyOf.Setting = c.Setting.Clone()
	copyOf.Cross = c.Cross.Clone()
	copyOf.TrustedProxies = append([]string(nil), c.TrustedProxies...)
	copyOf.Sites = cloneSites(c.Sites)
	copyOf.Servers = cloneServers(c.Servers)
	return copyOf
}

func (c ServerConfig) Clone() ServerConfig {
	copyOf := c
	copyOf.Static = c.Static
	copyOf.Setting = c.Setting.Clone()
	copyOf.Cross = c.Cross.Clone()
	copyOf.TrustedProxies = append([]string(nil), c.TrustedProxies...)
	copyOf.Sites = cloneSites(c.Sites)
	return copyOf
}

func (c SiteConfig) Clone() SiteConfig {
	copyOf := c
	copyOf.Static = c.Static
	copyOf.Hosts = append([]string(nil), c.Hosts...)
	copyOf.Aliases = append([]string(nil), c.Aliases...)
	copyOf.Setting = c.Setting.Clone()
	copyOf.Cross = c.Cross.Clone()
	return copyOf
}

func parseConfig(section Map) (Config, error) {
	config := Config{
		Host:              stringValue(section["host"]),
		Port:              intValue(section["port"]),
		Static:            stringValue(section["static"]),
		Require:           boolValue(section["require"]),
		Setting:           mapValue(section["setting"]),
		Cross:             mapValue(section["cross"]),
		ReadTimeout:       durationValue(section["read_timeout"]),
		ReadHeaderTimeout: durationValue(section["read_header_timeout"]),
		WriteTimeout:      durationValue(section["write_timeout"]),
		IdleTimeout:       durationValue(section["idle_timeout"]),
		RequestTimeout:    durationValue(section["request_timeout"]),
		ShutdownTimeout:   durationValue(section["shutdown_timeout"]),
		MaxHeaderBytes:    intValue(section["max_header_bytes"]),
		MaxBodyBytes:      int64Value(section["max_body_bytes"]),
		MaxConcurrency:    intValue(section["max_concurrency"]),
		Compress:          boolValue(section["compress"]),
		TLS:               parseTLS(section["tls"]),
		TrustedProxies:    stringList(firstPresent(section, "trusted_proxies", "trusted_proxy")),
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
		server := ServerConfig{
			Name:              name,
			Host:              firstString(serverMap["host"], defaults.Host),
			Port:              firstInt(serverMap["port"], defaults.Port),
			Static:            firstString(serverMap["static"], defaults.Static),
			Setting:           mergeMap(defaults.Setting, mapValue(serverMap["setting"])),
			Cross:             mergeMap(defaults.Cross, mapValue(serverMap["cross"])),
			ReadTimeout:       firstDuration(serverMap["read_timeout"], defaults.ReadTimeout),
			ReadHeaderTimeout: firstDuration(serverMap["read_header_timeout"], defaults.ReadHeaderTimeout),
			WriteTimeout:      firstDuration(serverMap["write_timeout"], defaults.WriteTimeout),
			IdleTimeout:       firstDuration(serverMap["idle_timeout"], defaults.IdleTimeout),
			RequestTimeout:    firstDuration(serverMap["request_timeout"], defaults.RequestTimeout),
			ShutdownTimeout:   firstDuration(serverMap["shutdown_timeout"], defaults.ShutdownTimeout),
			MaxHeaderBytes:    firstInt(serverMap["max_header_bytes"], defaults.MaxHeaderBytes),
			MaxBodyBytes:      firstInt64(serverMap["max_body_bytes"], defaults.MaxBodyBytes),
			MaxConcurrency:    firstInt(serverMap["max_concurrency"], defaults.MaxConcurrency),
			Compress:          firstBool(serverMap["compress"], defaults.Compress),
			TLS:               mergeTLS(defaults.TLS, parseTLS(serverMap["tls"])),
			TrustedProxies:    firstStringList(firstPresent(serverMap, "trusted_proxies", "trusted_proxy"), defaults.TrustedProxies),
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
			site.Hosts = stringList(firstPresent(siteMap, "hosts", "domains", "domain"))
			site.Static = stringValue(siteMap["static"])
			site.Aliases = stringList(firstPresent(siteMap, "aliases", "alias"))
			site.Setting = mapValue(siteMap["setting"])
			site.Cross = mapValue(siteMap["cross"])
		}
		sites[name] = site
	}
	return sites, nil
}

func defaultServerConfig(config Config) ServerConfig {
	return ServerConfig{
		Name:              defaultServerName,
		Host:              config.Host,
		Port:              config.Port,
		Static:            config.Static,
		Setting:           config.Setting.Clone(),
		Cross:             config.Cross.Clone(),
		ReadTimeout:       config.ReadTimeout,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
		WriteTimeout:      config.WriteTimeout,
		IdleTimeout:       config.IdleTimeout,
		RequestTimeout:    config.RequestTimeout,
		ShutdownTimeout:   config.ShutdownTimeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
		MaxBodyBytes:      config.MaxBodyBytes,
		MaxConcurrency:    config.MaxConcurrency,
		Compress:          config.Compress,
		TLS:               config.TLS,
		TrustedProxies:    append([]string(nil), config.TrustedProxies...),
		Sites:             cloneSites(config.Sites),
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
		if server.ReadTimeout < 0 || server.ReadHeaderTimeout < 0 || server.WriteTimeout < 0 || server.IdleTimeout < 0 || server.RequestTimeout < 0 || server.ShutdownTimeout < 0 {
			return fmt.Errorf("web server %q has a negative timeout", name)
		}
		if server.MaxHeaderBytes < 0 || server.MaxBodyBytes < 0 || server.MaxConcurrency < 0 {
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
