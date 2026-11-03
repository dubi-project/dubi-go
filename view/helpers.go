package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

var builtinNames = map[string]struct{}{
	"layout": {}, "body": {}, "render": {}, "title": {}, "author": {},
	"keywords": {}, "description": {}, "meta": {}, "metas": {},
	"style": {}, "styles": {}, "script": {}, "scripts": {},
	"language": {}, "timezone": {}, "format": {},
}

// Fixed templates preserve HTML attribute escaping and URL scheme filtering.
var resourceTags = template.Must(template.New("tags").Parse(
	`{{define "meta"}}<meta {{if .HTTP}}http-equiv{{else}}name{{end}}="{{.Name}}" content="{{.Content}}">{{end}}` +
		`{{define "style"}}<link rel="stylesheet" href="{{.Path}}"{{if .Media}} media="{{.Media}}"{{end}}>{{end}}` +
		`{{define "script"}}<script type="{{.Type}}" src="{{.Path}}"></script>{{end}}`))

func (p *renderer) builtins() template.FuncMap {
	zone := p.body.Timezone
	if zone == nil {
		zone = time.Local
	}
	return template.FuncMap{
		"layout": p.layoutHelper,
		"body":   func() template.HTML { return template.HTML(p.content) },
		"render": p.renderHelper,
		"title":  textHelper(&p.title), "author": textHelper(&p.author),
		"keywords": textHelper(&p.keywords), "description": textHelper(&p.description),
		"meta": p.metaHelper, "metas": func() template.HTML { return template.HTML(strings.Join(p.metas, "\n")) },
		"style": p.styleHelper, "styles": func() template.HTML { return template.HTML(strings.Join(p.styles, "\n")) },
		"script": p.scriptHelper, "scripts": func() template.HTML { return template.HTML(strings.Join(p.scripts, "\n")) },
		"language": func() string { return p.body.Language },
		"timezone": func() string { return zone.String() },
		"format": func(format string, args ...any) string {
			if len(args) == 1 {
				switch value := args[0].(type) {
				case time.Time:
					return value.In(zone).Format(format)
				case int64:
					if value >= 31507200 && value <= 31507200000 {
						return time.Unix(value, 0).In(zone).Format(format)
					}
				}
			}
			return fmt.Sprintf(format, args...)
		},
	}
}

func textHelper(value *string) func(...string) string {
	return func(args ...string) string {
		if len(args) > 0 {
			*value = args[0]
			return ""
		}
		return *value
	}
}

func (p *renderer) layoutHelper(name string, models ...any) (string, error) {
	if p.inLayout {
		return "", fmt.Errorf("view: a layout cannot select another layout")
	}
	if !validName(name) {
		return "", fmt.Errorf("view: invalid layout name %q", name)
	}
	if len(models) > 1 {
		return "", fmt.Errorf("view: layout accepts one model")
	}
	model := any(Map{})
	if len(models) > 0 {
		model = models[0]
		if encoded, ok := model.(string); ok {
			var decoded Map
			if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
				return "", fmt.Errorf("view: invalid layout model: %w", err)
			}
			model = decoded
		}
	}
	p.layout, p.layoutModel = name, model
	return "", nil
}

func renderTag(name string, data Map) (string, error) {
	var buffer bytes.Buffer
	if err := resourceTags.ExecuteTemplate(&buffer, name, data); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func (p *renderer) metaHelper(name, content string, httpEquiv ...bool) (string, error) {
	value := len(httpEquiv) > 0 && httpEquiv[0]
	tag, err := renderTag("meta", Map{"Name": name, "Content": content, "HTTP": value})
	if err == nil {
		p.metas = append(p.metas, tag)
	}
	return "", err
}

func (p *renderer) styleHelper(path string, media ...string) (string, error) {
	value := ""
	if len(media) > 0 {
		value = media[0]
	}
	tag, err := renderTag("style", Map{"Path": path, "Media": value})
	if err == nil {
		p.styles = append(p.styles, tag)
	}
	return "", err
}

func (p *renderer) scriptHelper(path string, types ...string) (string, error) {
	value := "text/javascript"
	if len(types) > 0 && types[0] != "" {
		value = types[0]
	}
	tag, err := renderTag("script", Map{"Path": path, "Type": value})
	if err == nil {
		p.scripts = append(p.scripts, tag)
	}
	return "", err
}
