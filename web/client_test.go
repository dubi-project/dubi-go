package web

import (
	"net/http/httptest"
	"testing"
)

func TestContextIP(t *testing.T) {
	for _, test := range []struct {
		name, peer, forwarded, real, want string
	}{
		{name: "connection", peer: "192.0.2.1:1234", want: "192.0.2.1"},
		{name: "ipv6 connection", peer: "[2001:db8::1]:1234", want: "2001:db8::1"},
		{name: "real IP", peer: "192.0.2.1:1234", real: "203.0.113.1", want: "203.0.113.1"},
		{name: "forwarded precedence", peer: "192.0.2.1:1234", forwarded: "203.0.113.2", real: "203.0.113.1", want: "203.0.113.2"},
		{name: "last forwarded", forwarded: "203.0.113.1, 198.51.100.1", want: "198.51.100.1"},
		{name: "no separator space", forwarded: "203.0.113.1,198.51.100.1", want: "198.51.100.1"},
		{name: "forwarded port", forwarded: "203.0.113.1, [2001:db8::1]:443", want: "2001:db8::1"},
		{name: "empty forwarded items", forwarded: " , ", real: "203.0.113.1", want: "203.0.113.1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "http://example.test/", nil)
			request.RemoteAddr = test.peer
			request.Header.Set("X-Forwarded-For", test.forwarded)
			request.Header.Set("X-Real-IP", test.real)
			ctx := &Context{Request: request, RemoteIP: "trusted-policy-result"}
			if got := ctx.IP(); got != test.want {
				t.Fatalf("IP = %q, want %q", got, test.want)
			}
			if got := ctx.Ip(); got != test.want {
				t.Fatalf("Ip = %q, want %q", got, test.want)
			}
			if ctx.ClientIP() != "trusted-policy-result" {
				t.Fatal("IP changed the trusted proxy result")
			}
		})
	}
}

func TestContextAgent(t *testing.T) {
	request := httptest.NewRequest("GET", "http://example.test/", nil)
	request.Header.Set("User-Agent", "ByOne/1.0 (test agent)")
	ctx := &Context{Request: request}
	if got := ctx.Agent(); got != "ByOne/1.0 (test agent)" {
		t.Fatalf("Agent = %q", got)
	}
	request.Header.Del("User-Agent")
	if got := ctx.Agent(); got != "" {
		t.Fatalf("missing Agent = %q", got)
	}
}
