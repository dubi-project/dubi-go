package dubi

import "testing"

type testFoundation struct{ enabled bool }

func (testFoundation) Scope() string      { return "example:resource" }
func (v testFoundation) Foundation() bool { return v.enabled }

func TestFoundationInterfaceControlsProfile(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	rt.Register("yes", testFoundation{enabled: true})
	rt.Register("no", testFoundation{})
	rt.Register("mime", Mime{"application/example"})
	rt.Register("plain", []string{"application/example"})
	rt.Register("selected", Profile{Include: []string{"unmatched:*"}, Exclude: []string{"example:resource:*", "basic:mime:*"}})
	if err := rt.Run("selected"); err != nil {
		t.Fatal(err)
	}
	entries := rt.Entries()
	if len(entries) != 2 || entries[0].Name != "mime" || entries[1].Name != "yes" {
		t.Fatalf("foundation profile selection: %#v", entries)
	}
	if got := Mimetype("mime"); got != "application/example" {
		t.Fatalf("foundation MIME was not registered: %q", got)
	}
}

func TestBuiltInFoundationResources(t *testing.T) {
	for _, value := range []any{Type{}, Language{}, Mime{}, Status(1), Statuses{}, Codec{}, Library{}} {
		resource, ok := value.(Foundation)
		if !ok || !resource.Foundation() {
			t.Errorf("%T is not a foundation resource", value)
		}
	}
}

func TestMimesBulkRegistration(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	Register("jpg", Service{Name: "same name"})
	Register(Mimes{"json": "application/test+json", "jpg": "image/jpeg"})
	Register("json", Mime{"application/final+json"})
	for name, want := range map[string]string{"json": "application/final+json", "jpg": "image/jpeg"} {
		value, ok := LookupScope("basic:mime", name)
		mime, valid := value.(Mime)
		if !ok || !valid || len(mime) != 1 || mime[0] != want {
			t.Fatalf("registered MIME %s: %#v", name, value)
		}
	}
	if _, ok := LookupScope("core:service", "jpg"); !ok {
		t.Fatal("MIME registration overwrote a different component scope")
	}
	Register("selected", Profile{Include: []string{"core:service:*"}, Exclude: []string{"basic:mime:*"}})
	if err := rt.Run("selected"); err != nil {
		t.Fatal(err)
	}
	if got := Mimetype(".json"); got != "application/final+json" {
		t.Fatalf("JSON MIME: %q", got)
	}
	if got := Mimetype("jpg"); got != "image/jpeg" {
		t.Fatalf("JPG MIME: %q", got)
	}
}
