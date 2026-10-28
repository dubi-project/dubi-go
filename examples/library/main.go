package main

import (
	"fmt"
	"log"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func init() {
	dubi.Register("greeting", dubi.Library{
		Name:     "Greeter",
		Settings: Map{"prefix": "Hello"},
		Methods: dubi.Methods{
			"say": {
				Args: Vars{"name": {Type: "string", Required: true}},
				Action: func(ctx *dubi.Context) Map {
					return Map{"message": fmt.Sprintf("%s, %s", ctx.Settings["prefix"], ctx.Args["name"])}
				},
			},
		},
	})
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
		if closeErr := dubi.End(); err == nil {
			err = closeErr
		}
	}()
	meta := dubi.NewMeta()
	lib := meta.Library("greeting")
	data := lib.Invoke("say", Map{"name": "Ada"})
	if result := lib.Result(); result.Fail() {
		return result
	}
	fmt.Println(data["message"])

	// A second invoker can override settings without changing shared defaults.
	localized := meta.Library("greeting", Map{"prefix": "你好"})
	data = localized.Invoke("say", Map{"name": "Ada"})
	if result := localized.Result(); result.Fail() {
		return result
	}
	fmt.Println(data["message"])
	return nil
}
