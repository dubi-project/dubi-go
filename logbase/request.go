package logbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIError preserves server guidance and the mutation identity for an explicit
// retry. Read cursors may be consumed by a lost response: they are never retried
// automatically, nor replaced with a fresh snapshot.
type APIError struct {
	Status           int
	Code             string
	Message          string
	Suggestions      []string
	RequestID        string
	MayHaveCommitted bool
	Cause            error
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Logbase API: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}
func (e *APIError) Unwrap() error { return e.Cause }

const maxAPIResponseBytes = 20 << 20

func apiCall[T any](ctx context.Context, c *Client, method, path, requestID string, input any, mutation bool) (out T, err error) {
	if ctx == nil || c == nil || (mutation && (requestID == "" || len(requestID) > 128 || strings.ContainsAny(requestID, "\r\n"))) {
		return out, ErrInvalid
	}
	var raw []byte
	if input != nil {
		raw, err = json.Marshal(input)
		if err != nil {
			return out, err
		}
		if len(raw) > MaxBytes {
			return out, ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.o.Endpoint+path, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+c.o.Token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutation {
		req.Header.Set("Dubi-Request-ID", requestID)
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	response, err := c.o.HTTPClient.Do(req)
	if err != nil {
		return out, &APIError{Code: "transport_error", RequestID: requestID, MayHaveCommitted: mutation, Cause: err}
	}
	defer response.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(response.Body, maxAPIResponseBytes+1))
	if err != nil || len(raw) > maxAPIResponseBytes {
		if err == nil {
			err = fmt.Errorf("response exceeds %d bytes", maxAPIResponseBytes)
		}
		return out, &APIError{Status: response.StatusCode, Code: "invalid_response", RequestID: requestID, MayHaveCommitted: mutation, Cause: err}
	}
	if response.StatusCode != http.StatusOK {
		e := &APIError{Status: response.StatusCode, Code: "http_error", RequestID: requestID, MayHaveCommitted: mutation && response.StatusCode >= 500}
		var failure struct {
			Code        string   `json:"code"`
			Message     string   `json:"message"`
			Suggestions []string `json:"suggestions"`
		}
		if json.Unmarshal(raw, &failure) == nil && failure.Code != "" {
			e.Code = failure.Code
			e.Message = failure.Message
			e.Suggestions = failure.Suggestions
		}
		return out, e
	}
	// Decode into a temporary result: malformed successes must not expose partial
	// rows as a valid answer. Unknown response fields remain forward compatible.
	var result T
	if err = json.Unmarshal(raw, &result); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if err == nil {
			err = fmt.Errorf("null API response")
		}
		return out, &APIError{Status: response.StatusCode, Code: "invalid_response", RequestID: requestID, MayHaveCommitted: mutation, Cause: err}
	}
	return result, nil
}
