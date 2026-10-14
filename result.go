package dubi

import (
	"fmt"

	base "github.com/dubi-project/dubi-go/base"
)

type result struct {
	code   int
	status string
	args   []base.Any
}

var (
	// OK is the standard successful result.
	OK base.Res = &result{code: 0, status: "ok"}
	// Fail is the standard unsuccessful result.
	Fail base.Res = &result{code: 1, status: "fail"}
)

func init() {
	registerStatus("ok", Status(0), "成功")
	registerStatus("fail", Status(1), "失败")
}

func (r *result) OK() bool { return r == nil || r.code == 0 }

func (r *result) Fail() bool { return !r.OK() }

func (r *result) Code() int {
	if r == nil {
		return 0
	}
	return r.code
}

func (r *result) Status() string {
	if r == nil {
		return ""
	}
	return r.status
}

func (r *result) Args() []base.Any {
	if r == nil {
		return nil
	}
	return append([]base.Any(nil), r.args...)
}

func (r *result) With(args ...base.Any) base.Res {
	if r == nil {
		return &result{args: append([]base.Any(nil), args...)}
	}
	copyOf := *r
	copyOf.args = append([]base.Any(nil), args...)
	return &copyOf
}

// Error resolves the result text using Dubi's default language catalog.
// Request-specific languages should be resolved through Meta.String or the
// corresponding entry context.
func (r *result) Error() string {
	if r == nil || r.code == 0 {
		return ""
	}
	return defaultBasicInstance().text("default", r.status, r.args...)
}

// String makes results convenient to log and format. It intentionally uses
// the default language, just like Error.
func (r *result) String() string { return r.Error() }

// Result defines one reusable internal status and registers its default text.
func Result(code int, status, text string) base.Res {
	registerStatus(status, Status(code), text)
	return &result{code: code, status: status}
}

// ResultOf creates a result from an internal code and status key.
func ResultOf(code int, status string, args ...base.Any) base.Res {
	return &result{code: code, status: status, args: append([]base.Any(nil), args...)}
}

// ErrorResult converts a Go error into a generic internal failure result.
func ErrorResult(err error) base.Res {
	if err == nil {
		return OK
	}
	return ResultOf(1, err.Error())
}

var _ fmt.Stringer = (*result)(nil)
