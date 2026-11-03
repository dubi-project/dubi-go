package view

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func testEngine(t *testing.T, files fs.FS, config Map, helpers map[string]Helper) *engine {
	t.Helper()
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(files)
	m := &Module{}
	if err := m.Config(Map{"view": config}); err != nil {
		t.Fatal(err)
	}
	for name, helper := range helpers {
		m.Register(name, helper)
	}
	if err := m.Setup(nil); err != nil {
		t.Fatal(err)
	}
	return m.engine
}

func TestLayoutPartialsAndEscaping(t *testing.T) {
	files := fstest.MapFS{
		"views/shop/zh/order/detail.html":     {Data: []byte(`{% layout "layout" .model.layout %}{% title .model.title %}{% meta "description" .model.title %}{% style .model.css %}{% script .model.js %}<main>{% render "parts/card" .model.card %}</main>`)},
		"views/shop/zh/order/layout.html":     {Data: []byte(`<title>{% title %}</title>{% metas %}{% styles %}{% scripts %}{% body %}|{% .model.note %}|{% .data.user %}`)},
		"views/shop/zh/order/parts/card.html": {Data: []byte(`<b>{% upper .model.name %}</b>{% render "leaf" .model %}`)},
		"views/shop/zh/order/parts/leaf.html": {Data: []byte(`<i>{% .model.name %}</i>`)},
	}
	e := testEngine(t, files, Map{"root": "views"}, map[string]Helper{
		"shout": {Alias: []string{"upper"}, Action: strings.ToUpper},
	})
	data := Map{"data": Map{"user": "Ada"}}
	model := Map{
		"title": `<img src=x onerror="bad">`, "css": `javascript:alert(1)`, "js": `/app.js?q="bad"`,
		"card": Map{"name": "a&b"}, "layout": Map{"note": "layout model"},
	}
	output, err := e.render(Body{View: "order/detail", Site: "shop", Language: "zh", Data: data, Model: model})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<title>&lt;img src=x onerror=&#34;bad&#34;&gt;</title>`,
		`<main><b>A&amp;B</b><i>a&amp;b</i></main>|layout model|Ada`,
		`href="#ZgotmplZ"`, `<meta name="description" content="&lt;img`,
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("missing %q in %s", expected, output)
		}
	}
	if strings.Contains(output, "javascript:") || strings.Contains(output, "<img") {
		t.Fatalf("unescaped output: %s", output)
	}
	if _, exists := data["model"]; exists {
		t.Fatal("render changed shared input data")
	}
	if model["title"] != `<img src=x onerror="bad">` {
		t.Fatal("render changed the model")
	}
}

func TestTemplateLookupOrder(t *testing.T) {
	order := []string{
		"views/shop/zh/page.html", "views/shop/zh/page/index.html", "views/shop/zh/shared/page.html",
		"views/shop/page.html", "views/shop/shared/page.html",
		"views/zh/page.html", "views/zh/shared/page.html", "views/page.html", "views/shared/page.html",
	}
	files := fstest.MapFS{}
	for _, name := range order {
		files[name] = &fstest.MapFile{Data: []byte(name)}
	}
	e := testEngine(t, files, Map{"root": "views", "cache": false}, nil)
	for _, name := range order {
		output, err := e.render(Body{View: "page", Site: "shop", Language: "zh"})
		if err != nil || output != name {
			t.Fatalf("want %s, got %q, %v", name, output, err)
		}
		delete(files, name)
	}
}

