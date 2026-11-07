package dubi

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/dubi-project/dubi-go/base"
)

func TestNotifyRetriesSelectedServicesWithoutRepublishing(t *testing.T) {
	rt := testRuntime()
	Register("created", Notification{Args: Vars{"id": {Type: "int", Required: true}}})
	type attempt struct {
		name, id string
		number   int
		value    string
	}
	seen := make(chan attempt, 20)
	gate := make(chan struct{})
	for name, group := range map[string]string{
		"mail.a": "mail", "mail.b": "mail", "account": "account", "invalid": "invalid", "exhausted": "exhausted",
	} {
		Register(name, Service{Listen: "created", Group: group, Retry: []time.Duration{time.Millisecond, time.Millisecond}, Action: func(ctx *Context) Res {
			<-gate
			seen <- attempt{ctx.Name, ctx.Id(), ctx.Attempt(), fmt.Sprint(ctx.Value["id"])}
			ctx.Value["id"] = "mutated by service"
			switch ctx.Name {
			case "mail.a":
				if ctx.Attempt() < 3 {
					return Retry
				}
				return OK
			case "invalid":
				return Fail
			case "exhausted":
				return Retry
			default:
				return OK
			}
		}})
	}
	startTestBus(t, rt)
	ctx, cancel := context.WithCancel(context.Background())
	meta := NewMeta().WithContext(ctx)
	value := Map{"id": "42"}
	if err := meta.Notify("created", value); err != nil {
		t.Fatal(err)
	}
	cancel()
	meta.Reset(context.Background())
	value["id"] = "caller mutation"
	// Already accepted tasks must not revalidate or republish the Notification.
	Register("created", Notification{Args: Vars{"new.required.field": {Required: true}}})
	close(gate)
	waitMemoryJobs()
	counts := map[string]int{}
	ids := map[string]string{}
	for len(seen) > 0 {
		item := <-seen
		counts[item.name]++
		if previous, exists := ids[item.name]; exists && previous != item.id {
			t.Fatalf("task ID changed: %#v", item)
		}
		ids[item.name] = item.id
		if item.id == "" || item.number != counts[item.name] || item.value != "42" {
			t.Fatalf("task changed across retries: %#v", item)
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"mail.a": 3, "account": 1, "invalid": 1, "exhausted": 3}) {
		t.Fatalf("unexpected replay/reselection: %v", counts)
	}
	unique := map[string]bool{}
	for _, id := range ids {
		unique[id] = true
	}
	if len(unique) != len(ids) {
		t.Fatal("different group tasks shared an ID")
	}
	failures := BusFailures()
	if len(failures) != 2 {
		t.Fatalf("terminal failures = %#v", failures)
	}
	for _, failure := range failures {
		if failure.Notification != "created" || failure.ID != ids[failure.Service] || failure.Attempts != counts[failure.Service] || fmt.Sprint(failure.Value["id"]) != "42" {
			t.Fatalf("failure provenance changed: %#v", failure)
		}
	}
}

func TestNotifyShutdownCancelsDelayedTasks(t *testing.T) {
	rt := testRuntime()
	Register("shutdown", Notification{})
	seen := make(chan int, 2)
	Register("worker", Service{Listen: "shutdown", Retry: []time.Duration{time.Hour}, Action: func(ctx *Context) Res {
		seen <- ctx.Attempt()
		return Retry
	}})
	startTestBus(t, rt)
	if err := Notify("shutdown"); err != nil {
		t.Fatal(err)
	}
	if attempt := receiveBus(t, seen); attempt != 1 {
		t.Fatalf("first attempt = %d", attempt)
	}
	done := make(chan error, 1)
	go func() { done <- rt.End() }()
	if err := receiveBus(t, done); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Fatal("Notify retry survived shutdown")
	}
}

