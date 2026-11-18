package logbase

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func (l *Logbase) Get(ctx context.Context) (LogbaseInfo, error) {
	return apiCall[LogbaseInfo](ctx, l.c, http.MethodGet, l.path(""), "", nil, false)
}

func (l *Logbase) Create(ctx context.Context, requestID string) (LogbaseInfo, error) {
	return apiCall[LogbaseInfo](ctx, l.c, http.MethodPost, l.path("create"), requestID, struct{}{}, true)
}

func (l *Logbase) Retire(ctx context.Context, requestID string) (LogbaseInfo, error) {
	return apiCall[LogbaseInfo](ctx, l.c, http.MethodPost, l.path("retire"), requestID, struct{}{}, true)
}

func (l *Logbase) Status(ctx context.Context) (LogbaseStatus, error) {
	return apiCall[LogbaseStatus](ctx, l.c, http.MethodPost, l.path("status"), "", struct{}{}, false)
}

func (l *Logbase) Query(ctx context.Context, q QueryRequest) (QueryResult, error) {
	return apiCall[QueryResult](ctx, l.c, http.MethodPost, l.path("query"), "", q, false)
}

func (l *Logbase) ValidateQuery(ctx context.Context, q QueryRequest) (QueryValidation, error) {
	return apiCall[QueryValidation](ctx, l.c, http.MethodPost, l.path("validate-query"), "", q, false)
}

func (l *Logbase) ExplainQuery(ctx context.Context, q QueryRequest) (QueryExplanation, error) {
	return apiCall[QueryExplanation](ctx, l.c, http.MethodPost, l.path("explain-query"), "", q, false)
}

func (l *Logbase) FieldNames(ctx context.Context, q QueryRequest) (FieldResult, error) {
	return apiCall[FieldResult](ctx, l.c, http.MethodPost, l.path("field-names"), "", q, false)
}

func (l *Logbase) Aggregate(ctx context.Context, q AggregateRequest) (AggregateResult, error) {
	return apiCall[AggregateResult](ctx, l.c, http.MethodPost, l.path("aggregate"), "", q, false)
}

func (l *Logbase) Export(ctx context.Context, q QueryRequest) (ExportResult, error) {
	return apiCall[ExportResult](ctx, l.c, http.MethodPost, l.path("export"), "", q, false)
}

func (l *Logbase) Tail(ctx context.Context, q QueryRequest) (TailResult, error) {
	return apiCall[TailResult](ctx, l.c, http.MethodPost, l.path("tail"), "", q, false)
}

func (l *Logbase) CreateExport(ctx context.Context, requestID string, q CreateExportRequest) (ExportJobInfo, error) {
	return apiCall[ExportJobInfo](ctx, l.c, http.MethodPost, l.path("create-export"), requestID, q, true)
}

func (l *Logbase) CreateAggregate(ctx context.Context, requestID string, q CreateAggregateRequest) (ExportJobInfo, error) {
	return apiCall[ExportJobInfo](ctx, l.c, http.MethodPost, l.path("create-aggregate"), requestID, q, true)
}

func (l *Logbase) JobStatus(ctx context.Context, id string) (ExportJobInfo, error) {
	return apiCall[ExportJobInfo](ctx, l.c, http.MethodPost, l.path("export-status"), "", map[string]any{"id": id}, false)
}

func (l *Logbase) CancelJob(ctx context.Context, requestID string, id string) (ExportJobInfo, error) {
	return apiCall[ExportJobInfo](ctx, l.c, http.MethodPost, l.path("cancel-export"), requestID, map[string]any{"id": id}, true)
}

func (l *Logbase) RetryJob(ctx context.Context, requestID string, id string) (ExportJobInfo, error) {
	return apiCall[ExportJobInfo](ctx, l.c, http.MethodPost, l.path("retry-job"), requestID, map[string]any{"id": id}, true)
}

func (l *Logbase) ExportPart(ctx context.Context, id string, part int) (ExportResult, error) {
	return apiCall[ExportResult](ctx, l.c, http.MethodPost, l.path("export-part"), "", map[string]any{"id": id, "part": part}, false)
}

func (l *Logbase) AggregateResult(ctx context.Context, id string) (AggregateResult, error) {
	return apiCall[AggregateResult](ctx, l.c, http.MethodPost, l.path("aggregate-result"), "", map[string]any{"id": id}, false)
}

func (l *Logbase) ListJobs(ctx context.Context, q JobListRequest) (JobListResult, error) {
	return apiCall[JobListResult](ctx, l.c, http.MethodPost, l.path("jobs"), "", q, false)
}

func (l *Logbase) Backlog(ctx context.Context, q BacklogRequest) (BacklogResult, error) {
	return apiCall[BacklogResult](ctx, l.c, http.MethodPost, l.path("backlog"), "", q, false)
}

