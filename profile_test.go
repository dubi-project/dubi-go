package dubi

import (
	"context"
	"reflect"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestConfiguredProfilesFilterAndInherit(t *testing.T) {
	rt := testRuntime()
	rt.config = Map{
		"profiles": Map{
			"base": Map{
				"include": []any{"core:method:*"},
			},
			"web": Map{
				"profiles": []any{"base"},
				"exclude":  []any{"core:method:internal"},
			},
		},
	}
	rt.configLoaded = true
	rt.identity.Profile = "web"
	rt.Register("user.get", Method{})
	rt.Register("internal", Method{})
	rt.Register("mail.send", Service{})

	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := entryKeys(rt.Entries()), []string{"core:method:user.get"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("configured profile entries = %v, want %v", got, want)
	}
}

func TestConfiguredProfilesDetectInheritanceCycle(t *testing.T) {
	profiles, err := configuredProfiles(Map{
		"profiles": Map{
			"one": Map{"profiles": []any{"two"}},
			"two": Map{"profiles": []any{"one"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveProfiles(profiles, "one"); err == nil {
		t.Fatal("expected profile inheritance cycle error")
	}
}

func entryKeys(entries []Entry) []string {
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key())
	}
	return keys
}
