package dubi

import (
	"context"
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestMappingUsesRuntimeTypesAndAliases(t *testing.T) {
	rt := testRuntime()
	rt.Register("positive", Type{
		Valid: func(value Any, _ Var) bool {
			switch n := value.(type) {
			case int:
				return n > 0
			case int64:
				return n > 0
			default:
				return false
			}
		},
		Value: func(value Any, _ Var) Any { return int64(value.(int)) },
	})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())

	result, res := rt.Mapping(Vars{
		"id":    {Type: "int64", Alias: []string{"user_id"}, Required: true},
		"page":  {Type: "int", Default: "2"},
		"score": {Type: "positive"},
	}, Map{"user_id": "42", "score": 3})
	if res != nil && res.Fail() {
		t.Fatal(res)
	}
	if result["id"] != int64(42) || result["page"] != int64(2) || result["score"] != int64(3) {
		t.Fatalf("unexpected mapped values: %#v", result)
	}
}

func TestMappingReportsRequiredArgument(t *testing.T) {
	result, res := Mapping(Vars{"name": {Required: true}}, nil)
	if result == nil || res == nil || res.Code() != 400 {
		t.Fatalf("expected required argument result, data=%#v result=%#v", result, res)
	}
}

func TestMappingUsesEmptyResultForMissingArguments(t *testing.T) {
	_, res := Mapping(Vars{
		"name": {Required: true, Empty: ResultOf(422, "name.required")},
	}, nil)
	if res == nil || res.Code() != 422 || res.Status() != "name.required" {
		t.Fatalf("empty result = %#v", res)
	}
}

func TestMappingNestedValuesAcceptTypedSlicesAndMaps(t *testing.T) {
	result, res := Mapping(Vars{
		"users": {
			Children: Vars{
				"id": {Type: "int", Required: true},
			},
		},
		"scores": {Type: "[int]"},
	}, Map{
		"users":  []Map{{"id": "7"}},
		"scores": []int{1, 2, 3},
	})
	if res != nil && res.Fail() {
		t.Fatal(res)
	}
	users, ok := result["users"].([]any)
	if !ok || len(users) != 1 || users[0].(Map)["id"] != int64(7) {
		t.Fatalf("nested users = %#v", result["users"])
	}
	scores, ok := result["scores"].([]any)
	if !ok || len(scores) != 3 || scores[0] != int64(1) || scores[2] != int64(3) {
		t.Fatalf("array scores = %#v", result["scores"])
	}
}

func TestPreparedMappingReusesCompiledDefinition(t *testing.T) {
	rt := testRuntime()
	definition := Vars{
		"id":   {Type: "int", Required: true},
		"name": {Type: "string"},
	}
	plan := rt.PrepareMapping(definition)
	result, res := rt.ApplyMapping(plan, Map{"id": "7", "name": "Ada"})
	if res != nil && res.Fail() {
		t.Fatal(res)
	}
	if result["id"] != int64(7) || result["name"] != "Ada" {
		t.Fatalf("prepared mapping = %#v", result)
	}
}

func TestMappingReportsNestedArgumentPath(t *testing.T) {
	_, res := Mapping(Vars{
		"users": {
			Children: Vars{
				"id": {Type: "int", Required: true},
			},
		},
	}, Map{"users": []Map{{}}})
	if res == nil || res.Status() != "missing argument: users[0].id" {
		t.Fatalf("nested mapping error = %#v, want users[0].id", res)
	}
}

