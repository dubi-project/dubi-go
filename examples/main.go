package main

import (
	"flag"
	"log"

	dubi "github.com/dubi-project/dubi-go"
	"github.com/dubi-project/dubi-go/empty"
)

func init() {
	// Components and Profiles share the global Registry.
	dubi.Register("filter.request", empty.Filter{Name: "request"})
	dubi.Register("filter.auth", empty.Filter{Name: "auth"})
	dubi.Register("handler.mail", empty.Handler{Name: "mail"})
	dubi.Register("handler.audit", empty.Handler{Name: "audit"})
	dubi.Register("user.get", "global user.get")
	dubi.Register("user.admin", "global user.admin")
	dubi.Register("queue.mail", "global queue.mail")
	dubi.Register("queue.internal", "global queue.internal")
	dubi.Register("debug.internal", "excluded from both profiles")
	dubi.Register("web", dubi.Profile{
		Include: []string{"empty:filter:*", "user.*"},
		Exclude: []string{"user.admin"},
	})
	dubi.Register("worker", dubi.Profile{
		Include: []string{"empty:handler:*", "queue.*"},
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

	if err := web.Run("web"); err != nil {
		return err
	}
	printEntries("web all", web.AllEntries())
	printEntries("web matched", web.Entries())
	if err := worker.Run("worker"); err != nil {
		return err
	}
	printEntries("worker all", worker.AllEntries())
	printEntries("worker matched", worker.Entries())
	log.Println("multiple Runtimes are running; press Ctrl-C to stop all")
	return dubi.Wait()
}

func printEntries(label string, entries []dubi.Entry) {
	log.Printf("%s (%d entries)", label, len(entries))
	for _, entry := range entries {
		log.Printf("  name=%q key=%q scope=%q type=%T", entry.Name, entry.Key(), entry.Scope, entry.Value)
	}
}
