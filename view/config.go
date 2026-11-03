package view

import (
	"fmt"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

type Config struct {
	Root     string
	Shared   string
	Left     string
	Right    string
	Cache    bool
	Settings Map
}

func defaultConfig() Config {
	return Config{Root: "asset/views", Shared: "shared", Left: "{%", Right: "%}", Cache: true}
}

func parseConfig(global Map) (Config, error) {
	config := defaultConfig()
	raw, exists := global["view"]
	if !exists {
		return config, nil
	}
	section, ok := asMap(raw)
	if !ok {
		return config, fmt.Errorf("view: config must be an object")
	}
	for key, field := range map[string]*string{"root": &config.Root, "shared": &config.Shared, "left": &config.Left, "right": &config.Right} {
		if value, exists := section[key]; exists {
			text, ok := value.(string)
			if !ok {
				return config, fmt.Errorf("view: %s must be a string", key)
			}
			*field = text
		}
	}
	if strings.TrimSpace(config.Root) == "" || config.Left == "" || config.Right == "" {
		return config, fmt.Errorf("view: root and delimiters must not be empty")
	}
	if config.Shared != "" && !validName(config.Shared) {
		return config, fmt.Errorf("view: shared must be a relative resource path")
	}
	if value, exists := section["cache"]; exists {
		var ok bool
		config.Cache, ok = value.(bool)
		if !ok {
			return config, fmt.Errorf("view: cache must be a boolean")
		}
	}
	config.Settings = Map{}
	for _, key := range []string{"setting", "settings"} {
		if value, exists := section[key]; exists {
			settings, ok := asMap(value)
			if !ok {
				return config, fmt.Errorf("view: %s must be an object", key)
			}
			for name, value := range settings.Clone() {
				config.Settings[name] = value
			}
		}
	}
	return config, nil
}

func asMap(value any) (Map, bool) {
	switch value := value.(type) {
	case Map:
		return value, true
	case map[string]any:
		return Map(value), true
	default:
		return nil, false
	}
}
