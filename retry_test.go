package dubi

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func TestRetryPreservesBusinessResult(t *testing.T) {
	original := Result(701, "retry.test.unavailable", "%s暂时不可用").With("邮件服务")
	marked := original.Retry()
	if !IsRetry(marked) || IsRetry(original) {
		t.Fatal("Retry mutated the original result or failed to mark its copy")
	}
	if marked.Code() != original.Code() || marked.Status() != original.Status() || marked.Error() != original.Error() || !reflect.DeepEqual(marked.Args(), original.Args()) {
		t.Fatalf("business result changed: %v", marked)
	}
	if got := marked.With("短信服务"); !IsRetry(got) || got.Error() != "短信服务暂时不可用" {
		t.Fatalf("With lost retry or text: %v", got)
	}
	if IsRetry(marked.Retry(false)) || !IsRetry(marked) {
		t.Fatal("Retry(false) did not return an unmarked copy")
	}
	if !IsRetry(Retry) || !IsRetry(Retry.With("detail")) || IsRetry(Retry.Retry(false)) {
		t.Fatal("standard Retry result markers are inconsistent")
	}
	if IsRetry(nil) || IsRetry(OK.Retry()) || IsRetry(Fail) || IsRetry(ResultOf(2, "retry")) {
		t.Fatal("unmarked or successful result is retryable")
	}
	if RetryResult(nil) != nil || RetryResult(OK) != OK {
		t.Fatal("RetryResult converted success into failure")
	}
	if got := RetryResult(original, "存储服务"); !IsRetry(got) || got.Code() != 701 || got.Error() != "存储服务暂时不可用" {
		t.Fatalf("RetryResult = %v", got)
	}
	if got := ErrorResult(fmt.Errorf("transport: %w", marked)); !IsRetry(got) || got.Code() != 701 || got.Error() != marked.Error() {
		t.Fatalf("error conversion lost business result: %v", got)
	}
	if IsRetry(ErrorResult(errors.New("plain error"))) {
		t.Fatal("plain Go error became retryable")
	}
}

func TestQueueRetriesOnlyMarkedFailures(t *testing.T) {
	rt := testRuntime()
	Register("retry.notice", Notification{})
	business := Result(702, "retry.test.business", "服务繁忙")
	type observation struct {
		name    string
		attempt int
	}
	seen := make(chan observation, 32)
	for _, name := range []string{"ordinary", "marked", "switch", "success", "error", "raw.error", "notification"} {
		service := Service{Retry: []time.Duration{0, 0}, Action: func(ctx *Context) Res {
			seen <- observation{ctx.Name, ctx.Attempt()}
			switch ctx.Name {
			case "ordinary":
				return business
			case "switch":
				if ctx.Attempt() == 1 {
					return business.Retry()
				}
				return business
			case "success":
				return OK.Retry()
			default:
				return business.Retry()
			}
		}}
		if name == "error" || name == "raw.error" {
			service.Action = func(ctx *Context) error {
				seen <- observation{ctx.Name, ctx.Attempt()}
				if ctx.Name == "raw.error" {
					return errors.New("ordinary Go error")
				}
				return fmt.Errorf("wrapped: %w", business.Retry())
			}
		}
		if name == "notification" {
			service.Listen = "retry.notice"
		}
		Register(name, service)
	}
	startTestBus(t, rt)
	for _, name := range []string{"ordinary", "marked", "switch", "success", "error", "raw.error"} {
		if err := Enqueue(name); err != nil {
			t.Fatal(err)
		}
	}
	for _, call := range []func(string, ...Map) error{Broadcast, Dispatch} {
		if err := call("notification"); err != nil {
			t.Fatal(err)
		}
	}
	if err := Notify("retry.notice"); err != nil {
		t.Fatal(err)
	}
	waitMemoryJobs()
	counts := map[string]int{}
	notificationAttempts := map[int]int{}
	for len(seen) > 0 {
		item := <-seen
		counts[item.name]++
		if item.name == "notification" {
			notificationAttempts[item.attempt]++
		}
	}
	want := map[string]int{"ordinary": 1, "marked": 3, "switch": 2, "success": 1, "error": 3, "raw.error": 1, "notification": 5}
	if !reflect.DeepEqual(notificationAttempts, map[int]int{0: 2, 1: 1, 2: 1, 3: 1}) {
		t.Fatalf("notification/task attempts = %v", notificationAttempts)
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("executions = %v, want %v", counts, want)
	}
	failures := BusFailures()
	if len(failures) != 6 {
		t.Fatalf("failures = %#v", failures)
	}
	for _, failure := range failures {
		attempts := want[failure.Service]
		if failure.Service == "notification" {
			attempts = 3
			if failure.Notification != "retry.notice" {
				t.Fatalf("missing notification origin: %#v", failure)
			}
		} else if failure.Notification != "" {
			t.Fatalf("unexpected notification origin: %#v", failure)
		}
		if failure.Attempts != attempts {
			t.Fatalf("failure attempts = %#v", failure)
		}
		if failure.Service != "raw.error" && failure.Error != business.Error() {
			t.Fatalf("failure lost business text: %#v", failure)
		}
	}
	// The same action reports its original business result on synchronous calls;
	// the flag does not trigger retries or replace the error code/text.
	for _, call := range []func(string, ...Map) (Map, Res){Request, Invoke} {
		_, result := call("marked")
		if !IsRetry(result) || result.Code() != 702 || result.Error() != business.Error() {
			t.Fatalf("synchronous result = %v", result)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("synchronous call automatically retried: %d executions", len(seen))
	}
}
