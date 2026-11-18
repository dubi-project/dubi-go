package logbase

// Public wire types are independent of the server module and its dependencies.

type LogbaseInfo struct {
	RetiredAt        int64  `json:"retired_at,omitempty"`
	DeletedBefore    int64  `json:"deleted_before_ns,omitempty"`
	ID               string `json:"id"`
	Name             string `json:"name"`
	RetentionSeconds int64  `json:"retention_seconds"`
}

type Record struct {
	ID         string            `json:"_id"`
	Time       string            `json:"_time"`
	IngestedAt string            `json:"_ingested_at"`
	Fields     map[string]string `json:"fields"`
}

type QueryRequest struct {
	Query  string   `json:"query,omitempty"`
	Start  string   `json:"start,omitempty"`
	End    string   `json:"end,omitempty"`
	Limit  int      `json:"limit,omitempty"`
	Cursor string   `json:"cursor,omitempty"`
	Fields []string `json:"fields"`
}

type QueryResult struct {
	Records          []Record `json:"records"`
	Next             string   `json:"next,omitempty"`
	Complete         bool     `json:"complete"`
	Scanned          int      `json:"scanned"`
	ReadBytes        int64    `json:"read_bytes"`
	SummaryBytes     int64    `json:"summary_bytes"`
	ExaminedBatches  int      `json:"examined_batches"`
	SkippedBatches   int      `json:"skipped_batches"`
	UnindexedBatches int      `json:"unindexed_batches"`
}

type FieldResult struct {
	Values   []string `json:"values"`
	Complete bool     `json:"complete"`
}

type ListLogbasesRequest struct {
	After string `json:"after,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

type ListLogbasesResult struct {
	Logbases []LogbaseInfo `json:"logbases"`
	Next     string        `json:"next,omitempty"`
}

type LogbaseStatus struct {
	PendingCleanupContainers int               `json:"pending_cleanup_containers"`
	CleanupComplete          bool              `json:"cleanup_complete"`
	EncodedBodyBytes         int64             `json:"encoded_body_bytes"`
	StorageComplete          bool              `json:"storage_complete"`
	Logbase                  LogbaseInfo       `json:"logbase"`
	ObservedAt               string            `json:"observed_at"`
	Complete                 bool              `json:"complete"`
	Batches                  int               `json:"batches"`
	Records                  int               `json:"records"`
	InputBytes               int64             `json:"input_bytes"`
	UnknownSizeBatches       int               `json:"unknown_size_batches"`
	UnmarkedIndexBatches     int               `json:"unmarked_index_batches"`
	ExpiredBatches           int               `json:"expired_batches"`
	OldestUnmarkedEventTime  string            `json:"oldest_unmarked_event_time,omitempty"`
	Maintenance              MaintenanceHealth `json:"maintenance"`
	ReceiptPolicy            string            `json:"receipt_policy"`
}

type MaintenanceHealth struct {
	LastSuccessAt       int64  `json:"last_success_at,omitempty"`
	NextAttemptAt       int64  `json:"next_attempt_at,omitempty"`
	LastFailureAt       int64  `json:"last_failure_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
	Failures            uint64 `json:"failures_since_last_success,omitempty"`
	IndexedBatches      uint64 `json:"indexed_batches"`
	PurgedRecords       uint64 `json:"purged_records"`
	CompactPages        uint64 `json:"compact_pages"`
	DeferredCompactions uint64 `json:"deferred_compactions"`
}