func TestMappingParsesStructuredTextAndAppliesDefaultsAndEncoding(t *testing.T) {
	result, res := Mapping(Vars{
		"ids": {
			Type:     "[int]",
			Required: true,
		},
		"other_ids": {Type: "[]int"},
		"profile": {
			Type: "json",
			Children: Vars{
				"name": {Type: "string", Required: true},
			},
		},
		"limit": {Type: "int", Default: func() int { return 7 }},
		"token": {Type: "string", Default: "abc", Encode: "text"},
	}, Map{
		"ids":       "[1,2]",
		"other_ids": []string{"3", "4"},
		"profile":   `{"name":"alice"}`,
	})
	if res != nil && res.Fail() {
		t.Fatal(res)
	}
	ids, ok := result["ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != int64(1) || ids[1] != int64(2) {
		t.Fatalf("ids = %#v", result["ids"])
	}
	other, ok := result["other_ids"].([]any)
	if !ok || len(other) != 2 || other[0] != int64(3) || other[1] != int64(4) {
		t.Fatalf("other_ids = %#v", result["other_ids"])
	}
	profile, ok := result["profile"].(Map)
	if !ok || profile["name"] != "alice" {
		t.Fatalf("profile = %#v", result["profile"])
	}
	if result["limit"] != int64(7) {
		t.Fatalf("limit = %#v", result["limit"])
	}
	if token, ok := result["token"].([]byte); !ok || string(token) != "abc" {
		t.Fatalf("token = %#v", result["token"])
	}
	result, res = Mapping(Vars{
		"limit": {Type: "int", Default: 9},
	}, Map{"limit": ""})
	if res != nil || result["limit"] != int64(9) {
		t.Fatalf("empty default = %#v, result=%#v", result, res)
	}
}

func TestMappingTreatsEmptyCollectionsAsMissingRequiredValues(t *testing.T) {
	_, res := Mapping(Vars{
		"items": {Type: "[int]", Required: true},
	}, Map{"items": []int{}})
	if res == nil || res.Status() != "missing argument: items" {
		t.Fatalf("empty collection result = %#v", res)
	}
	_, res = Mapping(Vars{
		"items": {Type: "[int]", Required: true},
	}, Map{"items": "[]"})
	if res == nil || res.Status() != "missing argument: items" {
		t.Fatalf("empty encoded collection result = %#v", res)
	}
}

func TestMappingIsDeterministicAndPreservesOptionalNil(t *testing.T) {
	result, res := Mapping(Vars{"optional": {Type: "string"}}, Map{"optional": nil})
	if res != nil {
		t.Fatal(res)
	}
	value, exists := result["optional"]
	if !exists || value != nil {
		t.Fatalf("optional nil = %#v", result["optional"])
	}
	result, res = Mapping(Vars{"optional": {Type: "string", Nullable: true}}, nil)
	if res != nil {
		t.Fatal(res)
	}
	if _, exists = result["optional"]; exists {
		t.Fatalf("missing nullable field should be omitted: %#v", result)
	}
	_, res = Mapping(Vars{
		"z": {Required: true},
		"a": {Required: true},
	}, nil)
	if res == nil || res.Status() != "missing argument: a" {
		t.Fatalf("mapping error = %#v, want first key a", res)
	}
}

func TestRuntimeLanguageSelection(t *testing.T) {
	rt := testRuntime()
	rt.Register("zh-CN", Language{
		Accepts: []string{"zh", "zh-CN"},
		Strings: Map{"hello_world": "你好，%s"},
	})
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	if got := rt.DetectLanguage("en-US, zh-CN;q=0.9"); got != "zh-CN" {
		t.Fatalf("language = %q", got)
	}
	if got := rt.String("zh-CN", "hello.world", "Dubi"); got != "你好，Dubi" {
		t.Fatalf("translation = %q", got)
	}
	meta := NewMeta()
	meta.Language("zh-CN")
	if got := meta.String("hello.world", "Dubi"); got != "你好，Dubi" {
		t.Fatalf("meta translation = %q", got)
	}
}

func TestResultRegistersStatusAndLanguageText(t *testing.T) {
	invalidEmail := Result(91, "test.invalid.email", "无效电子邮箱: %s")
	if invalidEmail.Code() != 91 || invalidEmail.Status() != "test.invalid.email" {
		t.Fatalf("result = %#v", invalidEmail)
	}
	if got := invalidEmail.With("email").Error(); got != "无效电子邮箱: email" {
		t.Fatalf("result text = %q", got)
	}
	rt := testRuntime()
	if err := rt.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	if got := rt.StatusCode("test.invalid.email"); got != 91 {
		t.Fatalf("status code = %d", got)
	}
	if got := rt.String("default", "test.invalid.email", "email"); got != "无效电子邮箱: email" {
		t.Fatalf("runtime text = %q", got)
	}
}
