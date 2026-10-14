package web

import "testing"

func TestExpandRoutePathEscapesSingleAndMultiParameters(t *testing.T) {
	if got, want := expandRoutePath("/users/{id}", map[string]any{"id": "a/b"}), "/users/a%2Fb"; got != want {
		t.Fatalf("single path = %q, want %q", got, want)
	}
	if got, want := expandRoutePath("/files/{path...}", map[string]any{"path": "a/b c"}), "/files/a/b%20c"; got != want {
		t.Fatalf("multi path = %q, want %q", got, want)
	}
}

func TestRouteRegistryUsesFirstURIAsCanonical(t *testing.T) {
	registry := newRouteRegistry(ServerConfig{Name: defaultServerName})
	registry.add("user", "user", "user", "/users/{id}", "")
	registry.add("user", "user", "user", "/people/{id}", "")

	info, ok := registry.lookup(&Context{Name: "user"}, "user", nil)
	if !ok || info.uri != "/users/{id}" {
		t.Fatalf("route = %#v, ok = %v", info, ok)
	}
}