func TestTemplateAssetsAndLocalFallback(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("views", 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"page.html": "disk page", "local.html": "disk partial"} {
		if err := os.WriteFile(filepath.Join("views", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := testEngine(t, fstest.MapFS{"views/page.html": {Data: []byte(`asset {% render "local" %}`)}}, Map{"root": "views"}, nil)
	output, err := e.render(Body{View: "page"})
	if err != nil || output != "asset disk partial" {
		t.Fatalf("output = %q, %v", output, err)
	}
}

func TestTemplateCacheAndDevelopmentReload(t *testing.T) {
	for _, cache := range []bool{true, false} {
		t.Run(fmt.Sprint(cache), func(t *testing.T) {
			root := t.TempDir()
			filename := filepath.Join(root, "page.html")
			write := func(content string) {
				t.Helper()
				if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(`first {% current %}`)
			e := testEngine(t, nil, Map{"root": root, "cache": cache}, nil)
			body := Body{View: "page", Helpers: Map{"current": func() string { return "one" }}}
			output, err := e.render(body)
			if err != nil || output != "first one" {
				t.Fatalf("first = %q, %v", output, err)
			}
			write(`second {% current %}`)
			body.Helpers["current"] = func() string { return "two" }
			want := "second two"
			if cache {
				want = "first two"
			}
			output, err = e.render(body)
			if err != nil || output != want {
				t.Fatalf("second = %q, %v, want %q", output, err, want)
			}
		})
	}
}

func TestCachedTemplatesKeepRequestStateIsolated(t *testing.T) {
	e := testEngine(t, fstest.MapFS{
		"views/page.html":   {Data: []byte(`{% layout "layout" %}{% title .model.id %}{% current %}:{% render "part" .model %}`)},
		"views/part.html":   {Data: []byte(`{% current %}:{% .model.id %}`)},
		"views/layout.html": {Data: []byte(`<h1>{% title %}</h1>{% body %}`)},
	}, Map{"root": "views"}, nil)
	var workers sync.WaitGroup
	for worker := 0; worker < 12; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for request := 0; request < 20; request++ {
				id := fmt.Sprintf("%d-%d", worker, request)
				body := Body{View: "page", Model: Map{"id": id}, Helpers: Map{"current": func() string { return id }}}
				output, err := e.render(body)
				want := fmt.Sprintf("<h1>%s</h1>%s:%s:%s", id, id, id, id)
				if err != nil || output != want {
					t.Errorf("request %s: %q, %v", id, output, err)
					return
				}
				if len(body.Helpers) != 1 {
					t.Error("render mutated request helpers")
					return
				}
			}
		}()
	}
	workers.Wait()
}

func TestRenderErrorsDoNotReturnPartialOutput(t *testing.T) {
	e := testEngine(t, fstest.MapFS{
		"views/missing.html":    {Data: []byte(`before {% render "absent" %} after`)},
		"views/invalid.html":    {Data: []byte(`before {% if %}`)},
		"views/recursive.html":  {Data: []byte(`{% render "recursive" %}`)},
		"views/helper.html":     {Data: []byte(`before {% fail %}`)},
		"views/page.html":       {Data: []byte(`{% layout "bad-layout" %}body`)},
		"views/bad-layout.html": {Data: []byte(`before {% render "absent" %}`)},
	}, Map{"root": "views"}, nil)
	for _, name := range []string{"missing", "invalid", "recursive", "helper", "page", "absent", "../private", "/etc/passwd"} {
		output, err := e.render(Body{View: name, Helpers: Map{"fail": func() (string, error) { return "", fmt.Errorf("helper failed") }}})
		if err == nil || output != "" {
			t.Errorf("%s returned %q, %v", name, output, err)
		}
	}
	for _, body := range []Body{{View: "page", Site: "../private"}, {View: "page", Language: "en/secret"}} {
		if _, err := e.render(body); err == nil {
			t.Errorf("invalid scope accepted: %+v", body)
		}
	}
}

func TestConfigDelimitersSettingsAndLocale(t *testing.T) {
	e := testEngine(t, fstest.MapFS{
		"views/page.html": {Data: []byte(`{{language}}|{{timezone}}|{{format "2006-01-02 15:04" .model.time}}|{{.settings.label}}|{{.settings.extra}}`)},
	}, Map{"root": "views", "left": "{{", "right": "}}", "setting": Map{"label": "old", "extra": true}, "settings": Map{"label": "new"}}, nil)
	output, err := e.render(Body{View: "page", Language: "en", Timezone: time.FixedZone("example", 3600), Model: Map{"time": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}})
	if err != nil || output != "en|example|2026-01-01 01:00|new|true" {
		t.Fatalf("output = %q, %v", output, err)
	}
	for _, config := range []Map{{"cache": "yes"}, {"root": ""}, {"left": ""}, {"shared": "../private"}, {"settings": 123}} {
		if _, err := parseConfig(Map{"view": config}); err == nil {
			t.Errorf("invalid config accepted: %v", config)
		}
	}
}

func TestHelperValidation(t *testing.T) {
	var nilFunction func() string
	for _, helper := range []struct {
		name   string
		action any
	}{
		{"bad.name", func() string { return "" }}, {"nil", nilFunction}, {"number", 1},
		{"empty", func() {}}, {"returns", func() (string, string) { return "", "" }},
		{"title", func() string { return "" }},
	} {
		m := &Module{config: defaultConfig()}
		m.Register(helper.name, Helper{Action: helper.action})
		if err := m.Setup(nil); err == nil {
			t.Errorf("invalid helper %q accepted", helper.name)
		}
	}
}

func TestHelperRegistrationIgnoresProfile(t *testing.T) {
	dubi.Reset()
	t.Cleanup(dubi.Reset)
	previous := dubi.AssetFS()
	t.Cleanup(func() { dubi.AssetFS(previous) })
	dubi.AssetFS(fstest.MapFS{"asset/views/page.html": {Data: []byte(`{% greeting .model.name %}`)}})
	dubi.Register("hello", &Helper{Alias: []string{"greeting"}, Action: func(name string) string { return "Hello " + name }})
	dubi.Register("selected", dubi.Profile{Include: []string{"nonexistent:*"}, Exclude: []string{"view:*"}})
	if err := dubi.DefaultRuntime().Run("selected"); err != nil {
		t.Fatal(err)
	}
	output, err := Parse(Body{View: "page", Model: Map{"name": "Ada"}})
	if err != nil || output != "Hello Ada" {
		t.Fatalf("profile output = %q, %v", output, err)
	}
	if _, exists := dubi.LookupScope("view:helper", "hello"); !exists {
		t.Fatal("helper scope is missing")
	}
	if err := dubi.End(); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(Body{View: "page"}); err == nil {
		t.Fatal("closed module accepted a render")
	}
}
