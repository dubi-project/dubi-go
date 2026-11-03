package web

import (
	"mime"
	"strings"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
	"github.com/dubi-project/dubi-go/view"
)

type viewBody struct {
	name  string
	model Map
}

// View selects an HTML template. Optional arguments are a model Map, an HTTP
// status int, and a MIME string. Rendering happens after Response filters.
func (ctx *Context) View(name string, args ...any) {
	body := viewBody{name: name}
	contentType := "text/html; charset=utf-8"
	for _, arg := range args {
		switch value := arg.(type) {
		case Map:
			body.model = value
		case int:
			if value > 0 {
				ctx.Code = value
			}
		case string:
			if value != "" {
				contentType = value
				if !strings.Contains(value, "/") {
					contentType = dubi.Mimetype(value, "text/html")
				}
				if mediaType, parameters, err := mime.ParseMediaType(contentType); err == nil {
					if parameters["charset"] == "" {
						parameters["charset"] = "utf-8"
					}
					contentType = mime.FormatMediaType(mediaType, parameters)
				}
			}
		}
	}
	ctx.setResponseHeader("Content-Type", contentType)
	ctx.Body = body
}

func (ctx *Context) renderView(body viewBody) (string, error) {
	return view.Parse(view.Body{
		View: body.name, Site: ctx.Site, Language: ctx.Language(), Timezone: ctx.Timezone(),
		Model: body.model,
		Data: Map{
			"config": ctx.Config, "settings": ctx.Settings,
			"args": ctx.Args, "value": ctx.Value, "locals": ctx.Locals, "data": ctx.Data,
		},
		Helpers: Map{
			"string":  ctx.String,
			"url":     ctx.RouteUrl,
			"uri":     ctx.RouteUri,
			"siteurl": ctx.SiteUrl,
			"ctx":     func() *Context { return ctx },
		},
	})
}
