package data

import dubi "github.com/dubi-project/dubi-go"

// host is retained for module-owned process notifications and registrations.
var host dubi.Host

func init() { host = dubi.Mount(module) }
