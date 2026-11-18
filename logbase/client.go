// Package logbase provides a bounded HTTP client for DUBI's immutable logs.
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
	"net/url"
	"strconv"
	"strings"
	"time"
)

const MaxRecords = 1024
const MaxBytes = 1 << 20

var ErrInvalid = errors.New("invalid Logbase client request")

type Options struct {
	Endpoint    string
	Token       string
	HTTPClient  *http.Client
	Gzip        bool
	MaxAttempts int           // default 3; retries always preserve the batch and request ID
	RetryDelay  time.Duration // default 250ms, exponential up to 10 seconds
}

type Client struct{ o Options }
type Logbase struct {
	c    *Client
	name string
}

func New(o Options) (*Client, error) {
	u, e := url.Parse(o.Endpoint)
	if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(o.Token, "\r\n") || o.Token == "" {
		return nil, ErrInvalid
	}
	o.Endpoint = strings.TrimRight(o.Endpoint, "/")
	if o.MaxAttempts == 0 {
		o.MaxAttempts = 3
	}
	if o.RetryDelay == 0 {
		o.RetryDelay = 250 * time.Millisecond
	}
	if o.MaxAttempts < 1 || o.MaxAttempts > 10 || o.RetryDelay < 0 || o.RetryDelay > 10*time.Second {
		return nil, ErrInvalid
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 45 * time.Second}
	}
	// Never forward credentials or a mutating request through a redirect.
	h := *o.HTTPClient
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	o.HTTPClient = &h
	return &Client{o}, nil
}

func (c *Client) Logbase(name string) (*Logbase, error) {
	if len(name) == 0 || len(name) > 128 || strings.ContainsAny(name, "/\\?#%\r\n") || name == "." || name == ".." {
		return nil, ErrInvalid
	}
	return &Logbase{c: c, name: name}, nil
}

type AppendItem struct {
	Index  int    `json:"index"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}
type AppendResult struct {
	LogbaseID string       `json:"logbase_id"`
	Items     []AppendItem `json:"items"`
	Complete  bool         `json:"complete"`
}

// Batch owns immutable wire bytes. Keep it when the outcome is uncertain, then
// call Send again instead of allocating a new identity for the same records.
type Batch struct {
	base  *Logbase
	id    string
	body  []byte
	count int
	gzip  bool
}

func (b *Batch) RequestID() string { return b.id }

func (l *Logbase) NewBatch(id string, records []json.RawMessage) (*Batch, error) {
	if id == "" || len(id) > 128 || strings.ContainsAny(id, "\r\n") || len(records) == 0 || len(records) > MaxRecords {
		return nil, ErrInvalid
	}
	var out bytes.Buffer
	for _, raw := range records {
		if len(raw) > MaxBytes {
			return nil, ErrInvalid
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) {
			return nil, ErrInvalid
		}
		// Compact also removes embedded newlines, preserving JSON number text
		// and duplicate keys for the server's authoritative validation.
		if err := json.Compact(&out, raw); err != nil {
			return nil, ErrInvalid
		}
		out.WriteByte('\n')
		if out.Len() > MaxBytes {
			return nil, ErrInvalid
		}
	}
	body := out.Bytes()
	compressed := false
	if l.c.o.Gzip {
		var dst bytes.Buffer
		gz, _ := gzip.NewWriterLevel(&dst, gzip.BestSpeed)
		_, _ = gz.Write(body)
		_ = gz.Close()
		if dst.Len() < len(body) {
			body = dst.Bytes()
			compressed = true
		}
	}
	return &Batch{base: l, id: id, body: body, count: len(records), gzip: compressed}, nil
}

type AppendError struct {
	Batch            *Batch
	Status           int
	Code             string
	Result           AppendResult
	MayHaveCommitted bool
	Cause            error
}

func (e *AppendError) Error() string {
	return fmt.Sprintf("Logbase append %s: HTTP %d %s (outcome may be uncertain: %t)", e.Batch.id, e.Status, e.Code, e.MayHaveCommitted)
}
func (e *AppendError) Unwrap() error { return e.Cause }

func (l *Logbase) Append(ctx context.Context, id string, records []json.RawMessage) (AppendResult, error) {
	b, e := l.NewBatch(id, records)
	if e != nil {
		return AppendResult{}, e
	}
	return b.Send(ctx)
}

func validItems(r AppendResult, count int, complete bool) bool {
	if len(r.Items) > count || (complete && (!r.Complete || len(r.Items) != count || r.LogbaseID == "")) {
		return false
	}
	seen := map[int]bool{}
	ids := map[string]bool{}
	for _, i := range r.Items {
		if i.Index < 0 || i.Index >= count || seen[i.Index] {
			return false
		}
		seen[i.Index] = true
		if i.Status == "committed" {
			if i.ID == "" || ids[i.ID] {
				return false
			}
			ids[i.ID] = true
		} else if complete {
			return false
		}
	}
	return true
}

func (b *Batch) Send(ctx context.Context) (AppendResult, error) {
	o := b.base.c.o
	last := &AppendError{Batch: b}
	uncertain := false
	for attempt := 0; attempt < o.MaxAttempts; attempt++ {
		if e := ctx.Err(); e != nil {
			last.Cause = e
			last.MayHaveCommitted = uncertain
			return last.Result, last
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, o.Endpoint+"/v1/log/logbases/"+url.PathEscape(b.base.name)+"/append", bytes.NewReader(b.body))
		if e != nil {
			return last.Result, e
		}
		req.Header.Set("Authorization", "Bearer "+o.Token)
		req.Header.Set("Dubi-Request-ID", b.id)
		req.Header.Set("Content-Type", "application/x-ndjson")
		if b.gzip {
			req.Header.Set("Content-Encoding", "gzip")
		}
		response, e := o.HTTPClient.Do(req)
		delay := min(o.RetryDelay*time.Duration(1<<attempt), 10*time.Second)
		retry := true
		last.Status = 0
		last.Code = "transport_error"
		last.Cause = e
		if e != nil {
			uncertain = true
		} else {
			last.Status = response.StatusCode
			raw, readErr := io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
			response.Body.Close()
			if readErr != nil || len(raw) > MaxBytes {
				uncertain = true
				last.Code = "invalid_response"
				last.Cause = readErr
			} else if response.StatusCode == 200 {
				var result AppendResult
				if json.Unmarshal(raw, &result) == nil && validItems(result, b.count, true) {
					return result, nil
				}
				uncertain = true
				last.Code = "invalid_response"
			} else {
				var failure struct {
					Code   string       `json:"code"`
					Result AppendResult `json:"append_result"`
				}
				if json.Unmarshal(raw, &failure) != nil {
					failure.Code = "invalid_response"
				}
				last.Code = failure.Code
				if len(failure.Result.Items) > 0 && validItems(failure.Result, b.count, false) {
					last.Result = failure.Result
				}
				retry = response.StatusCode == 429 || response.StatusCode == 503
				if response.StatusCode >= 500 || len(last.Result.Items) > 0 {
					uncertain = true
				}
				if seconds, e := strconv.Atoi(response.Header.Get("Retry-After")); e == nil && seconds >= 0 {
					delay = max(delay, min(time.Duration(seconds), 10)*time.Second)
				}
			}
		}
		last.MayHaveCommitted = uncertain
		if !retry || attempt+1 == o.MaxAttempts {
			return last.Result, last
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			last.Cause = ctx.Err()
			return last.Result, last
		case <-timer.C:
		}
	}
	return last.Result, last
}
