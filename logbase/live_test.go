package logbase

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

// Launched only by the isolated server integration fixture. This is a separate
// client process using the actual public TLS API, never a production endpoint.
func TestLiveAppendAgainstServer(t *testing.T) {
	endpoint := os.Getenv("DUBI_LOG_LIVE_ENDPOINT")
	if endpoint == "" {
		t.Skip("isolated real-server client fixture only")
	}
	pem, e := os.ReadFile(os.Getenv("DUBI_LOG_LIVE_CA"))
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("missing fixture CA")
	}
	h := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	c, e := New(Options{Endpoint: endpoint, Token: os.Getenv("DUBI_LOG_LIVE_TOKEN"), HTTPClient: h, Gzip: true})
	if e != nil {
		t.Fatal(e)
	}
	l, e := c.Logbase("sdk")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rows := make([]json.RawMessage, 32)
	for i := range rows {
		rows[i] = json.RawMessage(fmt.Sprintf(`{"_msg":"sdk explicit repeated repeated repeated repeated","n":%d}`, i))
	}
	b, e := l.NewBatch("live-explicit", rows)
	if e != nil {
		t.Fatal(e)
	}
	first, e := b.Send(ctx)
	if e != nil {
		t.Fatal(e)
	}
	again, e := b.Send(ctx)
	if e != nil || !reflect.DeepEqual(first, again) {
		t.Fatal("real server replay changed IDs", e)
	}
	w, e := l.NewWriter(WriterOptions{BatchRecords: 32})
	if e != nil {
		t.Fatal(e)
	}
	var receipts []*Receipt
	for i := 0; i < 64; i++ {
		r, e := w.Submit(ctx, json.RawMessage(fmt.Sprintf(`{"_msg":"sdk automatic repeated repeated repeated repeated","n":%d}`, i)))
		if e != nil {
			t.Fatal(e)
		}
		receipts = append(receipts, r)
	}
	if e = w.Close(ctx); e != nil {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for _, item := range first.Items {
		seen[item.ID] = true
	}
	for _, r := range receipts {
		item, e := r.Wait(ctx)
		if e != nil || seen[item.ID] || item.ID == "" {
			t.Fatal("real server receipt", item, e)
		}
		seen[item.ID] = true
	}
	if len(seen) != 96 {
		t.Fatal("unique IDs", len(seen))
	}
}

