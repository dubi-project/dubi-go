package dubi

import "testing"

func TestRegistryScopedNamesAndReplacement(t *testing.T) {
	r := NewRegistry()
	r.Register("core.config", Method{Name: "old"})
	r.Register("core.config", Service{Name: "service"})
	r.Register("core.config", Method{Name: "new"})
	r.Register("core.config", Trigger{Name: "first"})
	r.Register("core.config", Trigger{Name: "second"})

	if got, ok := r.LookupScope("core:method", "core.config"); !ok || got.(Method).Name != "new" {
		t.Fatalf("method replacement: %#v, %v", got, ok)
	}
	if got, ok := r.LookupScope("core:service", "core.config"); !ok || got.(Service).Name != "service" {
		t.Fatalf("service: %#v, %v", got, ok)
	}
	if got, ok := r.Lookup("core.config"); !ok || got.(Method).Name != "new" {
		t.Fatalf("legacy lookup: %#v, %v", got, ok)
	}
	if got, ok := r.LookupScope("core:trigger", "core.config"); !ok || got.(Trigger).Name != "second" {
		t.Fatalf("trigger lookup: %#v, %v", got, ok)
	}
	if _, ok := r.LookupScope("data:table", "core.config"); ok {
		t.Fatal("lookup returned a different component's registration")
	}
	entries := r.Entries()
	want := []string{"core:method:core.config", "core:service:core.config", "core:trigger:core.config", "core:trigger:core.config"}
	if len(entries) != len(want) {
		t.Fatalf("entries: %#v", entries)
	}
	for i, entry := range entries {
		if entry.Key() != want[i] || entry.Name != "core.config" {
			t.Fatalf("entry %d: %#v", i, entry)
		}
	}
	if entries[2].Value.(Trigger).Name != "first" || entries[3].Value.(Trigger).Name != "second" {
		t.Fatalf("trigger registration order: %#v", entries)
	}
}
