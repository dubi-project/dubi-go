package dubi

import (
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestServiceSubjectRegistrationAndInvocation(t *testing.T) {
	rt := testRuntime()
	action := func(ctx *Context) Map {
		service := ctx.Config.(Service)
		return Map{"subject": service.Subject, "name": service.Name}
	}
	mail := &Service{Name: "Welcome", Subject: "user.created", Action: action}
	implicit := &Service{Action: action}
	rt.Register("mail.welcome", mail)
	rt.Register("account.initialize", Service{Subject: "user.created", Action: action})
	rt.Register("default.value", Service{Action: action})
	rt.Register("default.pointer", implicit)
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	if len(core.services) != 4 {
		t.Fatalf("loaded services = %d, want 4", len(core.services))
	}
	for name, subject := range map[string]string{
		"mail.welcome":       "user.created",
		"account.initialize": "user.created",
		"default.value":      "default.value",
		"default.pointer":    "default.pointer",
	} {
		data, result := Invoke(name)
		if result != nil && result.Fail() {
			t.Fatalf("Invoke(%q): %v", name, result)
		}
		if data["subject"] != subject {
			t.Fatalf("Invoke(%q) subject = %v, want %q", name, data["subject"], subject)
		}
	}
	if implicit.Subject != "" || mail.Subject != "user.created" || mail.Name != "Welcome" {
		t.Fatal("loading changed the caller's service definitions")
	}
	if _, _, found := (moduleHost{}).InvokeLocalService(NewMeta(), "user.created", nil); found {
		t.Fatal("local execution treated a subscription subject as a registration name")
	}
	if data, result := Invoke("user.created"); (result != nil && result.Fail()) || data["subject"] != "user.created" {
		t.Fatalf("Invoke BUS fallback = %v, %v", data, result)
	}
}

func TestServiceSubjectReplacementAndProfile(t *testing.T) {
	rt := testRuntime()
	rt.Register("mail.welcome", Service{Subject: "old.subject"})
	rt.Register("mail.welcome", &Service{Subject: "user.created", Action: func(*Context) Map {
		return Map{"replacement": true}
	}})
	rt.Register("account.initialize", Service{Subject: "user.created"})
	rt.Register("mail", Profile{Include: []string{"core:service:mail.*"}})
	if err := rt.Run("mail"); err != nil {
		t.Fatal(err)
	}
	defer rt.End()

	entries := rt.Entries()
	if len(entries) != 1 || entries[0].Key() != "core:service:mail.welcome" {
		t.Fatalf("profile entries = %#v", entries)
	}
	if len(core.services) != 1 || core.services["mail.welcome"].Subject != "user.created" {
		t.Fatalf("loaded services = %#v", core.services)
	}
	data, result := Invoke("mail.welcome")
	if (result != nil && result.Fail()) || data["replacement"] != true {
		t.Fatalf("replacement invocation = %#v, %v", data, result)
	}
}
