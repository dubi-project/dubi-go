package main

import (
	dubi "github.com/dubi-project/dubi-go"
	"github.com/dubi-project/dubi-go/empty"
)

func init() {
	// Importing empty mounts its singleton Module automatically.
	dubi.Register("filter", empty.Filter{Name: "request"})
	dubi.Register("handler", empty.Handler{Name: "request"})
}

func main() {
	app := dubi.New()
	app.Run()
}
