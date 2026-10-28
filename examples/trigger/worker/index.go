// Package worker demonstrates a module-owned process notification.
package worker

import (
	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

const TEST = "worker.test"

var host dubi.Host
var module = &Module{}

func init() { host = dubi.Mount(module) }

func Emit(stage string) { host.SyncToggle(TEST, Map{"stage": stage}) }
