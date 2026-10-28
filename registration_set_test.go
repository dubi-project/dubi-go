package dubi

import (
	"strings"
	"testing"
)

func TestFoundationRegistrationSets(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	Register(Types{"custom": {Name: "custom"}})
	Register(Codecs{"custom": {Name: "custom"}})
	Register(Statuses{"custom": 123})
	Register(Regulars{"digits": {`^[0-9]+$`, `^none$`}})
	Register("selected", Profile{Include: []string{"core:service:*"}, Exclude: []string{"basic:*", "codec:*"}})
	if err := rt.Run("selected"); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"basic:type", "codec", "basic:status"} {
		if _, ok := LookupScope(scope, "custom"); !ok {
			t.Errorf("missing %s:custom", scope)
		}
	}
	if !Match("digits", "123") || !Match("digits", "none") || Match("digits", "abc") || Match("missing", "123") {
		t.Fatal("named regular matching failed")
	}
	expressions := Expressions("digits")
	expressions[0] = "changed"
	if Expressions("digits")[0] != `^[0-9]+$` {
		t.Fatal("Expressions exposed registered storage")
	}
	if got := Expressions("missing", "fallback"); len(got) != 1 || got[0] != "fallback" {
		t.Fatalf("defaults: %#v", got)
	}
}

func TestInvalidRegularFailsSetup(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	Register("bad", Regular{"["})
	if err := rt.Run(); err == nil || !strings.Contains(err.Error(), `regular "bad"`) {
		t.Fatalf("invalid regular error: %v", err)
	}
}
