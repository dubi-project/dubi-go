package data

import (
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/dubi-project/dubi-go/base"
	"modernc.org/sqlite"
)

type (
	sqliteDriver struct{}

	sqliteConnection struct {
		instance *Instance
		db       *sql.DB
		actives  int64
	}

	sqliteDialect struct{}
)

const sqliteTimeLayout = "2006-01-02T15:04:05.000000000Z"

func (d *sqliteDriver) Connect(inst *Instance) (Connection, error) {
	return &sqliteConnection{instance: inst}, nil
}

func (c *sqliteConnection) Open() error {
	dsn := strings.TrimSpace(c.instance.Config.Url)
	if dsn == "" {
		if v, ok := c.instance.Setting["dsn"].(string); ok {
			dsn = v
		}
	}
	if dsn == "" {
		dsn = "file:db"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	c.db = db
	return nil
}

func (c *sqliteConnection) Close() error {
	if c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	return err
}

func (c *sqliteConnection) Health() Health {
	return Health{Workload: atomic.LoadInt64(&c.actives)}
}

func (c *sqliteConnection) DB() *sql.DB {
	return c.db
}

func (c *sqliteConnection) Dialect() Dialect {
	return sqliteDialect{}
}

func (sqliteDialect) Name() string { return "sqlite" }
func (sqliteDialect) Quote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, `"`, ``)
	return `"` + s + `"`
}
func (sqliteDialect) Placeholder(_ int) string { return "?" }
func (sqliteDialect) SupportsILike() bool      { return false }
func (sqliteDialect) SupportsReturning() bool  { return true }
func (sqliteDialect) MaxParams() int           { return 999 }
func (sqliteDialect) ClassifyError(err error) error {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return nil
	}
	code := sqliteErr.Code()
	switch code {
	case 1555, 2067:
		return ErrDuplicate
	case 787:
		return ErrForeignKey
	case 5, 6:
		return ErrTimeout
	case 9:
		return ErrCanceled
	case 14:
		return ErrDriver
	default:
		if code&0xff == 19 {
			return ErrConflict
		}
		return nil
	}
}
func (sqliteDialect) BindValue(cfg Var, v any) (any, bool) {
	switch {
	case IsJSONVar(cfg):
		return BindJSONValue(v)
	case IsBinaryVar(cfg):
		return BindBinaryValue(v)
	case IsUUIDVar(cfg), IsDecimalVar(cfg):
		return BindTextValue(v)
	case IsTimeVar(cfg):
		value, ok := BindTimeValue(v)
		if !ok || value == nil {
			return value, ok
		}
		if moment, ok := value.(time.Time); ok {
			// SQLite has no native timestamp storage class. Persist and compare
			// every timestamp as the same fixed-width UTC representation so two
			// equal instants do not sort differently because of their offsets.
			return moment.UTC().Format(sqliteTimeLayout), true
		}
		return value, true
	default:
		return nil, false
	}
}
func (sqliteDialect) DecodeValue(cfg Var, value any) (any, bool) {
	switch {
	case IsJSONVar(cfg):
		return DecodeJSONValue(value)
	case IsBinaryVar(cfg):
		return DecodeBinaryValue(value)
	case IsUUIDVar(cfg), IsDecimalVar(cfg):
		return DecodeTextValue(value)
	case IsTimeVar(cfg):
		return DecodeTimeValue(value)
	default:
		return nil, false
	}
}
