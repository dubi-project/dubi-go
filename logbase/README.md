# Logbase HTTP client

This independent package uses the public Log API. It does not register or change
the existing dubi-go application framework. Only the Go standard library is used.

```go
client, err := logbase.New(logbase.Options{
    Endpoint: "https://api.example.com:7714",
    Token: token,
    HTTPClient: trustedHTTPClient,
    Gzip: true,
})
if err != nil { return err }
logs, err := client.Logbase("app")
if err != nil { return err }

batch, err := logs.NewBatch(stableRequestID, []json.RawMessage{
    json.RawMessage(`{"_msg":"request completed","status":200}`),
    json.RawMessage(`{"_msg":"slow request","duration_ms":9007199254740993}`),
})
if err != nil { return err }
result, err := batch.Send(ctx)
```

Import `github.com/dubi-project/dubi-go/logbase`. `Endpoint` is the API origin;
TLS certificate verification remains enabled. Supply an HTTP client trusting the
cluster API CA when the endpoint uses the cluster's private CA. Redirects are
rejected, including when a supplied HTTP client would normally follow them.

A Batch owns compact NDJSON bytes, never retains the caller's record buffers, and
preserves JSON number text. Optional gzip is applied once with BestSpeed and used
only if it reduces bytes. Automatic retries (default three attempts) reuse the
same body, encoding and `Dubi-Request-ID`, including after lost HTTP responses.
Retryable statuses are 429/503; retry delay grows from 250ms to 10s and respects
integer Retry-After seconds up to 10s. Other statuses return immediately. A lost,
malformed or incomplete success response never becomes a successful append.

The server validates all input before writing, but a legal request can span
multiple storage units and return a partial result or an uncertain outcome.
`*AppendError` exposes HTTP status, code, partial result, `MayHaveCommitted` and the
original `Batch`. Keep this object and retry `failure.Batch.Send(ctx)`; do not give
the remaining rows a new request ID. `transaction_aborted` is a terminal result
under that ID; a deliberate new operation requires a new ID. The caller controls
all retry lifetimes with context. `Append` is a convenient one-shot wrapper; the
explicit Batch API makes uncertain-outcome recovery clearer.

## Automatic batching

`logs.NewWriter(WriterOptions{})` creates a bounded in-memory writer. Defaults:
128 records or 256 KiB per batch, at most 5ms collection time, one active batch,
4096 pending records and 8 MiB of pending copied input, including in-flight work.
Each batch has its own random request identity, retained for every retry. The
server additionally combines small requests from independent clients.

```go
writer, err := logs.NewWriter(logbase.WriterOptions{})
if err != nil { return err }
defer writer.Close(ctx)
var receipts []*logbase.Receipt
for _, raw := range records {
    receipt, err := writer.Submit(ctx, raw)
    if err != nil { return err } // Handle ErrBackpressure before accepting more.
    receipts = append(receipts, receipt)
}
if err := writer.Close(ctx); err != nil { return err }
for _, receipt := range receipts {
    item, err := receipt.Wait(ctx)
    if err != nil { return err }
    _ = item.ID // This particular record has durable confirmation.
}
```

Submit means local queue acceptance only. Submit returns ErrBackpressure without
accepting the record when either bound is full. Wait means durable confirmation;
inspect every receipt even after Close succeeds. A committed item from a partial
response can complete its own receipt, while unresolved items expose the shared
AppendError and original whole Batch. Do not independently resubmit unresolved
records with new IDs. A cancelled Wait can be called again; it does not remove
accepted data from its shared batch.

Close stops admission and drains accepted records. If its context expires, the
worker continues bounded delivery and receipts remain usable; a later Close may
wait again. Each batch has a 45s deadline by default. There is no disk spool or
crash-surviving local queue: only successfully confirmed records are acknowledged.
Retaining completed receipts/errors retains their result and, on failure, the
retryable batch; applications should release them after resolving outcomes.

The server limit remains 1024 records and 1 MiB per request before and after gzip.
Writer batch limits are configurable within those limits. Wide fields and storage
operation budgets can impose tighter effective limits. Gzip and SDK batching do
not change durable confirmation, tenant authorization or retention semantics.

## Reads, paging and management

