package web

import (
	"testing"

	. "github.com/dubi-project/dubi-go/base"
)

func TestExpandRouterUsesParentDefaultsAndMethodOverrides(t *testing.T) {
	baseAction := func(*Context) {}
	getAction := func(*Context) {}
	routes := expandRouters("user", Router{
		Uri:     "/users/{id}",
		Args:    Vars{"id": {Default: "parent"}, "page": {Default: 1}},
		Data:    Vars{"kind": {Default: "user"}},
		Setting: Map{"auth": true, "format": "json"},
		Action:  baseAction,
		Routing: Routing{
			"GET": {
				Args:    Vars{"id": {Default: "get"}},
				Setting: Map{"format": "full"},
				Action:  getAction,
			},
			"POST": {
				Setting: Map{"write": true},
			},
		},
	})
	get, ok := routes["user.get"]
	if !ok {
		t.Fatalf("routes = %#v", routes)
	}
	if get.Method != "GET" || get.Uri != "/users/{id}" || get.Action == nil {
		t.Fatalf("get route = %#v", get)
	}
	if get.Args["id"].Default != "get" || get.Args["page"].Default != 1 || get.Setting["auth"] != true || get.Setting["format"] != "full" {
		t.Fatalf("get inheritance = %#v %#v", get.Args, get.Setting)
	}
	post := routes["user.post"]
	if post.Action != nil || post.Setting["auth"] != true || post.Setting["write"] != true {
		t.Fatalf("post inheritance = %#v", post)
	}
	if _, ok := routes["user.*"]; !ok {
		t.Fatal("expected parent action fallback route")
	}
	if routes["user.get"].Data["kind"].Default != "user" {
		t.Fatalf("data inheritance = %#v", routes["user.get"].Data)
	}
}

func TestExpandRouterWithoutRoutingKeepsAllMethods(t *testing.T) {
	routes := expandRouters("index", Router{Uri: "/"})
	route, ok := routes["index"]
	if !ok || route.Method != "" {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestFilterAndHandlerComponentsExposeScopes(t *testing.T) {
	filters := []struct {
		name  string
		value any
	}{
		{"filter", Filter{}},
		{"serve", ServeFilter{}},
		{"request", RequestFilter{}},
		{"execute", ExecuteFilter{}},
		{"response", ResponseFilter{}},
	}
	for _, item := range filters {
		component, ok := item.value.(interface{ Scope() string })
		if !ok {
			t.Fatalf("%s does not expose Scope", item.name)
		}
		if got := component.Scope(); got != "web:filter" {
			t.Fatalf("%s scope = %q, want web:filter", item.name, got)
		}
	}

	handlers := []struct {
		name  string
		value any
	}{
		{"handler", Handler{}},
		{"notfound", NotFoundHandler{}},
		{"error", ErrorHandler{}},
		{"failed", FailedHandler{}},
		{"unsigned", UnsignedHandler{}},
		{"unauthed", UnauthedHandler{}},
		{"denied", DeniedHandler{}},
	}
	for _, item := range handlers {
		component, ok := item.value.(interface{ Scope() string })
		if !ok {
			t.Fatalf("%s does not expose Scope", item.name)
		}
		if got := component.Scope(); got != "web:handler" {
			t.Fatalf("%s scope = %q, want web:handler", item.name, got)
		}
	}
}

func TestSpecializedComponentsRegisterIntoTheirPipelines(t *testing.T) {
	serve := func(*Context) {}
	request := func(*Context) {}
	execute := func(*Context) {}
	response := func(*Context) {}
	notFound := func(*Context) {}
	errorAction := func(*Context) {}
	failed := func(*Context) {}
	unsigned := func(*Context) {}
	unauthed := func(*Context) {}
	denied := func(*Context) {}

	module := &Module{}
	module.Register("serve", ServeFilter{Name: "serve-name", Desc: "serve-desc", Action: serve})
	module.Register("request", &RequestFilter{Name: "request-name", Desc: "request-desc", Action: request})
	module.Register("execute", ExecuteFilter{Action: execute})
	module.Register("response", &ResponseFilter{Action: response})
	module.Register("notfound", NotFoundHandler{Name: "notfound-name", Desc: "notfound-desc", Action: notFound})
	module.Register("error", &ErrorHandler{Action: errorAction})
	module.Register("failed", FailedHandler{Action: failed})
	module.Register("unsigned", &UnsignedHandler{Action: unsigned})
	module.Register("unauthed", UnauthedHandler{Action: unauthed})
	module.Register("denied", &DeniedHandler{Action: denied})

	if got := module.filters["serve"]; got.Name != "serve-name" || got.Desc != "serve-desc" || got.Serve == nil {
		t.Fatalf("serve filter = %#v", got)
	}
	if got := module.filters["request"]; got.Name != "request-name" || got.Desc != "request-desc" || got.Request == nil {
		t.Fatalf("request filter = %#v", got)
	}
	if module.filters["execute"].Execute == nil || module.filters["response"].Response == nil {
		t.Fatal("specialized filter action was not registered")
	}
	if got := module.handlers["notfound"]; got.Name != "notfound-name" || got.Desc != "notfound-desc" || got.NotFound == nil {
		t.Fatalf("not found handler = %#v", got)
	}
	if module.handlers["error"].Error == nil || module.handlers["failed"].Failed == nil ||
		module.handlers["unsigned"].Unsigned == nil || module.handlers["unauthed"].Unauthed == nil ||
		module.handlers["denied"].Denied == nil {
		t.Fatal("specialized handler action was not registered")
	}
}
