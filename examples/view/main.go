package main

import (
	"log"
	"strings"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/view"
	"github.com/dubi-project/dubi-go/web"
)

func init() {
	dubi.Register("uppercase", view.Helper{Name: "Uppercase", Alias: []string{"upper"}, Action: strings.ToUpper})
	dubi.Register("index", web.Router{
		Uri: "/hello",
		Action: func(ctx *web.Context) {
			name, _ := ctx.Query["name"].(string)
			if name == "" {
				name = "Ada"
			}
			ctx.Data["year"] = time.Now().Year()
			ctx.View("index", Map{"title": "Dubi View", "name": name})
		},
	})
}

func main() {
	if err := dubi.Run(); err != nil {
		log.Fatal(err)
	}
}
