package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestInputPathTokens(t *testing.T) {
	tests := map[string][]string{
		"user.name":    {"user", "name"},
		"users[0].id":  {"users", "0", "id"},
		"tags[]":       {"tags", ""},
		"matrix[0][1]": {"matrix", "0", "1"},
		"broken[":      {"broken", "["},
	}
	for input, want := range tests {
		if got := inputPathTokens(input); !reflect.DeepEqual(got, want) {
			t.Fatalf("inputPathTokens(%q) = %#v, want %#v", input, got, want)
		}
	}
}

func TestPooledContextClearsRequestState(t *testing.T) {
	runtime := dubi.New()
	first := acquireContext(runtime, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	first.Params["stale"] = true
	first.Locals["stale"] = true
	first.Body = "stale"
	releaseContext(first)

	second := acquireContext(runtime, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	defer releaseContext(second)
	if len(second.Params) != 0 || len(second.Locals) != 0 || second.Body != nil || second.Code != 0 {
		t.Fatalf("pooled context retained state: params=%#v locals=%#v body=%#v code=%d", second.Params, second.Locals, second.Body, second.Code)
	}
}

func TestContextStatusCodePredictsResponseStatus(t *testing.T) {
	runtime := dubi.New()
	ctx := acquireContext(runtime, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	defer releaseContext(ctx)
	if got, want := ctx.StatusCode(), http.StatusNoContent; got != want {
		t.Fatalf("empty status = %d, want %d", got, want)
	}
	ctx.Text("ok")
	if got, want := ctx.StatusCode(), http.StatusOK; got != want {
		t.Fatalf("body status = %d, want %d", got, want)
	}
	ctx.Status(http.StatusCreated)
	if got, want := ctx.StatusCode(), http.StatusCreated; got != want {
		t.Fatalf("explicit status = %d, want %d", got, want)
	}
}

func TestAssignInputValueNested(t *testing.T) {
	values := Map{}
	assignInputValue(values, "users[0].name", "Ada")
	assignInputValue(values, "users[1].name", "Grace")
	assignInputValue(values, "tags[]", "go")
	assignInputValue(values, "tags[]", "dubi")
	users, ok := values["users"].([]any)
	if !ok || len(users) != 2 || users[0].(Map)["name"] != "Ada" || users[1].(Map)["name"] != "Grace" {
		t.Fatalf("users = %#v", values["users"])
	}
	if want := []any{"go", "dubi"}; !reflect.DeepEqual(values["tags"], want) {
		t.Fatalf("tags = %#v, want %#v", values["tags"], want)
	}
}