type AggregateMetric struct {
	Op    string `json:"op"`
	Field string `json:"field"`
	As    string `json:"as"`
}
type AggregateValue struct {
	Value   *string `json:"value"`
	Count   uint64  `json:"count"`
	Missing uint64  `json:"missing"`
	Invalid uint64  `json:"invalid"`
	Exact   bool    `json:"exact"`
}
type ContextRequest struct {
	ID            string            `json:"id"`
	Time          string            `json:"time"`
	Source        map[string]string `json:"source,omitempty"`
	Before        int               `json:"before,omitempty"`
	After         int               `json:"after,omitempty"`
	WindowSeconds int64             `json:"window_seconds,omitempty"`
}
type ContextResult struct {
	Anchor   Record   `json:"anchor"`
	Before   []Record `json:"before"`
	After    []Record `json:"after"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Scanned  int      `json:"scanned"`
	Complete bool     `json:"complete"`
}
type AggregateRequest struct {
	Metrics []AggregateMetric `json:"metrics,omitempty"`
	QueryRequest
	GroupBy         []string `json:"group_by,omitempty"`
	IntervalSeconds int64    `json:"interval_seconds,omitempty"`
	TopN            int      `json:"top_n,omitempty"`
}

type AggregateRow struct {
	Metrics map[string]AggregateValue `json:"metrics,omitempty"`
	Values  map[string]*string        `json:"values,omitempty"`
	Time    string                    `json:"time,omitempty"`
	Count   uint64                    `json:"count"`
}

type AggregateResult struct {
	Request  *AggregateRequest `json:"request,omitempty"`
	Rows     []AggregateRow    `json:"rows"`
	Count    uint64            `json:"count"`
	Complete bool              `json:"complete"`
	Scanned  int               `json:"scanned"`
}

type ExportResult struct {
	Records   []Record `json:"records"`
	Next      string   `json:"next,omitempty"`
	Complete  bool     `json:"complete"`
	Scanned   int      `json:"scanned"`
	ReadBytes int64    `json:"read_bytes"`
}

type TailResult struct {
	Records  []Record `json:"records"`
	Cursor   string   `json:"cursor"`
	CaughtUp bool     `json:"caught_up"`
	Scanned  int      `json:"scanned"`
}

type CreateExportRequest struct {
	QueryRequest
	RunSeconds       int `json:"run_seconds,omitempty"`
	ResultTTLSeconds int `json:"result_ttl_seconds,omitempty"`
}

type ExportJobInfo struct {
	Kind      string `json:"kind,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
	Scanned   int64  `json:"scanned"`
	Failures  uint64 `json:"failures"`
	LastError string `json:"last_error,omitempty"`
	ID        string `json:"id"`
	LogbaseID string `json:"logbase_id"`
	State     string `json:"state"`
	CreatedAt int64  `json:"created_at"`
	Deadline  int64  `json:"deadline"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
	Parts     int    `json:"parts"`
	Records   uint64 `json:"records"`
	Error     string `json:"error,omitempty"`
	Cleaned   bool   `json:"cleaned"`
}

type CreateAggregateRequest struct {
	AggregateRequest
	RunSeconds       int `json:"run_seconds,omitempty"`
	ResultTTLSeconds int `json:"result_ttl_seconds,omitempty"`
}

type JobListRequest struct {
	Limit int    `json:"limit,omitempty"`
	After string `json:"after,omitempty"`
	State string `json:"state,omitempty"`
}

type JobListResult struct {
	Jobs       []ExportJobInfo `json:"jobs"`
	Next       string          `json:"next,omitempty"`
	ObservedAt int64           `json:"observed_at"`
}

type MaintenanceTask struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	State            string `json:"state"`
	RequestedAt      int64  `json:"requested_at,omitempty"`
	UpdatedAt        int64  `json:"updated_at,omitempty"`
	ProcessedBatches uint64 `json:"processed_batches"`
	LastError        string `json:"last_error,omitempty"`
	NextAttemptAt    int64  `json:"next_attempt_at,omitempty"`
	Failures         uint64 `json:"failures"`
}

type MaintenanceTasksResult struct {
	Tasks      []MaintenanceTask `json:"tasks"`
	ObservedAt int64             `json:"observed_at"`
}

type BacklogRequest struct {
	Cursor              string `json:"cursor,omitempty"`
	IncludeIngestionAge bool   `json:"include_ingestion_age,omitempty"`
}

type BacklogResult struct {
	PendingInputBytes         int64   `json:"pending_input_bytes"`
	PendingUnknownSizeBatches int     `json:"pending_unknown_size_batches"`
	ExpiredRecords            int64   `json:"expired_records"`
	ExpiredInputBytes         int64   `json:"expired_input_bytes"`
	ExpiredUnknownSizeBatches int     `json:"expired_unknown_size_batches"`
	IngestionAgeIncluded      bool    `json:"ingestion_age_included"`
	OldestPendingIngestedAt   string  `json:"oldest_pending_ingested_at,omitempty"`
	OldestPendingAgeSeconds   float64 `json:"oldest_pending_age_seconds"`
	AgeReadBytes              int64   `json:"age_read_bytes"`

	SnapshotID       string `json:"snapshot_id"`
	ObservedAt       int64  `json:"observed_at"`
	Batches          int    `json:"batches"`
	Records          int64  `json:"records"`
	PendingBatches   int    `json:"pending_batches"`
	PendingRecords   int64  `json:"pending_records"`
	InvalidSummaries int    `json:"invalid_summaries"`
	ExpiredBatches   int    `json:"expired_batches"`
	Next             string `json:"next,omitempty"`
	Complete         bool   `json:"complete"`
}

type AlertSilence struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
type AlertRoute struct {
	Name                string            `json:"name"`
	Match               map[string]string `json:"match,omitempty"`
	Webhook             string            `json:"webhook,omitempty"`
	WebhookSecret       string            `json:"webhook_secret,omitempty"`
	PreserveDestination bool              `json:"preserve_destination,omitempty"`
}
type AlertRouteInfo struct {
	Name       string            `json:"name"`
	Match      map[string]string `json:"match,omitempty"`
	Configured bool              `json:"configured"`
}
type AlertGroupState struct {
	Values       map[string]*string `json:"values"`
	Count        uint64             `json:"count"`
	PendingSince int64              `json:"pending_since,omitempty"`
	LastFiredAt  int64              `json:"last_fired_at,omitempty"`
	Firing       bool               `json:"firing"`
}
type AlertGroupEvent struct {
	Values map[string]*string `json:"values"`
	Count  uint64             `json:"count"`
}
type AlertRuleConfig struct {
	GroupBy         []string       `json:"group_by,omitempty"`
	Silences        []AlertSilence `json:"silences,omitempty"`
	Routes          []AlertRoute   `json:"routes,omitempty"`
	Severity        string         `json:"severity,omitempty"`
	ID              string         `json:"id,omitempty"`
	Name            string         `json:"name"`
	Query           string         `json:"query"`
	Enabled         bool           `json:"enabled"`
	WindowSeconds   int64          `json:"window_seconds"`
	EverySeconds    int64          `json:"every_seconds"`
	Threshold       uint64         `json:"threshold"`
	ForSeconds      int64          `json:"for_seconds,omitempty"`
	CooldownSeconds int64          `json:"cooldown_seconds,omitempty"`
	NotifyResolved  bool           `json:"notify_resolved,omitempty"`
	Webhook         string         `json:"webhook,omitempty"`
	WebhookSecret   string         `json:"webhook_secret,omitempty"`
	PreserveWebhook bool           `json:"preserve_webhook,omitempty"`
}

type AlertRuleInfo struct {
	GroupBy                []string          `json:"group_by,omitempty"`
	Groups                 []AlertGroupState `json:"groups,omitempty"`
	Silences               []AlertSilence    `json:"silences,omitempty"`
	SilencedUntil          int64             `json:"silenced_until,omitempty"`
	Routes                 []AlertRouteInfo  `json:"routes,omitempty"`
	Severity               string            `json:"severity"`
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Query                  string            `json:"query"`
	Enabled                bool              `json:"enabled"`
	WindowSeconds          int64             `json:"window_seconds"`
	EverySeconds           int64             `json:"every_seconds"`
	Threshold              uint64            `json:"threshold"`
	ForSeconds             int64             `json:"for_seconds"`
	CooldownSeconds        int64             `json:"cooldown_seconds"`
	NotifyResolved         bool              `json:"notify_resolved"`
	WebhookConfigured      bool              `json:"webhook_configured"`
	EvaluationPending      bool              `json:"evaluation_pending"`
	EvaluationDelaySeconds int64             `json:"evaluation_delay_seconds"`
	LastEvaluatedAt        int64             `json:"last_evaluated_at,omitempty"`
	LastCount              uint64            `json:"last_count"`
	Firing                 bool              `json:"firing"`
	LastError              string            `json:"last_error,omitempty"`
}

type AlertEvent struct {
	Groups        []AlertGroupEvent `json:"groups,omitempty"`
	Route         string            `json:"route,omitempty"`
	Query         *string           `json:"query,omitempty"`
	Severity      string            `json:"severity"`
	ID            string            `json:"id"`
	RuleID        string            `json:"rule_id"`
	LogbaseID     string            `json:"logbase_id"`
	Kind          string            `json:"kind"`
	Count         uint64            `json:"count"`
	WindowStart   int64             `json:"window_start"`
	WindowEnd     int64             `json:"window_end"`
	CreatedAt     int64             `json:"created_at"`
	Delivery      string            `json:"delivery"`
	Attempts      int               `json:"attempts"`
	NextAttemptAt int64             `json:"next_attempt_at,omitempty"`
	LastError     string            `json:"last_error,omitempty"`
}

type AlertRulesResult struct {
	Rules []AlertRuleInfo `json:"rules"`
	Next  string          `json:"next,omitempty"`
}

type AlertEventsResult struct {
	Events []AlertEvent `json:"events"`
	Next   string       `json:"next,omitempty"`
}

type ReceiptMaintenanceResult struct {
	Scanned    int    `json:"scanned"`
	Compressed int    `json:"compressed"`
	SavedBytes int64  `json:"saved_bytes"`
	Next       string `json:"next,omitempty"`
	Complete   bool   `json:"complete"`
}

type QueryLimits struct {
	CandidateRecords int   `json:"candidate_records"`
	ExaminedBatches  int   `json:"examined_batches"`
	SummaryBytes     int64 `json:"summary_bytes"`
	BodyWorkBytes    int64 `json:"body_work_bytes"`
	ResultBytes      int64 `json:"result_bytes"`
}

type QueryExplanation struct {
	Valid            bool        `json:"valid"`
	Start            string      `json:"start"`
	End              string      `json:"end"`
	Limit            int         `json:"limit"`
	FilterFields     []string    `json:"filter_fields"`
	ReadAllFields    bool        `json:"read_all_fields"`
	AccessPath       []string    `json:"access_path"`
	BodyVerification bool        `json:"body_verification"`
	Limits           QueryLimits `json:"limits"`
	Warnings         []string    `json:"warnings"`
	Suggestions      []string    `json:"suggestions"`
}

type QueryValidation struct {
	Valid bool `json:"valid"`
}
