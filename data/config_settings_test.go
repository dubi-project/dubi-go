package data

import (
	"reflect"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestSettingsAliasesPreserveFilePrecedence(t *testing.T) {
	for _, key := range []string{"setting", "settings"} {
		t.Run(key, func(t *testing.T) {
			module.Reset()
			t.Cleanup(module.Reset)
			if err := module.Config(Map{"data": Map{key: Map{"safeWrite": false, "cache": Map{"enable": false}}}}); err != nil {
				t.Fatal(err)
			}
			module.RegisterConfig(defaultName, Config{Type: "sqlite", Settings: Map{"safeWrite": true, "cache": Map{"enable": true, "ttl": 30}, "code": true}})
			cfg := module.configs[defaultName]
			want := Map{"safeWrite": false, "cache": Map{"enable": false, "ttl": 30}, "code": true}
			if cfg.Type != "sqlite" || !reflect.DeepEqual(cfg.Settings, want) {
				t.Fatalf("file/default merge: %+v", cfg)
			}
			if len(module.configs) != 1 {
				t.Fatalf("settings treated as a connection: %+v", module.configs)
			}
		})
	}
}

func TestPluralDataSettingsOverrideLegacy(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)
	if err := module.Config(Map{"data": Map{
		"type":     "sqlite",
		"setting":  Map{"safeWrite": true, "legacy": true, "cache": Map{"enable": true, "ttl": 30}},
		"settings": Map{"safeWrite": false, "cache": Map{"enable": false}},
	}}); err != nil {
		t.Fatal(err)
	}
	want := Map{"safeWrite": false, "legacy": true, "cache": Map{"enable": false, "ttl": 30}}
	if cfg := module.configs[defaultName]; !reflect.DeepEqual(cfg.Settings, want) {
		t.Fatalf("settings = %v, want %v", cfg.Settings, want)
	}
	if err := module.Config(Map{"data": Map{"settings": false}}); err == nil {
		t.Fatal("non-object settings accepted")
	}
}
