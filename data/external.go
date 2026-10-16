package data

import (
	"fmt"

	. "github.com/dubi-project/dubi-go/base"
)

type RawExecutor interface {
	Command(Any) Map
	FindRaw(string, Any, ...Map) []Map
	AggregateRaw(string, Any) []Map
}

// Base returns a fresh request-scoped handle. Use one handle per goroutine or
// transaction; concurrent requests should call Base independently and share
// the underlying connection pool.
func Base(names ...string) DataBase { return module.Base(names...) }

func Sequence(key string, offset, step int64, names ...string) (int64, error) {
	db := Base(names...)
	defer db.Close()
	return db.Sequence(key, offset, step)
}

func SequenceMany(key string, count, offset, step int64, names ...string) ([]int64, error) {
	db := Base(names...)
	defer db.Close()
	return db.SequenceMany(key, count, offset, step)
}

func Migrate(names ...string) error {
	db := Base()
	defer db.Close()
	db.Migrate(names...)
	return db.Error()
}

func MigrateOn(base string, names ...string) error {
	db := Base(base)
	defer db.Close()
	db.Migrate(names...)
	return db.Error()
}

func MigratePlan(names ...string) (MigrateReport, error) {
	db := Base()
	defer db.Close()
	report := db.MigratePlan(names...)
	return report, db.Error()
}

func MigratePlanOn(base string, names ...string) (MigrateReport, error) {
	db := Base(base)
	defer db.Close()
	report := db.MigratePlan(names...)
	return report, db.Error()
}

func MigrateDiff(names ...string) (MigrateReport, error) {
	db := Base()
	defer db.Close()
	report := db.MigrateDiff(names...)
	return report, db.Error()
}

func MigrateDiffOn(base string, names ...string) (MigrateReport, error) {
	db := Base(base)
	defer db.Close()
	report := db.MigrateDiff(names...)
	return report, db.Error()
}

func MigrateUp(versions ...string) error {
	db := Base()
	defer db.Close()
	db.MigrateUp(versions...)
	return db.Error()
}

func MigrateUpOn(base string, versions ...string) error {
	db := Base(base)
	defer db.Close()
	db.MigrateUp(versions...)
	return db.Error()
}

func MigrateDown(steps int) error {
	db := Base()
	defer db.Close()
	db.MigrateDown(steps)
	return db.Error()
}

func MigrateDownOn(base string, steps int) error {
	db := Base(base)
	defer db.Close()
	db.MigrateDown(steps)
	return db.Error()
}

func GetCapabilities(names ...string) (Capabilities, error) { return module.GetCapabilities(names...) }
func GetStats(names ...string) Stats                        { return module.QueryStats(names...) }
func GetPoolStats(names ...string) []PoolStats              { return module.PoolStats(names...) }
func Parse(args ...Any) (Query, error)                      { return ParseQuery(args...) }
func GetTable(name string) *Table                           { return module.TableConfig(name) }
func GetView(name string) *View                             { return module.ViewConfig(name) }
func GetModel(name string) *Model                           { return module.ModelConfig(name) }
func Field(name, field string, extends ...Any) Var          { return module.Field(name, field, extends...) }
func Fields(name string, keys []string, extends ...Vars) Vars {
	return module.Fields(name, keys, extends...)
}
func Option(name, field, key string) Any { return module.Option(name, field, key) }
func Options(name, field string) Map     { return module.Options(name, field) }
func Tables() map[string]Table           { return module.Tables() }
func Views() map[string]View             { return module.Views() }
func Models() map[string]Model           { return module.Models() }
func Migrations(names ...string) []Migration {
	base := ""
	if len(names) > 0 {
		base = names[0]
	}
	return module.migrationConfigs(base)
}

func AsRawExecutor(db DataBase) (RawExecutor, bool) {
	re, ok := db.(RawExecutor)
	return re, ok
}

func Command(db DataBase, cmd Any) Map {
	re, ok := AsRawExecutor(db)
	if !ok {
		return nil
	}
	return re.Command(cmd)
}

func FindRaw(db DataBase, collection string, filter Any, opts ...Map) []Map {
	re, ok := AsRawExecutor(db)
	if !ok {
		return nil
	}
	return re.FindRaw(collection, filter, opts...)
}

func AggregateRaw(db DataBase, collection string, pipeline Any) []Map {
	re, ok := AsRawExecutor(db)
	if !ok {
		return nil
	}
	return re.AggregateRaw(collection, pipeline)
}

func EnsureMongoDriver(db DataBase) error {
	if _, ok := AsRawExecutor(db); !ok {
		return fmt.Errorf("data db is not mongodb driver")
	}
	return nil
}