func TestNotificationCatalogBypassesRegistryAndProfile(t *testing.T) {
	rt := testRuntime()
	definition := &Notification{Name: "Created", Desc: "A new user", Args: Vars{
		"id":       {Type: "int", Required: true},
		"settings": {Type: "object", Default: Map{"value": "original"}},
	}}
	Scope("user").Register("created", definition)
	NewRegistry().Register("independent", Notification{})
	rt.Register("none", Profile{Exclude: []string{"*"}})
	startTestBus(t, rt, "none")
	if len(rt.Entries()) != 0 || len(rt.AllEntries()) != 0 {
		t.Fatal("Notification leaked into runtime entries")
	}
	if _, exists := Lookup("user.created"); exists {
		t.Fatal("Notification stored in Registry")
	}
	if len(testReceiverModule.registered) != 0 {
		t.Fatal("Notification delivered to component modules")
	}
	if len(Notifications()) != 2 {
		t.Fatalf("notification catalog = %#v", Notifications())
	}
	definition.Args["id"] = Var{Type: "string"}
	catalog := Notifications()
	if catalog["user.created"].Args["id"].Type != "int" {
		t.Fatal("registration retained caller's Vars")
	}
	catalog["user.created"].Args["settings"].Default.(Map)["value"] = "mutated"
	delete(catalog, "independent")
	if Notifications()["user.created"].Args["settings"].Default.(Map)["value"] != "original" || len(Notifications()) != 2 {
		t.Fatal("catalog exposed mutable definitions")
	}
	// Registration and replacement remain legal after Registry is frozen.
	Register("user.created", Notification{Name: "Replacement"})
	Register("late", Notification{})
	if err := Notify("user.created"); err != nil {
		t.Fatal(err)
	}
	if Notifications()["user.created"].Name != "Replacement" {
		t.Fatal("definition was not replaced")
	}
	if err := Notify("late"); err != nil {
		t.Fatal(err)
	}
	Reset()
	if len(Notifications()) != 0 {
		t.Fatal("Reset retained notifications")
	}
}

func TestNotifyGroupsListenersAndValidatesBeforeDelivery(t *testing.T) {
	rt := testRuntime(WithRole("worker"))
	Register("created", Notification{Args: Vars{
		"id":     {Type: "int", Required: true},
		"source": {Type: "string", Default: "system"},
	}})
	type observation struct {
		name  string
		value Map
		args  Map
	}
	seen := make(chan observation, 16)
	for name, group := range map[string]string{"mail.a": "mail", "mail.b": "mail", "audit": "audit", "excluded": "excluded"} {
		Register(name, Service{Listen: " created ", Listens: []string{"", "created", "other", "other"}, Group: group,
			Retry: []time.Duration{0, 0},
			Args:  Vars{"id": {Type: "string", Required: true}},
			Action: func(ctx *Context) Res {
				seen <- observation{ctx.Name, ctx.Value.Clone(), ctx.Args.Clone()}
				ctx.Value["source"] = "changed"
				return Fail // An ordinary failure is terminal even with Retry delays.
			},
		})
	}
	Register("selected", Profile{Include: []string{"core:service:mail.*", "core:service:audit"}})
	startTestBus(t, rt, "selected")
	if !reflect.DeepEqual(core.services["mail.a"].Listens, []string{"created", "other"}) {
		t.Fatalf("merged listeners = %v", core.services["mail.a"].Listens)
	}
	meta := NewMeta()
	meta.Result(Fail)
	if err := meta.Notify("created"); err == nil {
		t.Fatal("missing required argument accepted")
	}
	for _, name := range []string{"unknown", "", "other"} {
		if err := Notify(name); !errors.Is(err, ErrNotificationNotFound) {
			t.Fatalf("unregistered %q: %v", name, err)
		}
	}
	if len(seen) != 0 {
		t.Fatal("invalid notification was delivered")
	}
	for i := 0; i < 2; i++ {
		if err := meta.Notify("created", Map{"id": "42"}); err != nil {
			t.Fatal(err)
		}
	}
	waitMemoryJobs()
	counts := map[string]int{}
	for len(seen) > 0 {
		item := <-seen
		counts[item.name]++
		if fmt.Sprint(item.value["id"]) != "42" || item.value["source"] != "system" || item.args["id"] != "42" {
			t.Fatalf("mapping stages = %#v", item)
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"mail.a": 1, "mail.b": 1, "audit": 2}) {
		t.Fatalf("notify recipients = %v", counts)
	}
	if meta.Result() != Fail {
		t.Fatal("Notify overwrote caller's business result")
	}
	if err := Broadcast("created"); err != nil {
		t.Fatal(err)
	}
	if err := Dispatch("created"); err != nil {
		t.Fatal(err)
	}
	waitMemoryJobs()
	if len(seen) != 0 {
		t.Fatal("service Broadcast/Dispatch matched notification listeners")
	}
	if _, result := Request("created"); result == nil || result.OK() {
		t.Fatal("Request matched notification listeners")
	}
	if err := Enqueue("created"); !errors.Is(err, ErrBusNoService) {
		t.Fatalf("Enqueue matched listeners: %v", err)
	}
}