The client also provides typed query/field discovery, count aggregation, export,
live Tail, Logbase lifecycle/retention, append status, background jobs, maintenance,
backlog and alert-rule APIs. `Logbase` is the client handle; `LogbaseInfo` is the
server resource description. Query values remain strings exactly as stored.

```go
pages := logs.QueryPages(logbase.QueryRequest{
    Query: `service:="api" ERROR`,
    Limit: 100,
    Fields: []string{"_msg", "service"},
})
defer pages.Close(ctx) // Releases an unfinished read; inspect its error if needed.
for pages.Next(ctx) {
    for _, record := range pages.Page().Records {
        // Consume record.ID, record.Time and record.Fields.
        _ = record
    }
}
if err := pages.Err(); err != nil { return err }
```

The iterator freezes query/time/projection/page-size inputs and follows the same
protected cut. It retains one page. Use it sequentially; it is not concurrency
safe. `Fields: nil` means all fields; `Fields: []string{}` means system fields
only. Both representations are preserved on the wire and across continuation.
The single-page `Query`, `Export` and `Tail` methods expose the original cursors;
pass the exact original request with its next cursor. `CancelRead` takes mode
`query`, `export` or `tail`. Tail is interactive polling, not durable consumption.

Only append Batches automatically retry. Read/management calls make one attempt:
a lost response may have consumed a read cursor or committed a mutation.
`*APIError` exposes status, code, message, suggestions, request ID and
`MayHaveCommitted`. Retry a mutation explicitly with the same request ID and
identical parameters. A consumed/expired read needs an explicit restart; no
method silently replaces its snapshot. `AppendStatus` resolves the original
append identity, while `Batch.Send` remains the authoritative append retry path.
Redirects remain disabled. Request bodies are bounded to 1 MiB and API responses
to 20 MiB; an invalid or oversized response is an error.

`CreateExport`/`CreateAggregate` return an `ExportJobInfo`. Poll `JobStatus`;
only `completed` exposes `ExportPart` or `AggregateResult`. `ListJobs`,
`CancelJob` and `RetryJob` use the same durable server jobs. A failed job may not
be resumable if its snapshot expired. No client automatically creates another job.

`Backlog` is one metadata page. `ObserveBacklog(ctx, true)` sums up to 2048 pages
of one protected cut, including optional ingestion-age reads; it cancels an
unfinished cursor on failure. Inspect both error and `Complete` before treating
counts as totals. Input bytes are logical JSON bytes; unknown legacy sizes have
separate counts and are not treated as zero-sized data. The oldest pending age
uses ingestion time, not event time, so late arrivals are not reported as having
waited since their old event timestamp. This is index-work age, not a guarantee
about query availability: pending batches can still use body fallback.

`ValidateQuery` and `ExplainQuery` validate a fresh query without reading logs.
Explanation reports the resolved time window, predicate fields, conservative
block-skipping eligibility, budgets and warnings. It does not reserve a snapshot,
check Logbase existence, estimate actual cost, or guarantee execution success.
A query budget error carries suggestions such as `narrow_time_range`,
`add_selective_filter`, `use_create_aggregate` or
`use_create_export_for_bulk_read`. An export supplies records in immutable batch
order, not the same global event ordering as a latest-N query.

## Context, numeric metrics and grouped alerts

`Logbase.Context` takes an anchor ID and exact RFC3339Nano time, up to four exact
source fields, before/after counts (maximum 100 each), and an anchor-centered
window (maximum 3600 seconds). Results are nearest event-time/opaque-ID neighbors
at one protected cut; the bounded scan fails explicitly when too broad.

`AggregateRequest.Metrics` supports up to four sum/avg/min/max specifications with
unique aliases. `AggregateRow.Metrics` returns decimal string values (nil when
empty), valid/missing/invalid counts and Exact. Recurring averages are rounded to
twelve fractional places. Background aggregates use the same request and result
shape and persist exact accumulators; TopN still ranks by log count.

`AlertRuleConfig.GroupBy` enables independent per-group state (32 current groups
maximum). Silences are absolute start/end windows; evaluation and cooldown
continue while new notifications are suppressed, and pending delivery is deferred.
Routes match grouped fields in order; each round bundles matching groups by route
and event kind. PreserveDestination retains a named route's hidden credentials
on update. Group states and bundled event values are public, URLs/secrets are
not. Bundles publish incrementally from a durable bounded plan after evaluation.
Mixed-version operation with older experimental Log binaries is unsupported.
