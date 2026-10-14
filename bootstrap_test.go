package dubi

import (
	"reflect"
	"testing"
)

func TestRuntimeIdentityAndConnectionOptionsAreIndependent(t *testing.T) {
	first := New(
		WithTenant("tenant-a"), WithRole("web"), WithProfile("web"), WithNode("node-a"),
		WithEndpoints("first:7000", "second:7000"),
	)
	second := New(
		WithTenant("tenant-b"), WithRole("worker"), WithProfile("worker"), WithNode("node-b"),
		WithEndpoint("other:7000"),
	)
	if got, want := first.Identity(), (Identity{Tenant: "tenant-a", Role: "web", Profile: "web", Node: "node-a"}); got != want {
		t.Fatalf("first identity = %#v, want %#v", got, want)
	}
	if got, want := second.Identity(), (Identity{Tenant: "tenant-b", Role: "worker", Profile: "worker", Node: "node-b"}); got != want {
		t.Fatalf("second identity = %#v, want %#v", got, want)
	}
	if got := first.Endpoints(); !reflect.DeepEqual(got, []string{"first:7000", "second:7000"}) {
		t.Fatalf("first endpoints = %v", got)
	}
	if got := second.Endpoints(); !reflect.DeepEqual(got, []string{"other:7000"}) {
		t.Fatalf("second endpoints = %v", got)
	}
	endpoints := first.Endpoints()
	endpoints[0] = "changed:7000"
	if first.Endpoints()[0] != "first:7000" {
		t.Fatal("Endpoints exposed the runtime's mutable endpoint list")
	}
}
