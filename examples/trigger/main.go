package main

import (
	"fmt"
	"log"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/examples/trigger/worker"
)

var started = make(chan struct{}, 1)

func init() {
	// Both listeners are retained under the same logical notification name.
	for _, name := range []string{"first", "second"} {
		dubi.Register(worker.TEST, dubi.Trigger{
			Name: name,
			Args: Vars{"stage": {Type: "string", Required: true}},
			Action: func(ctx *dubi.Context) {
				fmt.Printf("%s: %s\n", name, ctx.Args["stage"])
			},
		})
	}
	dubi.Register(dubi.START, dubi.Trigger{Action: func(*dubi.Context) {
		fmt.Println("runtime started")
		started <- struct{}{}
	}})
	dubi.Register(dubi.STOP, dubi.Trigger{Action: func(*dubi.Context) {
		fmt.Println("runtime stopping")
	}})
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (err error) {
	if err = dubi.DefaultRuntime().Run(); err != nil {
		return err
	}
	defer func() {
		if endErr := dubi.End(); err == nil {
			err = endErr
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("START trigger timed out")
	}
	worker.Emit("manual")
	dubi.SyncToggle(worker.TEST, Map{"stage": "package call"})
	return nil
}