func TestNotificationAndServiceSameNameRemainIndependent(t *testing.T) {
	rt := testRuntime()
	Register("same", Notification{})
	seen := make(chan string, 8)
	for _, name := range []string{"same", "listener"} {
		definition := Service{Action: func(ctx *Context) Map { seen <- ctx.Name; return Map{"name": ctx.Name} }}
		if name == "listener" {
			definition.Listen = "same"
		}
		Register(name, definition)
	}
	startTestBus(t, rt)
	if data, _ := Request("same"); data["name"] != "same" {
		t.Fatalf("service Request = %v", data)
	}
	for _, call := range []func(string, ...Map) error{Broadcast, Dispatch, Enqueue, Notify} {
		if err := call("same"); err != nil {
			t.Fatal(err)
		}
	}
	waitMemoryJobs()
	counts := map[string]int{}
	for len(seen) > 0 {
		counts[<-seen]++
	}
	if !reflect.DeepEqual(counts, map[string]int{"same": 4, "listener": 1}) {
		t.Fatalf("independent addressing = %v", counts)
	}
}

func TestNotifyHookReceivesOnlyValidatedPayload(t *testing.T) {
	rt := testRuntime()
	hook := &testBusHook{}
	mountTestBus(t, hook)
	Register("created", Notification{Args: Vars{
		"id":       {Type: "int", Required: true},
		"settings": {Type: "map", Default: Map{"value": "original"}},
	}})
	Register("handler", Service{Listen: "created", Listens: []string{"created", "other"}})
	startTestBus(t, rt)
	if err := Notify("unknown"); !errors.Is(err, ErrNotificationNotFound) {
		t.Fatal(err)
	}
	if err := Notify("created"); err == nil {
		t.Fatal("invalid args reached hook")
	}
	if len(hook.calls) != 0 {
		t.Fatalf("hook received invalid notification: %v", hook.calls)
	}
	if err := Notify("created", Map{"id": "7"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hook.calls, []string{"notify"}) || fmt.Sprint(hook.notified["id"]) != "7" {
		t.Fatalf("hook calls/payload = %v, %v", hook.calls, hook.notified)
	}
	hook.notified["settings"].(Map)["value"] = "changed by hook"
	if err := Notify("created", Map{"id": "8"}); err != nil {
		t.Fatal(err)
	}
	if hook.notified["settings"].(Map)["value"] != "original" {
		t.Fatal("Hook mutated the notification default")
	}
	if !reflect.DeepEqual(hook.services[0].Listens, []string{"created", "other"}) {
		t.Fatalf("hook listeners = %#v", hook.services)
	}
	hook.services[0].Listens[0] = "changed"
	if hook.host.Services()[0].Listens[0] != "created" {
		t.Fatal("hook catalog exposed listener slice")
	}
}

func TestNotificationConcurrentReplacementAndNotify(t *testing.T) {
	rt := testRuntime()
	Register("notice", Notification{})
	startTestBus(t, rt)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				Register("notice", Notification{Args: Vars{"value": {Type: "int", Default: j}}})
				if err := Notify("notice"); err != nil {
					t.Error(err)
				}
				_ = Notifications()
			}
		}()
	}
	wg.Wait()
}
