package main

import (
	"context"
	"log"

	"github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/empty"
	"github.com/dubi-project/dubi-go/web"
)

func init() {
	// Components and Profiles share the global Registry.
	dubi.Register("filter.request", empty.Filter{Name: "request"})
	dubi.Register("filter.auth", empty.Filter{Name: "auth"})
	dubi.Register("handler.mail", empty.Handler{Name: "mail"})
	dubi.Register("handler.audit", empty.Handler{Name: "audit"})
	dubi.Register("user.admin", "global user.admin")
	dubi.Register("queue.mail", "global queue.mail")
	dubi.Register("queue.internal", "global queue.internal")
	dubi.Register("debug.internal", "excluded from both profiles")
	dubi.Register("user.get", dubi.Method{Action: func(ctx *dubi.Context) (Map, Res) {
		return Map{
			"id":      ctx.Args["id"],
			"name":    "Ada Lovelace",
			"request": ctx.Id(),
		}, nil
	}})
	dubi.Register("mail.send", dubi.Service{Action: func(ctx *dubi.Context) Map {
		return Map{"sent": ctx.Value["subject"]}
	}})

	// HTTP is a standard entry point. Router actions write HTTP responses
	// directly; Method/Service calls remain optional business operations.
	dubi.Register("*.access", web.Filter{
		Serve: func(ctx *web.Context) {
			log.Printf("http request site=%s host=%s method=%s path=%s", ctx.Site, ctx.Host, ctx.Method, ctx.Path)
			ctx.Next()
		},
	})
	dubi.Register("*.errors", web.Handler{
		NotFound: func(ctx *web.Context) {
			ctx.JSON(Map{"error": "not found", "path": ctx.Path}, 404)
		},
		Error: func(ctx *web.Context) {
			ctx.JSON(Map{"error": "error", "path": ctx.Path, "err": ctx.Result()}, 500)
		},
	})
	dubi.Register("www.index", web.Router{
		Uri: "/", Name: "asdff", Desc: "asdfadf",
		Action: func(ctx *web.Context) {
			user := ctx.Invoke("user.get", Map{"id": "demo"})
			ctx.Answer(dubi.OK, Map{
				"site":    ctx.Site,
				"host":    ctx.Host,
				"message": "hello from www",
				"user":    user,
			})
		},
	})
	dubi.Register("api.user", web.Router{
		Uri: "/users/{id}",
		Args: Vars{
			"id": {Type: "string", Required: true},
		},
		Loading: web.Loading{
			"user": {Invoke: "user.get", Value: "id", Required: true},
		},
		Data: Vars{
			"id":   {Type: "string", Required: true},
			"name": {Type: "string", Required: true},
		},
		Routing: web.Routing{
			"GET": {
				Action: func(ctx *web.Context) {
					ctx.Answer(dubi.OK, ctx.Locals["user"].(Map))
				},
			},
			"POST": {
				Setting: Map{"operation": "create"},
				Action: func(ctx *web.Context) {
					ctx.JSON(Map{"site": ctx.Site, "operation": ctx.Setting["operation"]}, 201)
				},
			},
		},
	})
	dubi.Register("web", dubi.Profile{
		Include: []string{"empty:filter:*", "core:method:*", "web:router:*", "web:filter:*", "web:handler:*"},
		Exclude: []string{"user.admin"},
	})
	dubi.Register("worker", dubi.Profile{
		Include: []string{"empty:handler:*", "core:service:*", "queue.*"},
		Exclude: []string{"queue.internal"},
	})
}

func main() {
	log.Println("starting the process runtime with profile web; press Ctrl-C to stop")
	dubi.Configure(dubi.WithProfile("web"))
	if err := dubi.Setup(context.Background()); err != nil {
		log.Fatal(err)
	}
	printEntries("all", dubi.AllEntries())
	printEntries("matched", dubi.Entries())
	if err := dubi.Open(context.Background()); err != nil {
		log.Fatal(err)
	}
	if err := dubi.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	if err := dubi.Wait(); err != nil {
		log.Fatal(err)
	}
}

func printEntries(label string, entries []dubi.Entry) {
	log.Printf("%s (%d entries)", label, len(entries))
	for _, entry := range entries {
		log.Printf("  name=%q key=%q scope=%q type=%T", entry.Name, entry.Key(), entry.Scope, entry.Value)
	}
}
