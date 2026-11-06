package main

import (
	"log"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func main() {
	finished := make(chan string, 5)
	dubi.Register("user.get", dubi.Service{Action: func(ctx *dubi.Context) Map {
		return Map{"id": ctx.Value["id"], "name": "Ada"}
	}})
	for name, group := range map[string]string{"mail.welcome": "mail", "account.initialize": "account"} {
		dubi.Register(name, dubi.Service{Subject: "user.created", Group: group, Action: func(ctx *dubi.Context) {
			finished <- ctx.Name
		}})
	}
	dubi.Register("mail.send", dubi.Service{Retry: []time.Duration{20 * time.Millisecond}, Action: func(ctx *dubi.Context) Res {
		log.Printf("task=%s attempt=%d", ctx.Id(), ctx.Attempt())
		if ctx.Attempt() == 1 {
			return dubi.Fail
		}
		finished <- ctx.Name
		return dubi.OK
	}})

	// Use the nonblocking process handle so this demonstration exits by itself.
	rt := dubi.DefaultRuntime()
	if err := rt.Run(""); err != nil {
		log.Fatal(err)
	}
	defer rt.End()
	data, result := dubi.Request("user.get", Map{"id": "42"})
	log.Printf("Request: data=%v result=%v", data, result)
	for _, call := range []func(string, ...Map) error{dubi.Broadcast, dubi.Dispatch} {
		if err := call("user.created", Map{"id": "42"}); err != nil {
			log.Fatal(err)
		}
	}
	if err := dubi.Enqueue("mail.send"); err != nil {
		log.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		select {
		case name := <-finished:
			log.Printf("completed %s", name)
		case <-time.After(5 * time.Second):
			log.Fatal("BUS example timed out")
		}
	}
}
