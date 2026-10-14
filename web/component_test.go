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
