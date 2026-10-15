package dubi

import "testing"

func TestRegistryFreezeRejectsLateRegistration(t *testing.T) {
	registry := NewRegistry()
	registry.Register("before", 1)
	registry.Freeze()
	if !registry.Frozen() {
		t.Fatal("registry did not report frozen state")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("late registration did not panic")
		}
	}()
	registry.Register("after", 2)
}
