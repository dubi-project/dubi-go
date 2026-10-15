package main

import (
	"context"
	"log"

	dubi "github.com/dubi-project/dubi-go"
	"github.com/dubi-project/dubi-go/empty"
)

func init() {
	// Importing empty mounts its singleton Module automatically.
	dubi.Register("filter", empty.Filter{Name: "request"})
	dubi.Register("handler", empty.Handler{Name: "request"})
}

func main() {
	ctx := context.Background()

	if err := dubi.Setup(ctx); err != nil {
		log.Fatal(err)
	}
	if err := dubi.Open(ctx); err != nil {
		log.Fatal(err)
	}
	if err := dubi.Start(ctx); err != nil {
		log.Fatal(err)
	}
	if err := dubi.Wait(); err != nil {
		log.Fatal(err)
	}
}
