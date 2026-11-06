# BUS

Services retain their registration names. Notifications are separately registered
contracts without Action or business logic:

```go
dubi.Register("user.created", dubi.Notification{
    Name: "User created",
    Desc: "Sent after successful user creation",
    Args: Vars{"id": {Type: "string", Required: true}},
})
dubi.Register("mail.welcome", dubi.Service{
    Listen: "user.created",
    Group: "mail",
    Retry: []time.Duration{5 * time.Second, 30 * time.Second},
    Action: sendWelcome,
})
dubi.Register("audit.record", dubi.Service{
    Listens: []string{"user.created", "user.deleted"},
    Group: "audit",
    Action: recordAudit,
})
```

`Listen` and `Listens` are merged, whitespace-trimmed, and deduplicated; empty
names are ignored. They are used only by Notify. The former Subject field has
been removed and has no compatibility alias.

## Calls

| Call | Address | Recipients | Return value |
| --- | --- | --- | --- |
| `Request` | Service registration name | One instance | Business data/result |
| `Broadcast` | Service registration name | Every instance | Submission error |
| `Dispatch` | Service registration name | One instance per group | Submission error |
| `Enqueue` | Service registration name | One consumer, with retries | Admission error |
| `Notify` | Notification registration name | One listening Service per group | Validation/submission error |

```go
data, result := dubi.Request("user.get", Map{"id": "42"})
err := dubi.Broadcast("config.reload", Map{"version": 2})
err = dubi.Dispatch("cache.refresh", Map{"id": "42"})
err = dubi.Enqueue("mail.welcome", Map{"id": "42"})
err = dubi.Notify("user.created", Map{"id": "42"})
```

On Meta, Request returns Map and stores the result in `ctx.Result()`. The other
calls return error without changing that result. Request always uses BUS, even
when a local Method has the same name. Invoke remains local-first with Request
fallback; Execute remains local Method only.

The four service calls never search notification listeners. A Service and a
Notification may have the same registration name and remain independent. A Service
does not automatically listen to the same-named Notification: declare Listen/Listens.
With the single-process Registry there is one Service per registration name;
Broadcast/Dispatch to that name therefore execute it once locally. Different
Services sharing a notification name coexist without replacing each other.

## Notification catalog and validation

`dubi.Register` sends Notification values/pointers directly to a global catalog.
They are not component entries, have no Scope, are not delivered to modules, and
never enter Registry Entries, Runtime Entries/AllEntries, or Profile filtering.
The compatibility `Runtime.Register`, `Scope.Register`, and `Registry.Register`
paths also route definitions into this same global catalog.

`dubi.Notifications()` returns a detached map keyed by registration name for
introspection and documentation. Same-name registration replaces the definition.
Registration remains allowed after Registry freeze; concurrent sends use an
immutable snapshot of the definition they looked up. Reset clears the catalog.

Notify requires a registered Notification in the sending process, even if there
are no listeners. Unknown names return `ErrNotificationNotFound`. Listen does not
implicitly register a Notification. You may declare listeners before the producer
contract is registered, but cannot send until that contract exists.

Before invoking any BUS Hook, Notify applies the Notification Args mapping,
including defaults, conversion and validation. A failed validation is returned
as an error (the underlying Res) and no delivery is submitted. If Args is empty,
the payload is passed through as a detached snapshot. On receipt, each Service
still applies its own Args mapping before executing Action.

Notify neither persists nor retries, even when a listener Service has Retry
configured. A registered notification without listeners succeeds and is dropped.
Broadcast/Dispatch without matching services likewise return nil. Request reports
a failed Res when missing, and memory Enqueue returns `ErrBusNoService`.

## Groups and lifecycle

The effective Dispatch/Notify group is the first nonempty value:

```text
Service.Group -> Identity.Role -> Identity.Tenant -> "dubi"
```

For independent notification effects such as mail and audit, declare different
groups. The memory implementation selects one listener per group by round robin;
it does not send multiple copies when Listen and Listens repeat a name. Only
Profile-selected Services participate. Notification definitions themselves are
always available, even if every component is excluded by the Profile.

Calls become available after all modules finish Start. Use the process START
trigger for startup work. Calls before Start or after Stop return ErrBusNotReady
(unknown Notify names still return ErrNotificationNotFound). Request deadlines
come from Meta.Context; handlers must cooperate with cancellation.

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
service registration name, attempt count, original payload, and error text.

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

The hook implements Request, Broadcast, Dispatch, Enqueue, and Notify, each taking
`(*dubi.Meta, string, base.Map)`. Request returns `(base.Map, base.Res)`;
the other methods return error. Hook methods receive detached metadata/payload
snapshots, but their admission context ends on return: asynchronous implementations
must transfer accepted work to their own lifecycle context and snapshot anything
they retain. Keep task IDs stable across retries and manage acknowledgments and
retry storage inside the transport implementation.

During Setup, `host.Services()` returns the Profile-selected catalog with resolved
Listens/groups and retry delays. Notification definitions are available through
`dubi.Notifications()`; the main package validates Notify before calling the hook. During delivery, use:

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

Web actions use all five calls directly, including `ctx.Request(...)`.
Native HTTP objects are now `ctx.HTTP.Request` and `ctx.HTTP.Response`
(`http.ResponseWriter`). This replaces the previous top-level Request/Writer fields.
