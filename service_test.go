package dubi

import (
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestServiceListenRegistrationAndInvocation(t *testing.T) {
	rt := testRuntime()
	action := func(ctx *Context) Map {
		service := ctx.Config.(Service)
		return Map{"listen": service.Listen, "name": service.Name}
	}
	mail := &Service{Name: "Welcome", Listen: "user.created", Action: action}
	implicit := &Service{Action: action}
	rt.Register("mail.welcome", mail)
	rt.Register("account.initialize", Service{Listen: "user.created", Action: action})
	rt.Register("default.value", Service{Action: action})
	rt.Register("default.pointer", implicit)
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	if len(core.services) != 4 {
		t.Fatalf("loaded services = %d, want 4", len(core.services))
	}
	for name, listen := range map[string]string{
		"mail.welcome":       "user.created",
		"account.initialize": "user.created",
		"default.value":      "",
		"default.pointer":    "",
	} {
		data, result := Invoke(name)
		if result != nil && result.Fail() {
			t.Fatalf("Invoke(%q): %v", name, result)
		}
		if data["listen"] != listen {
			t.Fatalf("Invoke(%q) listen = %v, want %q", name, data["listen"], listen)
		}
	}
	if implicit.Listen != "" || mail.Listen != "user.created" || mail.Name != "Welcome" {
		t.Fatal("loading changed the caller's service definitions")
	}
	if _, _, found := (moduleHost{}).InvokeLocalService(NewMeta(), "user.created", nil); found {
		t.Fatal("local execution treated a subscription listen as a registration name")
	}
	if data, result := Invoke("user.created"); data != nil || result == nil || result.OK() {
		t.Fatal("Listen was treated as an Invoke alias")
	}
}

func TestServiceListenReplacementAndProfile(t *testing.T) {
	rt := testRuntime()
	rt.Register("mail.welcome", Service{Listen: "old.listen"})
	rt.Register("mail.welcome", &Service{Listen: "user.created", Action: func(*Context) Map {
		return Map{"replacement": true}
	}})
	rt.Register("account.initialize", Service{Listen: "user.created"})
	rt.Register("mail", Profile{Include: []string{"core:service:mail.*"}})
	if err := rt.Run("mail"); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	entries := rt.Entries()
	if len(entries) != 1 || entries[0].Key() != "core:service:mail.welcome" {
		t.Fatalf("profile entries = %#v", entries)
	}
	if len(core.services) != 1 || core.services["mail.welcome"].Listen != "user.created" {
		t.Fatalf("loaded services = %#v", core.services)
	}
	data, result := Invoke("mail.welcome")
	if (result != nil && result.Fail()) || data["replacement"] != true {
		t.Fatalf("replacement invocation = %#v, %v", data, result)
	}
}