func TestLiveReadAndManagementAgainstServer(t *testing.T) {
	endpoint := os.Getenv("DUBI_LOG_LIVE_ENDPOINT")
	if endpoint == "" {
		t.Skip("isolated real-server client fixture only")
	}
	pem, e := os.ReadFile(os.Getenv("DUBI_LOG_LIVE_CA"))
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("missing CA")
	}
	h := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	c, e := New(Options{Endpoint: endpoint, Token: os.Getenv("DUBI_LOG_LIVE_TOKEN"), HTTPClient: h})
	if e != nil {
		t.Fatal(e)
	}
	l, _ := c.Logbase("sdk-features")
	ctx, cancel := context.WithTimeout(context.Background(), 130*time.Second)
	defer cancel()
	base, e := l.Create(ctx, "feature-create")
	if e != nil {
		t.Fatal(e)
	}
	if got, e := l.Get(ctx); e != nil || got.ID != base.ID {
		t.Fatal(got, e)
	}
	if got, e := c.ListLogbases(ctx, ListLogbasesRequest{}); e != nil || len(got.Logbases) < 1 {
		t.Fatal(got, e)
	}
	input := []json.RawMessage{json.RawMessage(`{"_msg":"ERROR one","service":"api","secret":"hidden"}`), json.RawMessage(`{"_msg":"ERROR two","service":"api","secret":"hidden"}`), json.RawMessage(`{"_msg":"OK three","service":"api","secret":"hidden"}`), json.RawMessage(`{"_msg":"OK four","service":"api","secret":"hidden"}`)}
	appended, e := l.Append(ctx, "feature-append", input)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := l.AppendStatus(ctx, "feature-append"); e != nil || !reflect.DeepEqual(got, appended) {
		t.Fatal(got, e)
	}
	q := QueryRequest{Query: `service:="api"`, Limit: 1, Fields: []string{"service"}}
	pages := l.QueryPages(q)
	seen := map[string]bool{}
	for pages.Next(ctx) {
		for _, r := range pages.Page().Records {
			if seen[r.ID] || len(r.Fields) != 1 || r.Fields["service"] != "api" {
				t.Fatal("projection or pagination", r)
			}
			seen[r.ID] = true
		}
	}
	if pages.Err() != nil || len(seen) != 4 {
		t.Fatal(len(seen), pages.Err())
	}
	early := l.QueryPages(q)
	if !early.Next(ctx) {
		t.Fatal(early.Err())
	}
	if e = early.Close(ctx); e != nil {
		t.Fatal(e)
	}
	empty, e := l.Query(ctx, QueryRequest{Limit: 1, Fields: []string{}})
	if e != nil || len(empty.Records) != 1 || len(empty.Records[0].Fields) != 0 {
		t.Fatal(empty, e)
	}
	if empty.Next != "" {
		if e = l.CancelRead(ctx, QueryRequest{Limit: 1, Fields: []string{}, Cursor: empty.Next}, "query"); e != nil {
			t.Fatal(e)
		}
	}
	if got, e := l.ValidateQuery(ctx, q); e != nil || !got.Valid {
		t.Fatal(got, e)
	}
	if got, e := l.ExplainQuery(ctx, q); e != nil || !got.Valid || !got.BodyVerification || got.ReadAllFields {
		t.Fatal(got, e)
	}
	fields, e := l.FieldNames(ctx, QueryRequest{})
	if e != nil || len(fields.Values) != 3 {
		t.Fatal(fields, e)
	}
	values, e := l.FieldValues(ctx, "service", QueryRequest{})
	if e != nil || !reflect.DeepEqual(values.Values, []string{"api"}) {
		t.Fatal(values, e)
	}
	aq := AggregateRequest{GroupBy: []string{"service"}}
	if got, e := l.Aggregate(ctx, aq); e != nil || !got.Complete || got.Count != 4 {
		t.Fatal(got, e)
	}
	if got, e := l.SetRetention(ctx, "feature-retention", 86400); e != nil || got.RetentionSeconds != 86400 {
		t.Fatal(got, e)
	}
	if got, e := l.Status(ctx); e != nil || got.Records != 4 {
		t.Fatal(got, e)
	}
	if got, e := l.ObserveBacklog(ctx, true); e != nil || !got.Complete || got.Records != 4 {
		t.Fatal(got, e)
	}
	if _, e = l.MaintenanceTasks(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = l.RebuildIndex(ctx, "feature-rebuild"); e != nil {
		t.Fatal(e)
	}
	if _, e = l.RetryMaintenance(ctx, "feature-maintenance"); e != nil {
		t.Fatal(e)
	}
	exported := 0
	eq := QueryRequest{Limit: 1}
	for {
		page, e := l.Export(ctx, eq)
		if e != nil {
			t.Fatal(e)
		}
		exported += len(page.Records)
		if page.Complete {
			break
		}
		if page.Next == "" {
			t.Fatal(page)
		}
		eq.Cursor = page.Next
	}
	if exported != 4 {
		t.Fatal(exported)
	}
	tail, e := l.Tail(ctx, QueryRequest{})
	if e != nil || tail.Cursor == "" {
		t.Fatal(tail, e)
	}
	late := json.RawMessage(fmt.Sprintf(`{"_time":%q,"_msg":"late live","service":"api"}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)))
	lateResult, e := l.Append(ctx, "feature-late", []json.RawMessage{late})
	if e != nil {
		t.Fatal(e)
	}
	tail, e = l.Tail(ctx, QueryRequest{Cursor: tail.Cursor})
	if e != nil || len(tail.Records) != 1 || tail.Records[0].ID != lateResult.Items[0].ID {
		t.Fatal(tail, e)
	}
	if e = l.CancelRead(ctx, QueryRequest{Cursor: tail.Cursor}, "tail"); e != nil {
		t.Fatal(e)
	}
	job, e := l.CreateExport(ctx, "feature-export", CreateExportRequest{})
	if e != nil {
		t.Fatal(e)
	}
	aggregation, e := l.CreateAggregate(ctx, "feature-aggregate", CreateAggregateRequest{AggregateRequest: aq})
	if e != nil {
		t.Fatal(e)
	}
	for _, created := range []ExportJobInfo{job, aggregation} {
		deadline := time.Now().Add(60 * time.Second)
		for {
			info, e := l.JobStatus(ctx, created.ID)
			if e != nil {
				t.Fatal(e)
			}
			if info.State == "completed" {
				break
			}
			if info.State == "failed" || time.Now().After(deadline) {
				t.Fatal(info)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	if got, e := l.ExportPart(ctx, job.ID, 0); e != nil || len(got.Records) == 0 {
		t.Fatal(got, e)
	}
	if got, e := l.AggregateResult(ctx, aggregation.ID); e != nil || !got.Complete || got.Count != 4 {
		t.Fatal(got, e)
	}
	if got, e := l.ListJobs(ctx, JobListRequest{State: "completed"}); e != nil || len(got.Jobs) < 2 {
		t.Fatal(got, e)
	}
	cancelled, e := l.CreateExport(ctx, "feature-cancel-create", CreateExportRequest{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = l.CancelJob(ctx, "feature-cancel", cancelled.ID); e != nil {
		t.Fatal(e)
	}
	rule, e := l.PutAlertRule(ctx, "feature-rule", AlertRuleConfig{Name: "test disabled", Query: "ERROR", Threshold: 1})
	if e != nil {
		t.Fatal(e)
	}
	if got, e := l.ListAlertRules(ctx, ListLogbasesRequest{}); e != nil || len(got.Rules) != 1 {
		t.Fatal(got, e)
	}
	if _, e = l.ListAlertEvents(ctx, ListLogbasesRequest{}); e != nil {
		t.Fatal(e)
	}
	if e = l.DeleteAlertRule(ctx, "feature-delete-rule", rule.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Purge(ctx, "feature-purge"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.CompactReceipts(ctx, "feature-receipts", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Retire(ctx, "feature-retire"); e != nil {
		t.Fatal(e)
	}
	if _, e = l.Query(ctx, QueryRequest{}); e == nil {
		t.Fatal("retired base remained queryable")
	}
}
