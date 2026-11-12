package data

import (
	"os"
	"path/filepath"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestSQLiteArrayContainsMembers(t *testing.T) {
	dubi.Reset()
	t.Cleanup(func() { _ = dubi.End(); dubi.Reset() })
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"data":{"default":{"type":"sqlite","url":"file:array-contains?mode=memory&cache=shared"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)
	dubi.Register("array_members", Table{Fields: Vars{
		"name":       {Type: "string", Required: true},
		"upline_ids": {Type: "[int]", Required: true, Default: []Any{}},
	}})
	if err := dubi.DefaultRuntime().Run(); err != nil {
		t.Fatal(err)
	}
	db := Base()
	defer db.Close()
	db.Migrate()
	for _, row := range []Map{
		{"name": "first", "upline_ids": []int64{12, 45}},
		{"name": "second", "upline_ids": []int64{45, 12, 78}},
		{"name": "nearby", "upline_ids": []int64{112, 4512}},
		{"name": "empty", "upline_ids": []Any{}},
	} {
		db.Table("array_members").Insert(row)
		if err := db.Error(); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		label   string
		members Any
		count   int64
	}{
		{"member at any position", []Any{int64(12)}, 2},
		{"scalar member", int64(12), 2},
		{"numeric prefix does not match", []Any{int64(1)}, 0},
		{"multiple members", []Any{int64(45), int64(12)}, 2},
		{"partially missing subset", []Any{int64(12), int64(99)}, 0},
		{"empty subset", []Any{}, 4},
		{"number is not string", []Any{"12"}, 0},
	} {
		t.Run(test.label, func(t *testing.T) {
			count := db.Table("array_members").Count(Map{"upline_ids": Map{OpContains: test.members}})
			if err := db.Error(); err != nil {
				t.Fatal(err)
			}
			if count != test.count {
				t.Fatalf("got %d, want %d", count, test.count)
			}
		})
	}
	count, rows := db.Table("array_members").Slice(int64(1), int64(1), Map{
		OptFilter: Map{"upline_ids": Map{OpContains: []Any{int64(12)}}},
		OptSorts:  []Map{{"id": ASC}},
	})
	if err := db.Error(); err != nil {
		t.Fatal(err)
	}
	if count != 2 || len(rows) != 1 || rows[0]["name"] != "second" {
		t.Fatal("array membership broke database pagination")
	}
}
