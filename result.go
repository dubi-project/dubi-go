package dubi

import (
	"errors"
	"fmt"

	base "github.com/dubi-project/dubi-go/base"
)

type result struct {
	code   int
	status string
	args   []base.Any
	retry  bool
}

var (
	// OK is the standard successful result.
	OK base.Res = &result{code: 0, status: "ok"}
	// Fail is the standard unsuccessful result.
	Fail base.Res = &result{code: 1, status: "fail"}
	// Retry is an explicit retryable failure. To preserve a business error's
	// code and localized text, return that result's Retry() copy instead.
	Retry base.Res = &result{code: 2, status: "retry", retry: true}
)

func init() {
	registerStatus("ok", Status(0), "成功")
	registerStatus("fail", Status(1), "失败")
	registerStatus("retry", Status(2), "请稍后重试")
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

func (r *result) Retry(flags ...bool) base.Res {
	var copyOf result
	if r != nil {
		copyOf = *r
	}
	copyOf.args = append([]base.Any(nil), copyOf.args...)
	copyOf.retry = true
	if len(flags) > 0 {
		copyOf.retry = flags[0]
	}
	return &copyOf
}

func (r *result) Retriable() bool { return r != nil && r.retry }

// IsRetry reports an explicitly marked failure. Success is never retried,
// even if marked. A status string alone does not imply a retry instruction.
func IsRetry(res base.Res) bool { return res != nil && res.Fail() && res.Retriable() }

// RetryResult preserves a failure and adds its retry flag. Optional arguments
// replace its message arguments. Nil and successful results stay successful.
func RetryResult(res base.Res, args ...base.Any) base.Res {
	if res == nil || res.OK() {
		return res
	}
	if len(args) > 0 {
		res = res.With(args...)
	}
	return res.Retry()
}

// Error resolves the result text using Dubi's default language catalog.
// Request-specific languages should be resolved through Meta.String or the
// corresponding entry context.
func (r *result) Error() string {
	if r == nil || r.code == 0 {
		return ""
	}
	return processBasic().text("default", r.status, r.args...)
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

// ErrorResult preserves Res errors (including wrapped ones and retry flags),
// otherwise converts a Go error into a generic, non-retryable failure.
func ErrorResult(err error) base.Res {
	if err == nil {
		return OK
	}
	var res base.Res
	if errors.As(err, &res) {
		return res
	}
	return ResultOf(1, err.Error())
}

var _ fmt.Stringer = (*result)(nil)
