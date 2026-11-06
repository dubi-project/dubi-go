package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestContextRequestCallsBusAlongsideNativeHTTP(t *testing.T) {
	rt := testRuntime()
	rt.Register("echo", dubi.Service{Action: func(ctx *dubi.Context) Map {
		return Map{"value": ctx.Value["value"], "id": ctx.Id()}
	}})
	// With no Web routes or configuration, starting BUS opens no HTTP listener.
	if err := rt.Run(""); err != nil {
		t.Fatal(err)
	}
	defer rt.End()
	request := httptest.NewRequest("GET", "/?value=hello", nil)
	response := httptest.NewRecorder()
	ctx := acquireContext(response, request)
	id := ctx.Id()
	data := ctx.Request("echo", Map{"value": ctx.HTTP.Request.URL.Query().Get("value")})
	if ctx.Result() == nil || ctx.Result().Fail() || data["value"] != "hello" || data["id"] != id {
		t.Fatalf("Web BUS request = %v, %v", data, ctx.Result())
	}
	ctx.HTTP.Response.Header().Set("X-Native", "yes")
	if err := json.NewEncoder(ctx.HTTP.Response).Encode(data); err != nil {
		t.Fatal(err)
	}
	if response.Header().Get("X-Native") != "yes" || response.Body.Len() == 0 {
		t.Fatal("native HTTP response failed")
	}
	releaseContext(ctx)
	if ctx.HTTP.Request != nil || ctx.HTTP.Response != nil {
		t.Fatal("pooled context retained HTTP objects")
	}
}
