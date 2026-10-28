package worker

import (
	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

type Module struct{}

func (*Module) Register(string, any)      {}
func (*Module) Config(Map) error          { return nil }
func (*Module) Setup(*dubi.Context) error { return nil }
func (*Module) Open(*dubi.Context) error  { return nil }
func (*Module) Start(*dubi.Context) error { Emit("module.start"); return nil }
func (*Module) Stop(*dubi.Context) error  { Emit("module.stop"); return nil }
func (*Module) Close(*dubi.Context) error { return nil }
