package web

import (
	"strings"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

var (
	resultLoadingEmpty = dubi.Result(1, "http.loading.empty", "required resource is missing")
	resultLoadingError = dubi.Result(1, "http.loading.error", "resource could not be loaded")
)

// runLoading invokes the configured Method or Service values for a route.
// It returns false after marking the request as failed so the route action is
// skipped and the normal HTTP outcome handler can respond.
func runLoading(ctx *Context, loading Loading) bool {
	if len(loading) == 0 {
		return true
	}
	if ctx.Locals == nil {
		ctx.Locals = Map{}
	}
	for key, loader := range loading {
		valueKey := strings.TrimSpace(loader.Value)
		if valueKey == "" {
			valueKey = "id"
		}
		value, ok := ctx.Args[valueKey]
		if !ok {
			value, ok = ctx.Value[valueKey]
		}
		if value == nil && loader.Required {
			if loader.Empty != nil {
				ctx.Failed(loader.Empty)
			} else {
				ctx.Failed(resultLoadingEmpty)
			}
			return false
		}
		target := strings.TrimSpace(loader.Invoke)
		if target == "" || value == nil {
			continue
		}
		argsKey := strings.TrimSpace(loader.Args)
		if argsKey == "" {
			argsKey = "id"
		}
		item := ctx.Invoke(target, Map{argsKey: value})
		if (item == nil || (ctx.Result() != nil && ctx.Result().Fail())) && loader.Required {
			if loader.Error != nil {
				ctx.Failed(loader.Error)
			} else {
				ctx.Failed(resultLoadingError)
			}
			return false
		}
		ctx.Locals[key] = item
	}
	return true
}
