package base

import "testing"

func TestMapHelpers(t *testing.T) {
	var m Map
	m = m.Set("id", "u_1")
	if value, ok := m.Get("id"); !ok || value != "u_1" {
		t.Fatalf("Get(id) = %v, %v", value, ok)
	}

	clone := m.Clone()
	clone["id"] = "u_2"
	if value, _ := m.Get("id"); value != "u_1" {
		t.Fatalf("Clone mutated original: %v", value)
	}
}
