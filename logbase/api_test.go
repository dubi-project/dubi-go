package logbase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestQueryPagesFreezeProjectionAndCancel(t *testing.T) {
	calls := 0
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("Dubi-Request-ID") != "" {
			t.Error("headers", r.Header)
		}
		var in struct {
			QueryRequest
			Mode string `json:"mode"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			t.Error("decode")
		}
		if in.Fields == nil || len(in.Fields) != 0 || in.Query != "ERROR" || in.Limit != 1 {
			t.Error("query changed", in)
		}
		if calls == 1 {
			if in.Cursor != "" {
				t.Error(in)
			}
			io.WriteString(w, `{"records":[{"_id":"a"}],"next":"first","complete":false}`)
		} else if calls == 2 {
			if in.Cursor != "first" {
				t.Error(in)
			}
			io.WriteString(w, `{"records":[{"_id":"b"}],"next":"second","complete":false}`)
		} else {
			if in.Mode != "query" || in.Cursor != "second" || r.URL.Path != "/v1/log/logbases/app/cancel-read" {
				t.Error(in, r.URL.Path)
			}
			io.WriteString(w, `{"cancelled":true}`)
		}
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client()})
	l, _ := c.Logbase("app")
	q := QueryRequest{Query: "ERROR", Limit: 1, Fields: []string{}}
	pages := l.QueryPages(q)
	q.Query = "changed"
	q.Fields = append(q.Fields, "secret")
	if !pages.Next(context.Background()) || pages.Page().Records[0].ID != "a" {
		t.Fatal(pages.Err())
	}
	if !pages.Next(context.Background()) || pages.Page().Records[0].ID != "b" {
		t.Fatal(pages.Err())
	}
	if e := pages.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if pages.Next(context.Background()) || calls != 3 {
		t.Fatal(calls)
	}
	raw, _ := json.Marshal(QueryRequest{})
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	if string(fields["fields"]) != "null" {
		t.Fatal("all-fields lost", string(raw))
	}
}

func TestAPIRequestsNeverReplayLostReadsOrMutations(t *testing.T) {
	calls := 0
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/v1/log/logbases/app/create" && r.Header.Get("Dubi-Request-ID") != "stable" {
			t.Error("identity missing")
		}
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return
		}
		conn.Close()
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client(), MaxAttempts: 3})
	l, _ := c.Logbase("app")
	_, e := l.Query(context.Background(), QueryRequest{Cursor: "possibly-consumed"})
	var api *APIError
	if !errors.As(e, &api) || api.MayHaveCommitted || calls != 1 {
		t.Fatal(e, calls)
	}
	_, e = l.Create(context.Background(), "stable")
	if !errors.As(e, &api) || !api.MayHaveCommitted || api.RequestID != "stable" || calls != 2 {
		t.Fatal(e, calls)
	}
}

func TestAPIGuidanceAndMalformedSuccess(t *testing.T) {
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/log/logbases/app/aggregate" {
			w.WriteHeader(429)
			io.WriteString(w, `{"code":"budget_exceeded","message":"bounded scan","suggestions":["use_create_aggregate"]}`)
			return
		}
		io.WriteString(w, `null`)
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client()})
	l, _ := c.Logbase("app")
	_, e := l.Aggregate(context.Background(), AggregateRequest{})
	var api *APIError
	if !errors.As(e, &api) || api.Code != "budget_exceeded" || !reflect.DeepEqual(api.Suggestions, []string{"use_create_aggregate"}) {
		t.Fatal(e)
	}
	_, e = l.Create(context.Background(), "stable")
	if !errors.As(e, &api) || api.Code != "invalid_response" || !api.MayHaveCommitted {
		t.Fatal(e)
	}
}

func TestObserveBacklogRejectsMixedSnapshotsAndReleasesCursor(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "sum", true: "changed"}[changed], func(t *testing.T) {
			pages, cancels := 0, 0
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var q BacklogRequest
				json.NewDecoder(r.Body).Decode(&q)
				if !q.IncludeIngestionAge {
					t.Error("option lost")
				}
				if r.URL.Path == "/v1/log/logbases/app/cancel-backlog" {
					cancels++
					if q.Cursor != "next" {
						t.Error(q)
					}
					io.WriteString(w, `{"cancelled":true}`)
					return
				}
				pages++
				p := BacklogResult{SnapshotID: "cut", ObservedAt: 10, IngestionAgeIncluded: true, Batches: 1, Records: 2, PendingBatches: 1, PendingRecords: 2, PendingInputBytes: 20, OldestPendingIngestedAt: "2026-01-01T00:00:00Z", OldestPendingAgeSeconds: 5}
				if pages == 1 {
					p.Next = "next"
				} else {
					p.Complete = true
					if changed {
						p.SnapshotID = "another"
					}
				}
				json.NewEncoder(w).Encode(p)
			}))
			defer h.Close()
			c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client()})
			l, _ := c.Logbase("app")
			got, e := l.ObserveBacklog(context.Background(), true)
			if changed {
				if e == nil || got.Complete || cancels != 1 {
					t.Fatal(got, e, cancels)
				}
			} else if e != nil || !got.Complete || got.PendingRecords != 4 || got.PendingInputBytes != 40 || got.Batches != 2 || cancels != 0 {
				t.Fatal(got, e, cancels)
			}
		})
	}
}
