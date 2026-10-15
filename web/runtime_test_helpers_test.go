package web

import dubi "github.com/dubi-project/dubi-go"

func testRuntime() *dubi.Runtime {
	dubi.Reset()
	return dubi.DefaultRuntime()
}
