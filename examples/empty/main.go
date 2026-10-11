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
	runtime := dubi.DefaultRuntime()
	ctx := context.Background()

	if err := runtime.Setup(ctx); err != nil {
		log.Fatal(err)
	}
	if err := runtime.Open(ctx); err != nil {
		log.Fatal(err)
	}
	if err := runtime.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := runtime.Stop(ctx); err != nil {
			log.Print(err)
		}
		if err := runtime.Close(ctx); err != nil {
			log.Print(err)
		}
	}()
}
