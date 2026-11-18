package logbase

import (
	"context"
	"fmt"
	"time"
)

// QueryPages freezes the original request, including nil versus empty Fields.
// Iterate sequentially. Close early to release a live cursor; a failed Next
// never retries a possibly consumed cursor or changes the snapshot.
func (l *Logbase) QueryPages(q QueryRequest) *QueryIterator {
	if q.Fields != nil {
		q.Fields = append([]string{}, q.Fields...)
	}
	return &QueryIterator{base: l, request: q}
}

type QueryIterator struct {
	base    *Logbase
	request QueryRequest
	page    QueryResult
	err     error
	done    bool
}

func (p *QueryIterator) Next(ctx context.Context) bool {
	if p.done || p.err != nil {
		return false
	}
	result, err := p.base.Query(ctx, p.request)
	if err != nil {
		p.err = err
		return false
	}
	if (!result.Complete && result.Next == "") || (result.Complete && result.Next != "") || (result.Next != "" && result.Next == p.request.Cursor) {
		p.err = &APIError{Code: "invalid_response", Message: "query continuation did not advance"}
		return false
	}
	p.page = result
	p.request.Cursor = result.Next
	p.done = result.Complete
	return true
}
func (p *QueryIterator) Page() QueryResult { return p.page }
func (p *QueryIterator) Err() error        { return p.err }
func (p *QueryIterator) Close(ctx context.Context) error {
	p.done = true
	if p.request.Cursor == "" {
		return nil
	}
	if err := p.base.CancelRead(ctx, p.request, "query"); err != nil {
		return err
	}
	p.request.Cursor = ""
	return nil
}

// ObserveBacklog sums pages only within one protected metadata cut. It performs
// no polling and can be expensive for large histories. Counts/bytes refer to
// logical input, not physical storage. A completed observation is not an ingest
// completion watermark. Use a deadline and read Complete before using totals.
func (l *Logbase) ObserveBacklog(ctx context.Context, includeIngestionAge bool) (out BacklogResult, err error) {
	q := BacklogRequest{IncludeIngestionAge: includeIngestionAge}
	defer func() {
		if q.Cursor != "" {
			call, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_ = l.CancelBacklog(call, q)
		}
	}()
	if ctx == nil {
		return out, ErrInvalid
	}
	for pages := 0; pages < 2048; pages++ {
		page, e := l.Backlog(ctx, q)
		if e != nil {
			return out, e
		}
		if page.SnapshotID == "" || (out.SnapshotID != "" && (out.SnapshotID != page.SnapshotID || out.ObservedAt != page.ObservedAt)) || page.IngestionAgeIncluded != includeIngestionAge {
			return out, fmt.Errorf("backlog observation changed")
		}
		out.SnapshotID = page.SnapshotID
		out.ObservedAt = page.ObservedAt
		out.IngestionAgeIncluded = page.IngestionAgeIncluded
		out.Batches += page.Batches
		out.Records += page.Records
		out.PendingBatches += page.PendingBatches
		out.PendingRecords += page.PendingRecords
		out.PendingInputBytes += page.PendingInputBytes
		out.PendingUnknownSizeBatches += page.PendingUnknownSizeBatches
		out.ExpiredBatches += page.ExpiredBatches
		out.ExpiredRecords += page.ExpiredRecords
		out.ExpiredInputBytes += page.ExpiredInputBytes
		out.ExpiredUnknownSizeBatches += page.ExpiredUnknownSizeBatches
		out.InvalidSummaries += page.InvalidSummaries
		out.AgeReadBytes += page.AgeReadBytes
		if page.OldestPendingIngestedAt != "" {
			at, e := time.Parse(time.RFC3339Nano, page.OldestPendingIngestedAt)
			if e != nil {
				return out, e
			}
			old, e := time.Parse(time.RFC3339Nano, out.OldestPendingIngestedAt)
			if e != nil || at.Before(old) {
				out.OldestPendingIngestedAt = page.OldestPendingIngestedAt
			}
		}
		out.OldestPendingAgeSeconds = max(out.OldestPendingAgeSeconds, page.OldestPendingAgeSeconds)
		if page.Complete {
			if page.Next != "" {
				return out, fmt.Errorf("terminal backlog returned continuation")
			}
			out.Complete = true
			q.Cursor = ""
			return out, nil
		}
		if page.Next == "" || page.Next == q.Cursor {
			return out, fmt.Errorf("backlog continuation did not advance")
		}
		q.Cursor = page.Next
	}
	return out, fmt.Errorf("backlog observation exceeds 2048 pages")
}
