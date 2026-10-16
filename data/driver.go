package data

import (
	"database/sql"
	"fmt"
	"strings"

	. "github.com/dubi-project/dubi-go/base"
)

// connectBuiltin selects one of the database types compiled into this package.
// It is intentionally private: applications choose a built-in type by name
// and cannot register or replace implementations at runtime.
func connectBuiltin(inst *Instance) (Connection, error) {
	if inst == nil {
		return nil, errInvalidConnection
	}
	kind := strings.TrimSpace(inst.Config.Type)
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "postgres", "postgresql", "pgsql":
		return (&postgresDriver{}).Connect(inst)
	case "mysql":
		return (&mysqlDriver{}).Connect(inst)
	case "sqlite", "sqlite3":
		return (&sqliteDriver{}).Connect(inst)
	case "mongo", "mongodb", "mgdb":
		return (&mongodbDriver{}).Connect(inst)
	case "":
		return nil, fmt.Errorf("missing data type")
	default:
		return nil, fmt.Errorf("unsupported built-in data type %q", kind)
	}
}

type (
	Connection interface {
		Open() error
		Close() error
		Health() Health
		DB() *sql.DB
		Dialect() Dialect
	}

	Health struct {
		Workload int64
	}

	PoolStats struct {
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		Open         int     `json:"open"`
		InUse        int     `json:"inUse"`
		Idle         int     `json:"idle"`
		WaitCount    int64   `json:"waitCount"`
		WaitDuration int64   `json:"waitDurationMs"`
		MaxOpen      int     `json:"maxOpen"`
		Queries      int64   `json:"queries"`
		Writes       int64   `json:"writes"`
		Errors       int64   `json:"errors"`
		CacheHit     int64   `json:"cacheHit"`
		CacheRate    float64 `json:"cacheRate"`
		Slow         int64   `json:"slow"`
		SlowAvgMs    int64   `json:"slowAvgMs"`
		SlowP50Ms    int64   `json:"slowP50Ms"`
		SlowP95Ms    int64   `json:"slowP95Ms"`
	}

	Capabilities struct {
		Dialect       string `json:"dialect"`
		ILike         bool   `json:"ilike"`
		Returning     bool   `json:"returning"`
		Join          bool   `json:"join"`
		Group         bool   `json:"group"`
		Having        bool   `json:"having"`
		Aggregate     bool   `json:"aggregate"`
		KeysetAfter   bool   `json:"keyset_after"`
		JsonContains  bool   `json:"json_contains"`
		ArrayOverlap  bool   `json:"array_overlap"`
		JsonElemMatch bool   `json:"json_elem_match"`
	}

	Dialect interface {
		Name() string
		Quote(string) string
		Placeholder(int) string
		SupportsILike() bool
		SupportsReturning() bool
	}

	ValueBinder interface {
		BindValue(Var, any) (any, bool)
	}

	ValueDecoder interface {
		DecodeValue(Var, any) (any, bool)
	}

	ArrayBinder interface {
		BindArray(any) any
	}

	ParameterLimiter interface {
		MaxParams() int
	}

	ErrorClassifier interface {
		ClassifyError(error) error
	}
)
