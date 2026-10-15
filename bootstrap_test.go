package dubi

import (
	"reflect"
	"testing"
)

func TestRuntimeIdentityAndConnectionOptions(t *testing.T) {
	rt := testRuntime(
		WithTenant("tenant-a"), WithRole("web"), WithProfile("web"), WithNode("node-a"),
		WithEndpoints("first:7000", "second:7000"),
	)
	if got, want := rt.Identity(), (Identity{Tenant: "tenant-a", Role: "web", Profile: "web", Node: "node-a"}); got != want {
		t.Fatalf("identity = %#v, want %#v", got, want)
	}
	if got := rt.Endpoints(); !reflect.DeepEqual(got, []string{"first:7000", "second:7000"}) {
		t.Fatalf("endpoints = %v", got)
	}
	endpoints := rt.Endpoints()
	endpoints[0] = "changed:7000"
	if rt.Endpoints()[0] != "first:7000" {
		t.Fatal("Endpoints exposed mutable endpoint list")
	}
}
