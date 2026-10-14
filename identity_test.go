package dubi

import "testing"

func TestIdentityCommandLineOverridesEnvironment(t *testing.T) {
	env := map[string]string{
		"DUBI_TENANT": "env-tenant", "DUBI_ROLE": "env-role",
		"DUBI_PROFILE": "env-profile", "DUBI_NODE": "env-node",
	}
	getenv := func(name string) string { return env[name] }
	identity := parseIdentity(nil, getenv)
	if identity != (Identity{Tenant: "env-tenant", Role: "env-role", Profile: "env-profile", Node: "env-node"}) {
		t.Fatalf("environment identity = %#v", identity)
	}
	identity = parseIdentity([]string{"--tenant=cli-tenant", "--role", "cli-role", "--profile", "cli-profile", "--node=cli-node"}, getenv)
	if identity != (Identity{Tenant: "cli-tenant", Role: "cli-role", Profile: "cli-profile", Node: "cli-node"}) {
		t.Fatalf("command line identity = %#v", identity)
	}
	identity = parseIdentity([]string{"--profile=", "--", "--tenant=ignored"}, getenv)
	if identity.Profile != "" || identity.Tenant != "env-tenant" {
		t.Fatalf("explicit empty profile and argument terminator = %#v", identity)
	}
}

func TestRuntimeEmptyRunProfileOverridesWithProfile(t *testing.T) {
	rt := New(WithProfile("missing"))
	rt.Register("included", Method{})
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	if rt.Identity().Profile != "" || len(rt.Entries()) == 0 {
		t.Fatalf("empty Run profile did not load all entries: %#v", rt.Identity())
	}
}

func TestRuntimeNodeDefaultsAreShared(t *testing.T) {
	first, second := New(), New()
	if first.Identity().Node == "" || first.Identity().Node != second.Identity().Node {
		t.Fatal("runtimes must inherit the same nonempty process node ID")
	}
}

func TestMetaRequestIDsAreUnique(t *testing.T) {
	first := NewMeta().Id()
	second := NewMeta().Id()
	if first == "" || second == "" || first == second {
		t.Fatalf("request IDs are not unique: %q and %q", first, second)
	}
}

func TestRuntimeIdentityOptionsAndRunProfile(t *testing.T) {
	rt := New(
		WithTenant("tenant-a"),
		WithRole("worker"),
		WithProfile("default"),
		WithNode("node-a"),
	)
	identity := rt.Identity()
	if identity.Tenant != "tenant-a" || identity.Role != "worker" || identity.Profile != "default" || identity.Node != "node-a" {
		t.Fatalf("identity = %#v", identity)
	}
	rt.Register("default", Profile{})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	if got := rt.Identity().Profile; got != "default" {
		t.Fatalf("default profile = %q", got)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}

	rt = New(WithProfile("default"))
	rt.Register("web", Profile{})
	if err := rt.Run("web"); err != nil {
		t.Fatal(err)
	}
	if got := rt.Identity().Profile; got != "web" {
		t.Fatalf("Run profile = %q", got)
	}
	if err := rt.End(); err != nil {
		t.Fatal(err)
	}
}
