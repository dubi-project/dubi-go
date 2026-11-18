package logbase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTenantContextNumericAndRoutedAlertWire(t *testing.T) {
	calls := 0
	h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token" {
			t.Error("request", r.Method, r.Header)
		}
		switch calls {
		case 1:
			var q ContextRequest
			if json.NewDecoder(r.Body).Decode(&q) != nil || q.ID != "a" || q.Source["service"] != "api" || q.Before != 20 || r.URL.Path != "/v1/log/logbases/app/context" || r.Header.Get("Dubi-Request-ID") != "" {
				t.Error(q, r.URL.Path)
			}
			io.WriteString(w, `{"anchor":{"_id":"a","_time":"2026-11-18T00:00:00Z","fields":{"trace_id":"t"}},"before":[],"after":[],"complete":true}`)
		case 2:
			var q AggregateRequest
			if json.NewDecoder(r.Body).Decode(&q) != nil || len(q.Metrics) != 1 || q.Metrics[0] != (AggregateMetric{Op: "sum", Field: "n", As: "total"}) || r.URL.Path != "/v1/log/logbases/app/aggregate" {
				t.Error(q, r.URL.Path)
			}
			io.WriteString(w, `{"rows":[{"count":3,"metrics":{"total":{"value":"9007199254740993.01","count":1,"missing":1,"invalid":1,"exact":true}}}],"complete":true}`)
		case 3:
			var q AlertRuleConfig
			if json.NewDecoder(r.Body).Decode(&q) != nil || len(q.GroupBy) != 1 || len(q.Silences) != 1 || len(q.Routes) != 1 || !q.Routes[0].PreserveDestination || r.Header.Get("Dubi-Request-ID") != "stable" {
				t.Error(q, r.Header)
			}
			io.WriteString(w, `{"id":"logrule_a","group_by":["service"],"routes":[{"name":"api","match":{"service":"api"},"configured":true}],"groups":[{"values":{"service":"api"},"count":3,"firing":true}],"silenced_until":200}`)
		default:
			t.Error("unexpected call")
		}
	}))
	defer h.Close()
	c, _ := New(Options{Endpoint: h.URL, Token: "token", HTTPClient: h.Client()})
	l, _ := c.Logbase("app")
	around, err := l.Context(context.Background(), ContextRequest{ID: "a", Time: "2026-11-18T00:00:00Z", Before: 20, Source: map[string]string{"service": "api"}})
	if err != nil || !around.Complete || around.Anchor.Fields["trace_id"] != "t" {
		t.Fatal(around, err)
	}
	agg, err := l.Aggregate(context.Background(), AggregateRequest{Metrics: []AggregateMetric{{Op: "sum", Field: "n", As: "total"}}})
	if err != nil || len(agg.Rows) != 1 || agg.Rows[0].Metrics["total"].Value == nil || *agg.Rows[0].Metrics["total"].Value != "9007199254740993.01" || !agg.Rows[0].Metrics["total"].Exact {
		t.Fatal(agg, err)
	}
	rule, err := l.PutAlertRule(context.Background(), "stable", AlertRuleConfig{ID: "logrule_a", GroupBy: []string{"service"}, Silences: []AlertSilence{{Start: "2026-11-18T00:00:00Z", End: "2026-11-18T01:00:00Z"}}, Routes: []AlertRoute{{Name: "api", Match: map[string]string{"service": "api"}, PreserveDestination: true}}})
	if err != nil || len(rule.Groups) != 1 || !rule.Groups[0].Firing || len(rule.Routes) != 1 || !rule.Routes[0].Configured || rule.SilencedUntil != 200 || calls != 3 {
		t.Fatal(rule, err, calls)
	}
}
