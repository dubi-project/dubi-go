package dubi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	. "github.com/dubi-project/dubi-go/base"
	"gopkg.in/yaml.v3"
)

var configFileNames = []string{
	"config.json", "config.toml", "config.yaml", "config.yml",
	"dubi.json", "dubi.toml", "dubi.yaml", "dubi.yml",
}

// loadConfig loads the local startup configuration. A future Dubi Config
// Center source can be merged here without changing module configuration.
func (rt *Runtime) loadConfig() error {
	fileConfig, err := loadConfigFile()
	if err != nil {
		return err
	}
	config := fileConfig
	mergeConfig(config, loadConfigEnvironment())
	mergeConfig(config, loadConfigArguments())

	if section, ok := config["dubi"].(Map); ok {
		if rt.identity.Tenant == "" {
			rt.identity.Tenant = stringValue(section["tenant"])
		}
		if rt.identity.Role == "" {
			rt.identity.Role = stringValue(section["role"])
		}
		if !rt.profileSet {
			rt.identity.Profile = stringValue(section["profile"])
		}
		if !rt.nodeSet {
			rt.identity.Node = stringValue(section["node"])
		}
		if len(rt.bootstrap.Endpoints) == 0 {
			rt.bootstrap.Endpoints = configStringList(section["endpoints"])
		}
		if auth, ok := section["auth"].(Map); ok {
			if rt.bootstrap.Auth == nil {
				rt.bootstrap.Auth = Map{}
			}
			for key, value := range auth {
				if _, exists := rt.bootstrap.Auth[key]; !exists {
					rt.bootstrap.Auth[key] = value
				}
			}
		}
	}
	if rt.identity.Node == "" {
		rt.identity.Node = defaultNodeID()
	}
	identity := Map{}
	if section, ok := config["dubi"].(Map); ok {
		identity = section
	} else {
		config["dubi"] = identity
	}
	identity["tenant"] = rt.identity.Tenant
	identity["role"] = rt.identity.Role
	identity["profile"] = rt.identity.Profile
	identity["node"] = rt.identity.Node
	rt.config = config
	rt.configLoaded = true
	return nil
}

func loadConfigFile() (Map, error) {
	path := configPath()
	if path == "" {
		return Map{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dubi: read config %q: %w", path, err)
	}
	config, err := decodeConfig(data, filepath.Ext(path))
	if err != nil {
		return nil, fmt.Errorf("dubi: decode config %q: %w", path, err)
	}
	return config, nil
}

func configPath() string {
	if path := commandOption("config"); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("DUBI_CONFIG")); path != "" {
		return path
	}
	process := strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0]))
	candidates := append([]string{}, configFileNames...)
	if process != "" {
		for _, ext := range []string{".json", ".toml", ".yaml", ".yml"} {
			candidates = append(candidates, process+ext)
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func decodeConfig(data []byte, extension string) (Map, error) {
	var value any
	var err error
	switch strings.ToLower(extension) {
	case ".json":
		err = json.Unmarshal(data, &value)
	case ".toml":
		err = toml.Unmarshal(data, &value)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, &value)
	default:
		return nil, fmt.Errorf("unsupported config format %q", extension)
	}
	if err != nil {
		return nil, err
	}
	result, ok := normalizeConfigValue(value).(Map)
	if !ok {
		return nil, fmt.Errorf("config root must be an object")
	}
	return result, nil
}

func normalizeConfigValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := Map{}
		for key, value := range item {
			result[key] = normalizeConfigValue(value)
		}
		return result
	case map[any]any:
		result := Map{}
		for key, value := range item {
			result[fmt.Sprint(key)] = normalizeConfigValue(value)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, value := range item {
			result[i] = normalizeConfigValue(value)
		}
		return result
	default:
		return value
	}
}

// cloneConfigMap makes a detached configuration snapshot. Configuration is
// dynamic by design, so a shallow map copy would still let one module mutate a
// nested section observed by another module or by Runtime.Config.
func cloneConfigMap(value Map) Map {
	if value == nil {
		return nil
	}
	out := make(Map, len(value))
	for key, item := range value {
		out[key] = cloneConfigValue(item)
	}
	return out
}

func cloneConfigValue(value any) any {
	switch item := value.(type) {
	case Map:
		return cloneConfigMap(item)
	case map[string]any:
		out := make(map[string]any, len(item))
		for key, value := range item {
			out[key] = cloneConfigValue(value)
		}
		return out
	case map[any]any:
		out := make(map[any]any, len(item))
		for key, value := range item {
			out[key] = cloneConfigValue(value)
		}
		return out
	case []any:
		out := make([]any, len(item))
		for index, value := range item {
			out[index] = cloneConfigValue(value)
		}
		return out
	case []string:
		return append([]string(nil), item...)
	case []Map:
		out := make([]Map, len(item))
		for index, value := range item {
			out[index] = cloneConfigMap(value)
		}
		return out
	default:
		return value
	}
}

func mergeConfig(dst, src Map) {
	for key, value := range src {
		if left, ok := dst[key].(Map); ok {
			if right, ok := value.(Map); ok {
				mergeConfig(left, right)
				continue
			}
		}
		dst[key] = value
	}
}

