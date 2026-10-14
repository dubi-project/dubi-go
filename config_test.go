package dubi

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestConfigFileIdentityAndRunProfileOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"dubi":{"tenant":"file-tenant","role":"file-role","profile":"file-profile","node":"file-node","endpoints":["file:7000"]},"web":{"port":8080}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", path)
	rt := New(WithTenant("override-tenant"), WithRole("override-role"), WithNode("override-node"), WithProfile("option-profile"), WithEndpoint("override:7000"))
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	if got, want := rt.Identity(), (Identity{Tenant: "override-tenant", Role: "override-role", Profile: "", Node: "override-node"}); got != want {
		t.Fatalf("runtime identity = %#v, want %#v", got, want)
	}
	if got := rt.Endpoints(); len(got) != 1 || got[0] != "override:7000" {
		t.Fatalf("endpoints = %v", got)
	}
	config := rt.Config()
	identity := config["dubi"].(Map)
	if identity["profile"] != "" || identity["tenant"] != "override-tenant" {
		t.Fatalf("delivered identity = %#v", identity)
	}
	if config["web"].(Map)["port"] != float64(8080) {
		t.Fatalf("module config was lost: %#v", config["web"])
	}
}

func TestDecodeConfigFormats(t *testing.T) {
	cases := []struct {
		name string
		ext  string
		data string
		want any
	}{
		{name: "json", ext: ".json", data: `{"web":{"port":8080}}`, want: float64(8080)},
		{name: "toml", ext: ".toml", data: "[web]\nport = 8080\n", want: int64(8080)},
		{name: "yaml", ext: ".yaml", data: "web:\n  port: 8080\n", want: 8080},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			config, err := decodeConfig([]byte(item.data), item.ext)
			if err != nil {
				t.Fatal(err)
			}
			web, ok := config["web"].(Map)
			if !ok {
				t.Fatalf("web section = %#v", config["web"])
			}
			if web["port"] != item.want {
				t.Fatalf("port = %#v (%T), want %#v (%T)", web["port"], web["port"], item.want, item.want)
			}
		})
	}
}

func TestConfigValueOverridesNestedPath(t *testing.T) {
	config := Map{}
	setConfigPath(config, "http.port", int64(9000))
	setConfigPath(config, "http.cookie.name", "session")
	http := config["http"].(Map)
	if http["port"] != int64(9000) || http["cookie"].(Map)["name"] != "session" {
		t.Fatalf("config = %#v", config)
	}
}

func TestConfigFileProvidesRuntimeProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dubi.json")
	data := `{"dubi":{"profile":"web"},"profiles":{"web":{"include":["core:method:*"]}}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", path)
	rt := NewRuntime()
	rt.Register("user.get", Method{})
	rt.Register("mail.send", Service{})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	if got, want := entryKeys(rt.Entries()), []string{"core:method:user.get"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("config profile entries = %v, want %v", got, want)
	}
}
