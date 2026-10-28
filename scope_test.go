package dubi

import (
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestScopedName(t *testing.T) {
	for _, test := range []struct{ scope, name, want string }{
		{"www", "index", "www.index"},
		{"www", "www.login", "www.login"},
		{"www", ".index", ".index"},
		{"www", "*.access", "*.access"},
		{"www", "", "www"},
		{"", "index", "index"},
		{" User.Admin ", " GET ", "user.admin.get"},
		{"user", "username.get", "user.username.get"},
	} {
		if got := scopedName(test.scope, test.name); got != test.want {
			t.Errorf("scopedName(%q, %q) = %q, want %q", test.scope, test.name, got, test.want)
		}
	}
}

func TestScopeRegistrationAndProfile(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	scope := Scope(" User ")
	scope.Register("get", Method{Action: func(*Context) Map { return Map{"id": 1} }})
	scope.Register("hidden", Method{Action: func(*Context) Map { return Map{"hidden": true} }})
	scope.Register("user.full", Method{})
	scope.Register(".shared", Method{})
	scope.Register("*.access", Method{})
	scope.Register(Library{Methods: Methods{"utility": {Action: func(*Context) Map { return Map{"local": true} }}}})
	Scope("").Register("selected", Profile{Include: []string{"core:method:user.get"}})
	for _, name := range []string{"user.get", "user.hidden", "user.full", ".shared", "*.access", "user", "selected"} {
		if _, ok := Lookup(name); !ok {
			t.Errorf("missing registration %q", name)
		}
	}
	if err := rt.Run("selected"); err != nil {
		t.Fatal(err)
	}
	meta := NewMeta()
	if data, result := rt.Invoke(meta, "user.get", nil); result.Fail() || data["id"] != 1 {
		t.Fatalf("scoped method call: %v %v", data, result)
	}
	if _, result := rt.Invoke(meta, "user.hidden", nil); result.OK() {
		t.Fatal("scoped registration bypassed Profile")
	}
	lib := meta.Library("user")
	if data := lib.Invoke("utility"); lib.Result().Fail() || data["local"] != true {
		t.Fatalf("scoped foundation Library was filtered: %v %v", data, lib.Result())
	}
	defer func() {
		if recover() == nil {
			t.Error("Scope.Register bypassed registry freeze")
		}
	}()
	scope.Register("late", Method{})
}

func TestScopeRegistrationUsesGlobalReplacementRules(t *testing.T) {
	testRuntime()
	t.Cleanup(Reset)
	scope := Scope("app")
	// Infrago accepts values before the name and uses the final string argument.
	scope.Register(Method{Name: "first"}, "ignored", Service{Name: "last"}, "run")
	value, ok := Lookup("app.run")
	service, isService := value.(Service)
	if !ok || !isService || service.Name != "last" {
		t.Fatalf("expected last value to replace registration: %#v", value)
	}
	if _, ok := Lookup("app.ignored"); ok {
		t.Fatal("used a string other than the final name")
	}
}
