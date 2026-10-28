package web

import (
	"reflect"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func TestOptionsAliasesAndServerInheritance(t *testing.T) {
	config, err := parseConfig(Map{
		"read_timeout": "30s", // Legacy flat values are the lowest priority.
		"option":       Map{"read_timeout": "20s", "write_timeout": "40s"},
		"options": Map{
			"read_timeout": "10s", "read_header_timeout": "2s", "compress": true,
			"max_body_bytes": 4096, "max_concurrency": 16,
			"trusted_proxies": []any{"127.0.0.1"},
		},
		"servers": Map{
			"inherited": Map{"port": 8080},
			"overridden": Map{
				"port":    8081,
				"option":  Map{"read_header_timeout": "3s", "write_timeout": "50s", "trusted_proxy": []any{"10.0.0.1"}},
				"options": Map{"read_header_timeout": "4s", "read_timeout": 0, "max_body_bytes": 0, "max_concurrency": 0, "compress": false},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := config.Options
	if root.ReadTimeout != 10*time.Second || root.WriteTimeout != 40*time.Second {
		t.Fatalf("alias priority: %+v", root)
	}
	inherited := config.Servers["inherited"]
	if !reflect.DeepEqual(inherited.Options, root) {
		t.Fatalf("inherited options: %+v", inherited.Options)
	}
	options := config.Servers["overridden"].Options
	if options.ReadHeaderTimeout != 4*time.Second || options.WriteTimeout != 50*time.Second || options.ReadTimeout != 0 || options.MaxBodyBytes != 0 || options.MaxConcurrency != 0 || options.Compress {
		t.Fatalf("server overrides: %+v", options)
	}
	if !reflect.DeepEqual(options.TrustedProxies, []string{"10.0.0.1"}) {
		t.Fatalf("legacy proxy alias did not override inherited value: %v", options.TrustedProxies)
	}
	inherited.Options.TrustedProxies[0] = "changed"
	if config.Options.TrustedProxies[0] != "127.0.0.1" {
		t.Fatal("server shares proxy slice with root")
	}
	clone := config.Clone()
	clone.Options.TrustedProxies[0] = "cloned"
	if config.Options.TrustedProxies[0] != "127.0.0.1" {
		t.Fatal("Clone shares options with original")
	}
}

func TestSettingsAliasesMergeAcrossWebServerSite(t *testing.T) {
	config, err := parseConfig(Map{
		"setting":  Map{"legacy": true, "shared": "legacy"},
		"settings": Map{"root": true, "shared": "root"},
		"servers": Map{"default": Map{
			"port":     8080,
			"setting":  Map{"serverLegacy": true, "shared": "server-legacy"},
			"settings": Map{"server": true, "shared": "server"},
			"sites": Map{"api": Map{
				"setting":  Map{"siteLegacy": true, "shared": "site-legacy"},
				"settings": Map{"site": true, "shared": "site"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Settings["shared"] != "root" {
		t.Fatalf("plural settings lost: %v", config.Settings)
	}
	server := newServerState(config.Servers["default"], map[string]struct{}{"default": {}})
	settings := server.settingsForSite("api")
	want := Map{"legacy": true, "root": true, "serverLegacy": true, "server": true, "siteLegacy": true, "site": true, "shared": "site"}
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("merged settings = %v, want %v", settings, want)
	}
	if config.Settings["shared"] != "root" || config.Servers["default"].Settings["shared"] != "server" {
		t.Fatal("site merge mutated inherited settings")
	}
}

func TestOptionsAndSettingsRejectMalformedGroups(t *testing.T) {
	for _, section := range []Map{
		{"option": "bad"}, {"options": []any{}}, {"setting": false}, {"settings": 1},
		{"options": Map{"read_header_timeuot": "1s"}},
		{"servers": Map{"api": Map{"options": true}}},
		{"servers": Map{"api": Map{"settings": true}}},
		{"sites": Map{"api": Map{"settings": true}}},
	} {
		if _, err := parseConfig(section); err == nil {
			t.Errorf("accepted malformed group: %v", section)
		}
	}
	for _, fields := range []Map{{"read_timeout": "-1s"}, {"max_concurrency": -1}} {
		config, err := parseConfig(Map{"options": fields})
		if err != nil {
			t.Fatal(err)
		}
		if err := validateServers(map[string]ServerConfig{"default": defaultServerConfig(config)}); err == nil {
			t.Errorf("accepted negative option: %v", fields)
		}
	}
}
