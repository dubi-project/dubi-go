package data

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestSQLiteLifecycleAndCRUD(t *testing.T) {
	dubi.Reset()
	t.Cleanup(func() {
		_ = dubi.End()
		dubi.Reset()
	})

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"data":{"default":{"type":"sqlite","url":"file:test?mode=memory&cache=shared"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DUBI_CONFIG", configPath)

	dubi.Register("users", Table{Fields: Vars{
		"name": {Type: "string", Required: true},
	}})
	// A file-level connection is authoritative over a code default with the
	// same name. If this were ignored, startup would try to connect to MySQL.
	dubi.Register("default", Config{Type: "mysql", Url: "invalid-code-default"})
	if err := dubi.DefaultRuntime().Run(); err != nil {
		t.Fatal(err)
	}

	db := Base()
	t.Cleanup(func() { _ = db.Close() })
	db.Migrate()
	if err := db.Error(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	row := db.Table("users").Insert(Map{"name": "alice"})
	if row == nil || row["name"] != "alice" {
		t.Fatalf("insert row = %#v", row)
	}
	rows := db.Table("users").Query(Map{"name": "alice"})
	if len(rows) != 1 || rows[0]["name"] != "alice" {
		t.Fatalf("query rows = %#v", rows)
	}

	if affected := db.Exec("this is not valid sql"); affected != 0 {
		t.Fatalf("invalid SQL affected %d rows", affected)
	}
	first := db.Error()
	second := db.Error()
	if first == nil || second == nil || first.Error() != second.Error() {
		t.Fatalf("Error should be a stable read: first=%v second=%v", first, second)
	}
	_ = db.Table("users").Query(Map{"name": "alice"})
	if err := db.Error(); err != nil {
		t.Fatalf("new successful call did not clear the previous error: %v", err)
	}
	res := db.Table("users").Scan(func(Map) Res { return dubi.Fail })
	if res == nil || res.OK() {
		t.Fatalf("scan result = %#v, want failure", res)
	}
	if err := db.Error(); err == nil {
		t.Fatal("scan callback failure was not reported by Error")
	}

	missing := db.Table("missing")
	if missing == nil {
		t.Fatal("missing table returned nil")
	}
	if err := db.Error(); err == nil {
		t.Fatal("missing table did not report an error")
	}
	if result := missing.Query(); result != nil {
		t.Fatalf("missing table query = %#v, want nil", result)
	}
}

func TestConfigForms(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)

	if err := module.Config(Map{"data": Map{
		"type": "sqlite",
		"url":  "file:single-config?mode=memory&cache=shared",
		"pool": Map{"maxOpen": 3},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := len(module.configs); got != 1 {
		t.Fatalf("single connection config count = %d, want 1", got)
	}
	if cfg := module.configs[defaultName]; cfg.Type != "sqlite" || cfg.MaxOpen != 3 {
		t.Fatalf("single connection config = %#v", cfg)
	}

	module.Reset()
	if err := module.Config(Map{"data": Map{
		"default":   Map{"type": "sqlite", "url": "file:default?mode=memory&cache=shared"},
		"analytics": Map{"type": "sqlite", "url": "file:analytics?mode=memory&cache=shared"},
	}}); err != nil {
		t.Fatal(err)
	}
	if got := len(module.configs); got != 2 {
		t.Fatalf("multiple connection config count = %d, want 2", got)
	}

	module.Reset()
	if err := module.Config(Map{"data": Map{
		"default": Map{"type": "sqlite", "url": "file:file-defaults?mode=memory&cache=shared"},
	}}); err != nil {
		t.Fatal(err)
	}
	module.RegisterConfig("default", Config{
		Type:    "mysql",
		Url:     "code-default-must-not-win",
		MaxOpen: 7,
		Setting: Map{"scanBatch": int64(64)},
	})
	cfg := module.configs[defaultName]
	if cfg.Type != "sqlite" || cfg.Url != "file:file-defaults?mode=memory&cache=shared" {
		t.Fatalf("file connection values were not authoritative: %#v", cfg)
	}
	if cfg.MaxOpen != 7 || cfg.Setting["scanBatch"] != int64(64) {
		t.Fatalf("code defaults were not merged: %#v", cfg)
	}
}

func TestConfigRequiresType(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)
	module.RegisterConfig("default", Config{Url: "file:missing-type"})
	if err := module.Setup(nil); err == nil {
		t.Fatal("Setup accepted a data connection without a type")
	}
}

func TestConnectionCloseClearsLocalState(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)
	module.RegisterConfig("default", Config{Type: "sqlite", Url: "file:clear-state?mode=memory&cache=shared"})
	if err := module.Setup(nil); err != nil {
		t.Fatal(err)
	}
	if err := module.Open(nil); err != nil {
		t.Fatal(err)
	}

	cacheMap("default").Store("stale", cacheValue{expireAt: 1})
	mongoCacheMap("default").Store("stale", mongoCacheValue{expireAt: 1})
	statsFor("default").Queries.Add(1)

	module.Close(nil)

	if _, ok := cacheMap("default").Load("stale"); ok {
		t.Fatal("SQL cache survived connection close")
	}
	if _, ok := mongoCacheMap("default").Load("stale"); ok {
		t.Fatal("Mongo cache survived connection close")
	}
	if got := module.QueryStats("default").Queries; got != 0 {
		t.Fatalf("stats after close = %d, want 0", got)
	}
}

func TestClosedBaseReturnsErrClosed(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)
	module.RegisterConfig("default", Config{Type: "sqlite", Url: "file:closed-base?mode=memory&cache=shared"})
	if err := module.Setup(nil); err != nil {
		t.Fatal(err)
	}
	if err := module.Open(nil); err != nil {
		t.Fatal(err)
	}

	db := module.Base()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := db.Exec("SELECT 1"); got != 0 {
		t.Fatalf("closed Exec affected %d rows, want 0", got)
	}
	if err := db.Error(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Exec error = %v, want ErrClosed", err)
	}
	if caps := db.Capabilities(); caps.Dialect != "" {
		t.Fatalf("closed capabilities = %#v, want empty", caps)
	}
	if err := db.Error(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Capabilities error = %v, want ErrClosed", err)
	}
	if query, params := db.Parse(Map{"id": 1}); query != "" || params != nil {
		t.Fatalf("closed Parse = %q, %#v; want empty", query, params)
	}
	if err := db.Error(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Parse error = %v, want ErrClosed", err)
	}
	if _, err := db.Sequence("closed", 1, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed Sequence error = %v, want ErrClosed", err)
	}
}

func TestConcurrentBaseHandles(t *testing.T) {
	module.Reset()
	t.Cleanup(module.Reset)
	module.RegisterConfig("default", Config{Type: "sqlite", Url: "file:concurrent-handles?mode=memory&cache=shared"})
	if err := module.Setup(nil); err != nil {
		t.Fatal(err)
	}
	if err := module.Open(nil); err != nil {
		t.Fatal(err)
	}

	const workers = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := module.Base()
			defer db.Close()
			if rows := db.Raw("SELECT 1 AS value"); len(rows) != 1 || rows[0]["value"] == nil {
				errCh <- fmt.Errorf("unexpected rows: %#v", rows)
				return
			}
			if err := db.Error(); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestSQLPlanCacheCapacityIsolatedByConnection(t *testing.T) {
	first := "plan-cache-first"
	second := "plan-cache-second"
	firstKey := "instance=" + first + "|name=one"
	firstOtherKey := "instance=" + first + "|name=two"
	secondKey := "instance=" + second + "|name=one"
	resetSQLPlans(first)
	resetSQLPlans(second)
	t.Cleanup(func() {
		resetSQLPlans(first)
		resetSQLPlans(second)
	})

	sqlPlanCache.Store(firstKey, &cachedSQLPlan{})
	sqlPlanCache.Store(firstOtherKey, &cachedSQLPlan{})
	sqlPlanCache.Store(secondKey, &cachedSQLPlan{})
	sqlPlanCountPtr(first).Add(2)
	sqlPlanCountPtr(second).Add(1)

	evictSQLPlans(first, 1)
	if got := sqlPlanCountPtr(first).Load(); got != 1 {
		t.Fatalf("first connection plan count = %d, want 1", got)
	}
	if got := sqlPlanCountPtr(second).Load(); got != 1 {
		t.Fatalf("second connection plan count = %d, want 1", got)
	}
	if _, ok := sqlPlanCache.Load(secondKey); !ok {
		t.Fatal("evicting one connection removed another connection's plan")
	}
}

func TestBuiltinTypeSelection(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{name: "postgres", want: "*data.postgresConnection"},
		{name: "mysql", want: "*data.mysqlConnection"},
		{name: "sqlite3", want: "*data.sqliteConnection"},
		{name: "mongo", want: "*data.mongodbConnection"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, err := connectBuiltin(&Instance{Config: Config{Type: test.name}})
			if err != nil {
				t.Fatal(err)
			}
			if got := typeName(conn); got != test.want {
				t.Fatalf("connection type = %s, want %s", got, test.want)
			}
		})
	}
}

func typeName(value any) string {
	return fmt.Sprintf("%T", value)
}
