package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dubi "github.com/dubi-project/dubi-go"
	. "github.com/dubi-project/dubi-go/base"
)

func TestJSONPriceIntegerMapping(t *testing.T) {
	testRuntime()
	t.Cleanup(dubi.Reset)
	server := newServerState(ServerConfig{Name: defaultServerName}, map[string]struct{}{defaultServerName: {}})
	if err := server.register(map[string]Router{"price": {
		Method: http.MethodPost,
		Uri:    "/price",
		Args:   Vars{"price": {Type: "int64", Required: true}},
		Action: func(ctx *Context) {
			if ctx.Args["price"] != int64(1000000) {
				t.Errorf("mapped price: %#v", ctx.Args["price"])
			}
			ctx.JSON(ctx.Args)
		},
	}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body string
		code int
	}{
		{`{"price":1000000}`, http.StatusOK},
		{`{"price":1e6}`, http.StatusOK},
		{`{"price":1000000.0}`, http.StatusOK},
		{`{"price":"1000000"}`, http.StatusOK},
		{`{"price":1000000.5}`, http.StatusBadRequest},
		{`{"price":9223372036854775808}`, http.StatusBadRequest},
	} {
		t.Run(test.body, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://example.test/price", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.serveHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.code == http.StatusOK && response.Body.String() != `{"price":1000000}` {
				t.Fatalf("response=%s", response.Body.String())
			}
		})
	}
}
