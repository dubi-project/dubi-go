# BUS

BUS calls address `Service.Subject`, which defaults to the registration name.
Different services can share a subject. Registry keys, Profile filters, and
local invocation still use their distinct registration names.

```go
dubi.Register("mail.welcome", dubi.Service{
    Subject: "user.created",
    Group: "mail",
    Retry: []time.Duration{5 * time.Second, 30 * time.Second},
    Action: sendWelcome,
})
dubi.Register("account.initialize", dubi.Service{
    Subject: "user.created",
    Group: "account",
    Action: initializeAccount,
})
```

## Calls

| Call | Recipients | Return value |
| --- | --- | --- |
| `Request` | One matching service | Business data/result |
| `Broadcast` | Every matching service | Submission error only |
| `Dispatch` | One matching service per group | Submission error only |
| `Enqueue` | One consumer, with configured retries | Admission error only |

```go
data, result := dubi.Request("user.get", Map{"id": "42"})
err := dubi.Broadcast("config.changed", Map{"version": 2})
err = dubi.Dispatch("user.created", Map{"id": "42"})
err = dubi.Enqueue("mail.send", Map{"to": "user@example.test"})
```

On a Meta, `ctx.Request` returns Map and stores the business result in
`ctx.Result()`. The other three return error and do not overwrite that result.
`Request` always uses BUS, including when a local Method has the same name.
`Invoke` remains local-first and falls back to Request only when neither a local
Method nor Service exists. `Execute` remains local Method only.

These calls become available after all modules finish Start. Use the process
START trigger for startup work that requires BUS. Calls before Start or after
Stop return `ErrBusNotReady`. Request cancellation/deadlines come from
`Meta.Context()`; handlers must cooperate with context cancellation.

## Groups

The effective Dispatch group is the first nonempty value:

```text
Service.Group -> Identity.Role -> Identity.Tenant -> "dubi"
```

Grouping is per subject. For independent operations such as mail and account
initialization, declare different groups. Multiple handlers in one group share
one notification. The memory BUS uses round robin within each group; Request
and Enqueue also select one recipient by round robin across all matches.
Broadcast ignores groups and executes every matching handler.

Without subscribers, Broadcast and Dispatch return nil and drop the notification;
they do not replay it when a subscriber appears. Request reports a failed Res,
and memory Enqueue returns `ErrBusNoService` when there is no consumer.

## Memory queue

The default implementation is process-local. It copies dynamic Map/List payloads
and request metadata before accepting asynchronous work. Each recipient and retry
gets its own copy. Payloads should contain data, not live objects such as pointers,
readers, HTTP contexts, or channels. Accepted work is detached from the caller's
request cancellation and tied to the BUS lifetime instead.

`Retry` lists the delay before each additional attempt. Two delays mean up to
three executions. Negative delays are rejected at startup; zero means immediate
retry. A nil/success result acknowledges the job; a failed Res or recovered panic
retries it. All failed results are eligible when retries are configured, so avoid
retries for handlers where repeated business failures are inappropriate.

In the memory implementation a retry invokes the originally selected registration
again. `ctx.Id()` remains the task ID across attempts, while `ctx.Attempt()` is
1-based. Other calls have Attempt zero. Business actions should use the task ID
for idempotency where duplicate execution would be harmful.

After retries are exhausted, the failure is logged and retained in `dubi.BusFailures()`.
The memory BUS retains the most recent `capacity` failures, including task ID,
subject, registration name, attempt count, original payload, and error text.

```toml
[bus.options]
workers = 4
capacity = 1024
```

Workers limit concurrent asynchronous execution; default is Go's GOMAXPROCS.
Capacity bounds all outstanding asynchronous deliveries, including running and
delayed jobs; default is 1024. Fan-out reserves capacity for all recipients or
returns `ErrBusFull` without partial delivery. Request executes synchronously
without acquiring a worker slot, so nested service requests do not deadlock a
single-worker queue. No ordering guarantee is made between independent jobs.

Stop rejects new work, cancels active contexts/retry timers and waits for callbacks
to return before module resources close. Pending/delayed work is discarded.
Handlers must observe cancellation and must not synchronously call End/Reset
from inside a BUS callback that shutdown would have to join.

**Memory tasks and failure records do not survive process exit.** This implementation
does not provide durable delivery or exactly-once execution. A server-backed or
external durable BUS is required for persisted tasks.

## External Hook

A module implements `dubi.Module` and `dubi.BusHook`, then mounts normally:

```go
var module = &Module{}
var host = dubi.Mount(module)
```

Mount automatically recognizes BusHook. Mounting the same instance again is a
no-op; a second external BUS implementation is a conflict. The memory BUS is
neither allocated nor started when an external hook is selected.

The hook implements Request, Broadcast, Dispatch, and Enqueue, each taking
`(*dubi.Meta, string, base.Map)`. Request returns `(base.Map, base.Res)`;
the other methods return error. Hook methods receive detached metadata/payload
snapshots, but their admission context ends on return: asynchronous implementations
must transfer accepted work to their own lifecycle context and snapshot anything
they retain. Keep task IDs stable across retries and manage acknowledgments and
retry storage inside the transport implementation.

During Setup, `host.Services()` returns the Profile-selected catalog with resolved
subjects/groups and retry delays. During delivery, use:

```go
data, result, found := host.InvokeLocalService(meta, registrationName, value)
```

This executes only that local Service; it never routes back into BUS and never
selects a same-named Method. For a decoded remote delivery, the optional last
argument `dubi.BusDelivery{ID: taskID, Attempt: attempt}` restores the ID and
1-based queue attempt on a copied Meta. Omit Attempt (zero) for non-queue calls.
Business code still cannot overwrite `ctx.Id()`.

Host deliveries are available after Start, recover Action panics into failed
results, and participate in BUS shutdown cancellation/joining. Delivery attempts
after Stop are rejected instead of executing against closed module resources.

Hook modules own connections, consumer start/stop,
and delivery shutdown through the normal module lifecycle. Their Stop must join
consumers before dependencies close. A hook may optionally expose
`Failures() []dubi.BusFailure` for `dubi.BusFailures()`.

Selection order is external Hook, configured Dubi Server, then built-in memory.
The Dubi Server transport is reserved but not implemented: configured endpoints
without an external hook produce `ErrBusServerUnavailable` during Setup rather
than silently using local delivery. External hooks take precedence over endpoints.

## Web

Web actions use the same four calls directly, including `ctx.Request(...)`.
Native HTTP objects are now `ctx.HTTP.Request` and `ctx.HTTP.Response`
(`http.ResponseWriter`). This replaces the previous top-level Request/Writer fields.