func (l *Logbase) RebuildIndex(ctx context.Context, requestID string) (MaintenanceTask, error) {
	return apiCall[MaintenanceTask](ctx, l.c, http.MethodPost, l.path("rebuild-index"), requestID, struct{}{}, true)
}

func (l *Logbase) RetryMaintenance(ctx context.Context, requestID string) (MaintenanceTask, error) {
	return apiCall[MaintenanceTask](ctx, l.c, http.MethodPost, l.path("retry-maintenance"), requestID, struct{}{}, true)
}

func (l *Logbase) MaintenanceTasks(ctx context.Context) (MaintenanceTasksResult, error) {
	return apiCall[MaintenanceTasksResult](ctx, l.c, http.MethodPost, l.path("maintenance-tasks"), "", struct{}{}, false)
}

func (l *Logbase) AppendStatus(ctx context.Context, id string) (AppendResult, error) {
	return apiCall[AppendResult](ctx, l.c, http.MethodPost, l.path("append-status"), "", map[string]any{"request_id": id}, false)
}

func (l *Logbase) SetRetention(ctx context.Context, requestID string, seconds int64) (LogbaseInfo, error) {
	return apiCall[LogbaseInfo](ctx, l.c, http.MethodPost, l.path("retention"), requestID, map[string]any{"retention_seconds": seconds}, true)
}

func (l *Logbase) PutAlertRule(ctx context.Context, requestID string, q AlertRuleConfig) (AlertRuleInfo, error) {
	return apiCall[AlertRuleInfo](ctx, l.c, http.MethodPost, l.path("put-alert-rule"), requestID, q, true)
}

func (l *Logbase) ListAlertRules(ctx context.Context, q ListLogbasesRequest) (AlertRulesResult, error) {
	return apiCall[AlertRulesResult](ctx, l.c, http.MethodPost, l.path("alert-rules"), "", q, false)
}

func (l *Logbase) ListAlertEvents(ctx context.Context, q ListLogbasesRequest) (AlertEventsResult, error) {
	return apiCall[AlertEventsResult](ctx, l.c, http.MethodPost, l.path("alert-events"), "", q, false)
}

func (l *Logbase) path(op string) string {
	p := "/v1/log/logbases/" + url.PathEscape(l.name)
	if op != "" {
		p += "/" + op
	}
	return p
}
func (c *Client) ListLogbases(ctx context.Context, q ListLogbasesRequest) (ListLogbasesResult, error) {
	values := url.Values{}
	if q.Limit != 0 {
		values.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.After != "" {
		values.Set("after", q.After)
	}
	p := "/v1/log/logbases"
	if len(values) > 0 {
		p += "?" + values.Encode()
	}
	return apiCall[ListLogbasesResult](ctx, c, http.MethodGet, p, "", nil, false)
}
func (l *Logbase) FieldValues(ctx context.Context, field string, q QueryRequest) (FieldResult, error) {
	return apiCall[FieldResult](ctx, l.c, http.MethodPost, l.path("field-values"), "", struct {
		QueryRequest
		Field string `json:"field"`
	}{q, field}, false)
}
func (l *Logbase) CancelRead(ctx context.Context, q QueryRequest, mode string) error {
	_, e := apiCall[struct {
		Cancelled bool `json:"cancelled"`
	}](ctx, l.c, http.MethodPost, l.path("cancel-read"), "", struct {
		QueryRequest
		Mode string `json:"mode"`
	}{q, mode}, false)
	return e
}
func (l *Logbase) CancelBacklog(ctx context.Context, q BacklogRequest) error {
	_, e := apiCall[struct {
		Cancelled bool `json:"cancelled"`
	}](ctx, l.c, http.MethodPost, l.path("cancel-backlog"), "", q, false)
	return e
}
func (l *Logbase) Purge(ctx context.Context, requestID string) (int, error) {
	r, e := apiCall[struct {
		Removed int `json:"removed_records"`
	}](ctx, l.c, http.MethodPost, l.path("purge"), requestID, struct{}{}, true)
	return r.Removed, e
}
func (l *Logbase) DeleteAlertRule(ctx context.Context, requestID, id string) error {
	_, e := apiCall[struct {
		Deleted bool `json:"deleted"`
	}](ctx, l.c, http.MethodPost, l.path("delete-alert-rule"), requestID, map[string]string{"id": id}, true)
	return e
}
func (c *Client) CompactReceipts(ctx context.Context, requestID, after string) (ReceiptMaintenanceResult, error) {
	return apiCall[ReceiptMaintenanceResult](ctx, c, http.MethodPost, "/v1/log/receipts/compact", requestID, map[string]string{"after": after}, true)
}

// Context returns nearest event-time neighbours at one protected read cut.
func (l *Logbase) Context(ctx context.Context, q ContextRequest) (ContextResult, error) {
	return apiCall[ContextResult](ctx, l.c, http.MethodPost, l.path("context"), "", q, false)
}
