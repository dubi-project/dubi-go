package logbase

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRetryKeepsFrozenCompressedBatchAfterLostResponse(t *testing.T) {
	var bodies [][]byte
	var mu sync.Mutex
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, raw)
		attempt := len(bodies)
		mu.Unlock()
		if r.Header.Get("Dubi-Request-ID") != "stable" || r.Header.Get("Content-Encoding") != "gzip" || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("headers", r.Header)
		}
		z, e := gzip.NewReader(bytes.NewReader(raw))
		if e != nil {
			t.Error(e)
			return
		}
		decoded, _ := io.ReadAll(z)
		z.Close()
		if !bytes.Contains(decoded, []byte("9007199254740993")) {
			t.Error("number changed")
		}
		if attempt == 1 {
			conn, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			conn.Close()
			return
		}
		if attempt == 2 {
			w.WriteHeader(503)
			io.WriteString(w, `{"code":"outcome_unknown","append_result":{"items":[{"index":0,"id":"original","status":"committed"}]}}`)
			return
		}
		io.WriteString(w, `{"logbase_id":"logbase_a","complete":true,"items":[{"index":0,"id":"original","status":"committed"}]}`)
	}))
	defer h.Close()
	c, e := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client(), Gzip: true, RetryDelay: time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	l, _ := c.Logbase("app")
	raw := json.RawMessage(`{"_msg":"` + string(bytes.Repeat([]byte{'x'}, 500)) + `","n":9007199254740993}`)
	b, e := l.NewBatch("stable", []json.RawMessage{raw})
	if e != nil {
		t.Fatal(e)
	}
	clear(raw)
	r, e := b.Send(context.Background())
	if e != nil || !r.Complete || r.Items[0].ID != "original" {
		t.Fatal(r, e)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 || !reflect.DeepEqual(bodies[0], bodies[1]) || !reflect.DeepEqual(bodies[1], bodies[2]) {
		t.Fatal("retry changed wire body", len(bodies))
	}
}

func TestWriterBackpressureAndClosePreserveReceipts(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		io.WriteString(w, `{"logbase_id":"logbase_a","complete":true,"items":[{"index":0,"id":"one","status":"committed"},{"index":1,"id":"two","status":"committed"}]}`)
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client()})
	l, _ := c.Logbase("app")
	w, e := l.NewWriter(WriterOptions{BatchRecords: 2, PendingRecords: 2, FlushInterval: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, e := w.Submit(ctx, json.RawMessage(`{"_msg":"one"}`))
	if e != nil {
		t.Fatal(e)
	}
	b, e := w.Submit(ctx, json.RawMessage(`{"_msg":"two"}`))
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, e = w.Submit(ctx, json.RawMessage(`{}`)); !errors.Is(e, ErrBackpressure) {
		t.Fatal(e)
	}
	short, stop := context.WithTimeout(ctx, time.Millisecond)
	defer stop()
	if _, e = a.Wait(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if e = w.Close(short); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if _, e = w.Submit(ctx, json.RawMessage(`{}`)); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	close(release)
	for i, r := range []*Receipt{a, b} {
		v, e := r.Wait(ctx)
		if e != nil || v.Index != i || v.Status != "committed" {
			t.Fatal(v, e)
		}
	}
	if e = w.Close(ctx); e != nil {
		t.Fatal(e)
	}
	if s := w.Statistics(); s.PendingRecords != 0 || s.PendingBytes != 0 {
		t.Fatal(s)
	}
}

func TestPartialFailureCanRetryOriginalBatch(t *testing.T) {
	var attempts int
	var ids []string
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		ids = append(ids, r.Header.Get("Dubi-Request-ID"))
		if attempts == 1 {
			w.WriteHeader(503)
			io.WriteString(w, `{"code":"outcome_unknown","append_result":{"logbase_id":"logbase_a","complete":false,"items":[{"index":0,"id":"one","status":"committed"},{"index":1,"id":"two","status":"pending"}]}}`)
			return
		}
		io.WriteString(w, `{"logbase_id":"logbase_a","complete":true,"items":[{"index":0,"id":"one","status":"committed"},{"index":1,"id":"two","status":"committed"}]}`)
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client(), MaxAttempts: 1})
	l, _ := c.Logbase("app")
	w, _ := l.NewWriter(WriterOptions{BatchRecords: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, _ := w.Submit(ctx, json.RawMessage(`{"n":1}`))
	b, _ := w.Submit(ctx, json.RawMessage(`{"n":2}`))
	if item, e := a.Wait(ctx); e != nil || item.ID != "one" {
		t.Fatal(item, e)
	}
	_, e := b.Wait(ctx)
	var failure *AppendError
	if !errors.As(e, &failure) || !failure.MayHaveCommitted || failure.Batch == nil {
		t.Fatal(e)
	}
	r, e := failure.Batch.Send(ctx)
	if e != nil || r.Items[0].ID != "one" || len(ids) != 2 || ids[0] != ids[1] {
		t.Fatal(r, e, ids)
	}
	if e = w.Close(ctx); e != nil {
		t.Fatal(e)
	}
}

func TestClientRejectsMalformedSuccessAndRedirect(t *testing.T) {
	for _, mode := range []string{"malformed", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					w.Header().Set("Location", "https://example.invalid")
					w.WriteHeader(307)
					return
				}
				fmt.Fprint(w, `{"logbase_id":"base","complete":true,"items":[{"index":0,"id":"one","status":"pending"}]}`)
			}))
			defer h.Close()
			c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client(), MaxAttempts: 1})
			l, _ := c.Logbase("app")
			_, e := l.Append(context.Background(), "a", []json.RawMessage{json.RawMessage(`{}`)})
			var failure *AppendError
			if !errors.As(e, &failure) {
				t.Fatal(e)
			}
			if mode == "malformed" && !failure.MayHaveCommitted {
				t.Fatal("invalid success treated as rejection")
			}
		})
	}
}