func loadConfigEnvironment() Map {
	result := Map{}
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "DUBI_CONFIG_") {
			continue
		}
		setConfigPath(result, strings.TrimPrefix(parts[0], "DUBI_CONFIG_"), parseConfigValue(parts[1]))
	}
	return result
}

func loadConfigArguments() Map {
	result := Map{}
	for i, arg := range os.Args[1:] {
		if !strings.HasPrefix(arg, "--config.") && !strings.HasPrefix(arg, "--config-") {
			continue
		}
		key, value := "", ""
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			key, value = parts[0], parts[1]
		} else if i+1 < len(os.Args[1:]) {
			key, value = arg, os.Args[1:][i+1]
		}
		key = strings.TrimPrefix(strings.TrimPrefix(key, "--config."), "--config-")
		key = strings.ReplaceAll(key, "-", ".")
		if key != "" {
			setConfigPath(result, key, parseConfigValue(value))
		}
	}
	return result
}

func commandOption(name string) string {
	args := os.Args[1:]
	for i, arg := range args {
		if arg == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "--"+name+"=") {
			return strings.TrimPrefix(arg, "--"+name+"=")
		}
	}
	return ""
}

func setConfigPath(target Map, path string, value any) {
	parts := strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '_' || r == '.' })
	if len(parts) == 0 {
		return
	}
	current := target
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part].(Map)
		if !ok {
			next = Map{}
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = value
}

func parseConfigValue(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		return parsed
	}
	return value
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

// configuredProfiles reads dynamic Profile definitions from configuration.
// The top-level "profiles" section is preferred; "profile" and the nested
// dubi.profiles section are accepted as compatible forms for small configs.
func configuredProfiles(config Map) (map[string]Profile, error) {
	profiles := make(map[string]Profile)
	if section, ok := configMap(config["dubi"]); ok {
		for _, key := range []string{"profile", "profiles"} {
			if raw, exists := profileField(section, key); exists {
				if _, isSection := configMap(raw); !isSection {
					continue
				}
				if err := decodeProfileSection(profiles, raw); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, key := range []string{"profile", "profiles"} {
		raw, exists := config[key]
		if !exists {
			continue
		}
		if _, ok := configMap(raw); !ok {
			continue
		}
		if err := decodeProfileSection(profiles, raw); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func decodeProfileSection(target map[string]Profile, raw any) error {
	section, ok := configMap(raw)
	if !ok {
		return fmt.Errorf("profiles must be an object")
	}
	for rawName, rawProfile := range section {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return fmt.Errorf("profile name is empty")
		}
		profile, err := decodeProfile(name, rawProfile)
		if err != nil {
			return err
		}
		target[name] = profile
	}
	return nil
}

func decodeProfile(name string, raw any) (Profile, error) {
	if text, ok := raw.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return Profile{}, nil
		}
		return Profile{Include: []string{text}}, nil
	}
	if _, ok := raw.([]any); ok {
		include, err := profilePatterns(name, "include", raw)
		return Profile{Include: include}, err
	}
	if values, ok := raw.([]string); ok {
		include := cleanProfilePatterns(values)
		return Profile{Include: include}, nil
	}
	section, ok := configMap(raw)
	if !ok {
		return Profile{}, fmt.Errorf("profile %q must be an object, string, or list", name)
	}
	profile := Profile{}
	for _, field := range []struct {
		name    string
		aliases []string
		target  *[]string
	}{
		{name: "include", aliases: []string{"include", "includes"}, target: &profile.Include},
		{name: "exclude", aliases: []string{"exclude", "excludes"}, target: &profile.Exclude},
		{name: "profiles", aliases: []string{"profiles", "extends", "parents"}, target: &profile.Profiles},
	} {
		rawValue, exists := profileField(section, field.aliases...)
		if !exists {
			continue
		}
		values, err := profilePatterns(name, field.name, rawValue)
		if err != nil {
			return Profile{}, err
		}
		*field.target = values
	}
	return profile, nil
}

func profilePatterns(profileName, field string, raw any) ([]string, error) {
	switch values := raw.(type) {
	case string:
		value := strings.TrimSpace(values)
		if value == "" {
			return nil, nil
		}
		return []string{value}, nil
	case []string:
		return cleanProfilePatterns(values), nil
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("profile %q field %q must contain strings", profileName, field)
			}
			if text = strings.TrimSpace(text); text != "" {
				result = append(result, text)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("profile %q field %q must be a string or list", profileName, field)
	}
}

func cleanProfilePatterns(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func configMap(value any) (Map, bool) {
	switch item := value.(type) {
	case Map:
		return item, true
	case map[string]any:
		return normalizeConfigValue(item).(Map), true
	default:
		return nil, false
	}
}

func profileField(section Map, names ...string) (any, bool) {
	for _, name := range names {
		for key, value := range section {
			if strings.EqualFold(key, name) {
				return value, true
			}
		}
	}
	return nil, false
}

func configStringList(value any) []string {
	switch item := value.(type) {
	case string:
		return cleanEndpoints([]string{item})
	case []any:
		values := make([]string, 0, len(item))
		for _, value := range item {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
		return cleanEndpoints(values)
	case []string:
		return cleanEndpoints(item)
	default:
		return nil
	}
}
