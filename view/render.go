package view

import (
	"bytes"
	"fmt"
	"html/template"
	"path"

	. "github.com/dubi-project/dubi-go/base"
)

const maxRenderDepth = 64

type renderer struct {
	engine      *engine
	body        Body
	functions   template.FuncMap
	signature   string
	frames      []string
	layout      string
	layoutModel any
	inLayout    bool
	content     string
	title       string
	author      string
	keywords    string
	description string
	metas       []string
	styles      []string
	scripts     []string
}

func (e *engine) render(body Body) (string, error) {
	p := &renderer{engine: e, body: body}
	p.functions = template.FuncMap{}
	for name, action := range e.helpers {
		p.functions[name] = action
	}
	for name, action := range body.Helpers {
		p.functions[name] = action
	}
	for name, action := range p.builtins() {
		p.functions[name] = action
	}
	if err := validateFunctions(p.functions); err != nil {
		return "", err
	}
	p.signature = functionSignature(p.functions)
	page, err := e.find(body.View, body.Site, body.Language, "")
	if err != nil {
		return "", err
	}
	p.content, err = p.execute(page, body.Model)
	if err != nil {
		return "", err
	}
	if p.layout == "" {
		return p.content, nil
	}
	layout, err := e.find(p.layout, body.Site, body.Language, path.Dir(page.name))
	if err != nil {
		return "", err
	}
	p.inLayout = true
	return p.execute(layout, p.layoutModel)
}

func (p *renderer) execute(file source, model any) (string, error) {
	if len(p.frames) >= maxRenderDepth {
		return "", fmt.Errorf("view: maximum render depth exceeded at %s", file.name)
	}
	tpl, err := p.engine.load(file, p.functions, p.signature)
	if err != nil {
		return "", err
	}
	p.frames = append(p.frames, file.name)
	defer func() { p.frames = p.frames[:len(p.frames)-1] }()
	data := Map{}
	for key, value := range p.body.Data {
		data[key] = value
	}
	if _, ok := data["settings"]; !ok {
		data["settings"] = p.engine.config.Settings.Clone()
	}
	data["model"] = model
	var output bytes.Buffer
	if err := tpl.Execute(&output, data); err != nil {
		return "", fmt.Errorf("view: execute %s: %w", file.name, err)
	}
	return output.String(), nil
}

func (p *renderer) renderHelper(name string, models ...any) (template.HTML, error) {
	if len(models) > 1 {
		return "", fmt.Errorf("view: render accepts one model")
	}
	var model any = Map{}
	if len(models) > 0 {
		model = models[0]
	}
	relative := ""
	if len(p.frames) > 0 {
		relative = path.Dir(p.frames[len(p.frames)-1])
	}
	file, err := p.engine.find(name, p.body.Site, p.body.Language, relative)
	if err != nil {
		return "", err
	}
	output, err := p.execute(file, model)
	return template.HTML(output), err
}
