package data

import (
	"os"
	"path/filepath"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestScopedTableAndServiceRunTogether(t *testing.T) {
	dubi.Reset()
	t.Cleanup(func() { _ = dubi.End(); dubi.Reset() })
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"data":{"type":"sqlite","url":"file:scoped-registry?mode=memory&cache=shared"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", path)

	dubi.Register("core.config", Table{Fields: Vars{"name": {Type: "string"}}})
	dubi.Register("core.config", dubi.Service{Action: func(*dubi.Context) Map {
		db := Base()
		defer db.Close()
		return db.Table("core.config").First()
	}})
	dubi.Register("only.table", &Table{Fields: Vars{"name": {Type: "string"}}})
	dubi.Register("shared.view", View{})
	dubi.Register("shared.model", &Model{})
	dubi.Register("only.table", dubi.Service{Action: func(*dubi.Context) Map { return Map{"unexpected": true} }})
	dubi.Register("selected", dubi.Profile{Include: []string{"core:service:core.config"}, Exclude: []string{"data:*"}})

	if _, ok := dubi.LookupScope("data:table", "core.config"); !ok {
		t.Fatal("table missing from registry")
	}
	if _, ok := dubi.LookupScope("core:service", "core.config"); !ok {
		t.Fatal("service missing from registry")
	}
	if err := dubi.DefaultRuntime().Run("selected"); err != nil {
		t.Fatal(err)
	}
	if GetTable("core.config") == nil || GetTable("only.table") == nil {
		t.Fatal("Profile filtered foundation tables")
	}
	if GetView("shared.view") == nil || GetModel("shared.model") == nil {
		t.Fatal("Profile filtered foundation view or model")
	}
	db := Base()
	defer db.Close()
	db.Migrate()
	if err := db.Error(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if row := db.Table("core.config").Insert(Map{"name": "example"}); row == nil || row["name"] != "example" {
		t.Fatalf("insert: %#v, %v", row, db.Error())
	}
	if rows := db.Table("core.config").Query(Map{"name": "example"}); len(rows) != 1 {
		t.Fatalf("query: %#v, %v", rows, db.Error())
	}
	if result, status := dubi.Invoke("core.config"); status.Fail() || result["name"] != "example" {
		t.Fatalf("invoke: %#v, %v", result, status)
	}
	if _, status := dubi.Invoke("only.table"); status.OK() {
		t.Fatal("Profile loaded a service when only the same-named table was selected")
	}
}

func TestDataComponentScopes(t *testing.T) {
	r := dubi.NewRegistry()
	r.Register("shared", Table{})
	r.Register("shared", View{})
	r.Register("shared", Model{})
	for _, scope := range []string{"data:table", "data:view", "data:model"} {
		value, ok := r.LookupScope(scope, "shared")
		if !ok || value == nil {
			t.Errorf("missing %s registration", scope)
		}
	}
	if len(r.Entries()) != 3 {
		t.Fatalf("entries: %#v", r.Entries())
	}
}

func TestFieldsBeforeRuntimeSetup(t *testing.T) {
	dubi.Reset()
	t.Cleanup(dubi.Reset)
	dubi.Register("core.config", &Table{Fields: Vars{
		"name": {Type: "string", Options: Map{"a": "original"}},
		"id":   {Type: "int"},
	}})
	dubi.Register("core.config", dubi.Service{})
	dubi.Register("view", View{Fields: Vars{"viewField": {Type: "string"}}})
	dubi.Register("model", &Model{Fields: Vars{"modelField": {Type: "string"}}})

	fields := Fields("core.config", []string{"name"})
	if len(fields) != 1 || fields["name"].Type != "string" {
		t.Fatalf("table fields before setup: %#v", fields)
	}
	fields["name"].Options["a"] = "changed"
	if got := Fields("core.config", nil)["name"].Options["a"]; got != "original" {
		t.Fatalf("field definition was modified: %v", got)
	}
	if Field("view", "viewField").Type != "string" || Field("model", "modelField").Type != "string" {
		t.Fatal("view or model fields missing before setup")
	}
	if Option("core.config", "name", "a") != "original" {
		t.Fatal("field options missing before setup")
	}
	service := dubi.Service{Args: Fields("core.config", []string{"name"})}
	dubi.Register("use.config", service)
	registered, _ := dubi.LookupScope("core:service", "use.config")
	if registered.(dubi.Service).Args["name"].Type != "string" {
		t.Fatal("service did not capture registered table fields")
	}
}
