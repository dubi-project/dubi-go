package main

import (
	"flag"
	"log"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/empty"
	dubiweb "github.com/dubi-project/dubi-go/web"
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
		return Map{"id": ctx.Args["id"], "request": ctx.Id()}, nil
	}})
	dubi.Register("mail.send", dubi.Service{Action: func(ctx *dubi.Context) Map {
		return Map{"sent": ctx.Value["subject"]}
	}})

	// HTTP is a standard entry point. Router actions write HTTP responses
	// directly; Method/Service calls remain optional business operations.
	dubi.Register("*.access", dubiweb.Filter{
		Serve: func(ctx *dubiweb.Context) {
			log.Printf("http request site=%s host=%s method=%s path=%s", ctx.Site, ctx.Host, ctx.Method, ctx.Path)
			ctx.Next()
		},
	})
	dubi.Register("*.errors", dubiweb.Handler{
		NotFound: func(ctx *dubiweb.Context) {
			ctx.JSON(Map{"error": "not found", "path": ctx.Path}, 404)
		},
		Error: func(ctx *dubiweb.Context) {
			ctx.JSON(Map{"error": "error", "path": ctx.Path, "err": ctx.Result()}, 500)
		},
	})
	dubi.Register("www.index", dubiweb.Router{
		Uri: "/", Name: "asdff", Desc: "asdfadf",
		Action: func(ctx *dubiweb.Context) {
			ctx.JSON(Map{"site": ctx.Site, "host": ctx.Host, "message": "hello from www"})
			ctx.Invoke("user.login.get", Map{"id": ctx.Args["id"]})
		},
	})
	dubi.Register("api.user", dubiweb.Router{
		Uri: "/users/{id}",
		Routing: dubiweb.Routing{
			"GET": {
				Action: func(ctx *dubiweb.Context) {
					ctx.JSON(Map{"site": ctx.Site, "id": ctx.Params["id"]})
				},
			},
			"POST": {
				Setting: Map{"operation": "create"},
				Action: func(ctx *dubiweb.Context) {
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
	mode := flag.String("mode", "multi", "run mode: single or multi")
	flag.Parse()

	if *mode == "single" {
		log.Println("starting one Runtime with profile web; press Ctrl-C to stop")
		if err := dubi.Run("web"); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runMultiple(); err != nil {
		log.Fatal(err)
	}
}

func runMultiple() error {
	web := dubi.New()
	web.Register("filter.local", empty.Filter{Name: "web-local"})
	web.Register("user.local", "web local user")

	worker := dubi.New()
	worker.Register("handler.local", empty.Handler{Name: "worker-local"})
	worker.Register("queue.local", "worker local queue")
	worker.Register("local.codec", dubi.Codec{
		Encode: func(value Any) (Any, error) {
			return []byte("worker:" + value.(string)), nil
		},
	})
	web.Register("local.codec", dubi.Codec{
		Encode: func(value Any) (Any, error) {
			return []byte("web:" + value.(string)), nil
		},
	})

	if err := web.Run("web"); err != nil {
		return err
	}
	printEntries("web all", web.AllEntries())
	printEntries("web matched", web.Entries())
	meta := dubi.NewMeta(web)
	log.Printf("user.get result: %#v", meta.Invoke("user.get", Map{"id": "u-1"}))
	webCodec, err := meta.Marshal("local.codec", "value")
	if err != nil {
		return err
	}
	log.Printf("web local codec: %s", webCodec)
	if err := worker.Run("worker"); err != nil {
		return err
	}
	printEntries("worker all", worker.AllEntries())
	printEntries("worker matched", worker.Entries())
	meta = dubi.NewMeta(worker)
	log.Printf("mail.send result: %#v", meta.Invoke("mail.send", Map{"subject": "welcome"}))
	workerCodec, err := meta.Marshal("local.codec", "value")
	if err != nil {
		return err
	}
	log.Printf("worker local codec: %s", workerCodec)
	log.Println("multiple Runtimes are running; press Ctrl-C to stop all")
	return dubi.Wait()
}

func printEntries(label string, entries []dubi.Entry) {
	log.Printf("%s (%d entries)", label, len(entries))
	for _, entry := range entries {
		log.Printf("  name=%q key=%q scope=%q type=%T", entry.Name, entry.Key(), entry.Scope, entry.Value)
	}
}
