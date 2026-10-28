package web

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func captureListenerLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &output
}

func TestWebStartupLogsActualListeningAddress(t *testing.T) {
	output := captureListenerLog(t)
	server := newServerState(ServerConfig{Name: "public", Host: "127.0.0.1", Port: freePort(t)}, map[string]struct{}{"public": {}})
	if err := server.register(map[string]Router{
		"public.index": {Uri: "/", Action: func(ctx *Context) { ctx.Text("ready") }},
	}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := server.open(); err != nil {
		t.Fatal(err)
	}
	defer server.close(context.Background())
	address := server.listener.Addr().String()
	server.start()
	server.start() // Re-entering the module must not create duplicate accept loops.
	response, err := http.Get("http://" + address + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || string(body) != "ready" {
		t.Fatalf("listener unavailable: status=%d body=%q error=%v", response.StatusCode, body, readErr)
	}
	for _, expected := range []string{`dubi web: listening server="public"`, "url=http://" + address, "sites=0"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("listener log missing %q: %s", expected, output)
		}
	}
	if count := strings.Count(output.String(), "dubi web: listening"); count != 1 {
		t.Fatalf("listener logged %d starts: %s", count, output)
	}
}

func TestFailedBindDoesNotLogListening(t *testing.T) {
	output := captureListenerLog(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	_, portText, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	server := newServerState(ServerConfig{Name: "occupied", Host: "127.0.0.1", Port: port}, nil)
	if err := server.open(); err == nil {
		defer server.close(context.Background())
		t.Fatal("occupied address opened successfully")
	}
	server.start()
	if strings.Contains(output.String(), "dubi web: listening") {
		t.Fatalf("failed listener reported success: %s", output)
	}
}
