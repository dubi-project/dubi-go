package data

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	. "github.com/dubi-project/dubi-go/base"
	mysql "github.com/go-sql-driver/mysql"
)

type (
	mysqlDriver struct{}

	mysqlConnection struct {
		instance *Instance
		db       *sql.DB
		actives  int64
	}

	mysqlDialect struct{}
)

func (d *mysqlDriver) Connect(inst *Instance) (Connection, error) {
	return &mysqlConnection{instance: inst}, nil
}

func (c *mysqlConnection) Open() error {
	dsn := strings.TrimSpace(c.instance.Config.Url)
	if dsn == "" {
		if v, ok := c.instance.Setting["dsn"].(string); ok {
			dsn = v
		}
	}
	if dsn == "" {
		return fmt.Errorf("missing mysql dsn")
	}
	db, err := sql.Open("mysql", dsn)
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

func (c *mysqlConnection) Close() error {
	if c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	return err
}

func (c *mysqlConnection) Health() Health {
	return Health{Workload: atomic.LoadInt64(&c.actives)}
}

func (c *mysqlConnection) DB() *sql.DB {
	return c.db
}

func (c *mysqlConnection) Dialect() Dialect {
	return mysqlDialect{}
}

func (mysqlDialect) Name() string { return "mysql" }
func (mysqlDialect) Quote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "`", "")
	return "`" + s + "`"
}
func (mysqlDialect) Placeholder(_ int) string { return "?" }
func (mysqlDialect) SupportsILike() bool      { return false }
func (mysqlDialect) SupportsReturning() bool  { return false }
func (mysqlDialect) MaxParams() int           { return 65535 }
func (mysqlDialect) ClassifyError(err error) error {
	var myerr *mysql.MySQLError
	if !errors.As(err, &myerr) {
		return nil
	}
	switch myerr.Number {
	case 1062:
		return ErrDuplicate
	case 1451, 1452:
		return ErrForeignKey
	case 1205:
		return ErrTimeout
	case 1213:
		return ErrConflict
	case 1317:
		return ErrCanceled
	case 2006, 2013:
		return ErrDriver
	default:
		return nil
	}
}
func (mysqlDialect) BindValue(cfg Var, v any) (any, bool) {
	switch {
	case IsJSONVar(cfg):
		return BindJSONValue(v)
	case IsBinaryVar(cfg):
		return BindBinaryValue(v)
	case IsUUIDVar(cfg), IsDecimalVar(cfg):
		return BindTextValue(v)
	case IsTimeVar(cfg):
		return BindTimeValue(v)
	default:
		return nil, false
	}
}
func (mysqlDialect) DecodeValue(cfg Var, value any) (any, bool) {
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
