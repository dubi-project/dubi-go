package dubi

import (
	"reflect"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func TestBusHookPublishesWithoutLocalRecipients(t *testing.T) {
	rt := testRuntime()
	hook := &testBusHook{}
	mountTestBus(t, hook)
	Register("user.created", Notification{Args: Vars{"id": {Type: "string", Required: true}}})
	Register("local", Service{Listen: "user.created", Action: func(*Context) {
		t.Error("host delivered a notification instead of delegating to hook")
	}})
	Register("sender", Profile{Exclude: []string{"core:service:*"}})
	startTestBus(t, rt, "sender")
	if len(hook.services) != 0 || bus.memory != nil {
		t.Fatal("sender unexpectedly has local recipients or memory BUS")
	}
	if err := Notify("user.created", Map{"id": "42"}); err != nil {
		t.Fatal(err)
	}
	if err := Dispatch("remote.service"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hook.calls, []string{"notify", "dispatch"}) ||
		hook.notificationName != "user.created" || hook.dispatchName != "remote.service" || hook.notified["id"] != "42" {
		t.Fatalf("host changed transport destinations: %#v", hook)
	}
}

func TestBusHookSubscriptionBindsServiceForTaskAttempts(t *testing.T) {
	rt := testRuntime()
	hook := &testBusHook{}
	mountTestBus(t, hook)
	calls := 0
	for _, name := range []string{"mail.a", "mail.b"} {
		Register(name, Service{Listen: "remote.created", Group: "mail", Retry: []time.Duration{0}, Action: func(ctx *Context) Res {
			calls++
			if ctx.Name != "mail.b" || ctx.Id() != "incoming-task" || ctx.Attempt() != calls || ctx.Value["id"] != "42" {
				t.Errorf("subscription binding changed: name=%s id=%s attempt=%d value=%v", ctx.Name, ctx.Id(), ctx.Attempt(), ctx.Value)
			}
			if calls == 1 {
				return Retry
			}
			return OK
		}})
	}
	Register("mail.b", Method{Action: func(*Context) { t.Error("delivery executed a Method") }})
	startTestBus(t, rt)
	if _, registered := Notifications()["remote.created"]; registered {
		t.Fatal("receiver unexpectedly has a producer notification contract")
	}
	// Model a broker invoking the callback bound to mail.b. The transport owns
	// this task, its retry policy, and its ID; Host executes only one attempt.
	var bound ServiceInfo
	for _, service := range hook.services {
		if service.Name == "mail.b" {
			bound = service
		}
	}
	if bound.Name == "" {
		t.Fatal("missing local subscription binding")
	}
	for attempt := 1; ; attempt++ {
		_, result, found := hook.host.InvokeLocalService(nil, bound.Name, Map{"id": "42"}, BusDelivery{ID: "incoming-task", Attempt: attempt})
		if !found || calls != attempt {
			t.Fatalf("Host did not execute exactly one bound attempt: found=%v calls=%d attempt=%d", found, calls, attempt)
		}
		if !IsRetry(result) || attempt > len(bound.Retry) {
			if result != nil && result.Fail() {
				t.Fatalf("task failed: %v", result)
			}
			break
		}
	}
	if calls != 2 || len(hook.calls) != 0 || bus.memory != nil {
		t.Fatalf("unexpected republish, routing or memory fallback: calls=%d outgoing=%v", calls, hook.calls)
	}
}
