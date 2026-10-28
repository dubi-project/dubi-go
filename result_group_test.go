package dubi

import (
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestResultGroupNumberingAndStatus(t *testing.T) {
	group := ResultGroup("test.group.user", 2100)
	if group.Name() != "test.group.user" {
		t.Fatalf("group name = %q", group.Name())
	}
	for _, test := range []struct {
		ok     bool
		status string
		code   int
		key    string
	}{
		{true, "created", 0, "test.group.user.created"},
		{false, "invalid.email", 2100, "test.group.user.invalid.email"},
		{true, "updated", 0, "test.group.user.updated"},
		{false, "test.group.user.not.exists", 2101, "test.group.user.not.exists"},
		{false, "test.group.username.invalid", 2102, "test.group.user.test.group.username.invalid"},
	} {
		res := group.Result(test.ok, test.status, "result: %s")
		if res.Code() != test.code || res.Status() != test.key || res.OK() != test.ok {
			t.Fatalf("group result = code:%d status:%q ok:%v", res.Code(), res.Status(), res.OK())
		}
		if !test.ok && res.With("value").Error() != "result: value" {
			t.Fatalf("default result text = %q", res.With("value").Error())
		}
	}
}

func TestResultGroupDefaultBaseAndEmptyPrefix(t *testing.T) {
	group := ResultGroup("")
	res := group.Result(false, "test.group.unprefixed", "failed")
	if group.Name() != "" || res.Code() != 1000 || res.Status() != "test.group.unprefixed" {
		t.Fatalf("unprefixed result = %#v", res)
	}
	other := ResultGroup("test.group.other")
	if res := other.Result(false, "failed", "failed"); res.Code() != 1000 {
		t.Fatalf("separate group should start at its own base: %d", res.Code())
	}
}

func TestResultGroupRegistersStatusAndLanguage(t *testing.T) {
	rt := testRuntime()
	t.Cleanup(Reset)
	group := ResultGroup("test.group.catalog", 3100)
	res := group.Result(false, "invalid", "无效参数: %s")
	Register("en", Language{Strings: Map{"test_group_catalog_invalid": "Invalid argument: %s"}})
	if err := rt.Run(); err != nil {
		t.Fatal(err)
	}
	if code := StatusCode(res.Status()); code != 3100 {
		t.Fatalf("registered code = %d", code)
	}
	meta := NewMeta()
	meta.Language("en")
	if text := meta.String(res.Status(), "email"); text != "Invalid argument: email" {
		t.Fatalf("localized text = %q", text)
	}
	if text := res.With("email").Error(); text != "无效参数: email" {
		t.Fatalf("default text = %q", text)
	}
}
