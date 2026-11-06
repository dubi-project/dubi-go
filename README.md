# dubi-go

The Go runtime and SDK for DUBI applications.

The framework uses a dynamic registration model:

```go
dubi.Register("user.get", dubi.Service{})
dubi.Register("worker", dubi.Profile{})
```

Services keep their registration names for all four BUS calls and can also listen
to registered notifications:

```go
dubi.Register("user.created", dubi.Notification{
    Name: "User created",
    Args: Vars{"id": {Type: "string", Required: true}},
})
dubi.Register("mail.welcome", dubi.Service{
    Listen: "user.created",
    Group: "mail",
    Action: sendWelcome,
})
dubi.Register("account.initialize", dubi.Service{
    Listens: []string{"user.created"},
    Group: "account",
    Action: initializeAccount,
})
```

`Request`, `Broadcast`, `Dispatch`, and `Enqueue` address Service registration
names. `Notify("user.created", args)` validates the Notification's arguments and
sends to one listening Service per group. Listen/Listens are merged and deduplicated
and never act as aliases for the four service calls. `Subject` has been removed.

Notifications have no Action and live in a separate global catalog, outside
Registry entries and Profile selection. `dubi.Notifications()` returns detached
definitions for documentation. Definitions may be replaced even after startup;
`Reset` clears them. `Invoke` remains local-first with BUS Request fallback.

The built-in memory BUS implements all five calls on both `dubi` and `Meta`.
A mounted module implementing `BusHook` takes over delivery; Notify registration
and argument checks remain in the main package. See the [BUS documentation](docs/bus.md)
and [runnable example](examples/bus). The memory queue is not persistent.

Shared dynamic values live in [`base`](base): `base.Any`, `base.Map`,
`base.List`, and `base.Res`.

The process owns one registry and one lifecycle. Register components from
`init`, then start the selected profile with `dubi.Run`; it blocks until a
signal or `dubi.End` stops the process:

```go
func init() {
	dubi.Register("user.get", dubi.Method{Action: loadUser})
}

func main() {
	if err := dubi.Run("web"); err != nil {
		log.Fatal(err)
	}
}
```

`dubi.Setup`, `dubi.Open`, `dubi.Start`, `dubi.Stop`, and `dubi.Close` are
available when a test or embedded host needs individual lifecycle phases.

The Web entry module is documented in [`web`](web/README.md).

The built-in [`View module`](view/README.md) renders HTML templates, layouts,
and partials with `ctx.View`. It also exposes `view.Parse` for rendering outside
HTTP, with process-wide Helper registration and request-isolated template caching.

Process-wide resources use `dubi.AssetFS(fsys)`, accepting `embed.FS`,
`os.DirFS`, or any `fs.FS`. `AssetFS()` gets the current filesystem;
`AssetFile`, `AssetDir`, and `AssetStat` read its files and metadata.
Resources do not participate in Registry or Profile selection. See the
[embedded static-file example](examples/assets) and [Web documentation](web/README.md).

The Data module is included as a single package. Import it for its process
module registration, then select one of the built-in database types in the
configuration; there is no application-level type registry:

```go
import _ "github.com/dubi-project/dubi-go/data"
```

```yaml
data:
  default:
    type: sqlite # sqlite, mysql, pgsql, or mongodb
    url: file:app.db
```

For one connection, `data` may use `type`, `url`, and the other connection
options directly. For more than one connection, put named connection objects
under `data.connections` (or use named keys such as `default` and `analytics`).
Every connection must name its type explicitly; SQLite is not selected
implicitly when the type is missing.

The API keeps the concise infrago-style form:

```go
db := data.Base()
defer db.Close()
users := db.Table("users")
users.Insert(base.Map{"name": "alice"})
if err := db.Error(); err != nil {
    // Error is a stable read; the next database call clears it.
}
```

Authentication is built into the process runtime. `web.Router` can require a signed
and/or authenticated Token, and `Router.Kind` limits the identity type:

```go
dubi.Register("login", web.Router{
	Uri:  "/login",
	Kind: "user",
	Action: func(ctx *web.Context) {
		ctx.JSON(Map{"token": ctx.Auth(Map{"id": "u-1"})})
	},
})
dubi.Register("profile", web.Router{
	Uri:  "/profile",
	Kind: "user",
	Auth: true,
	Action: func(ctx *web.Context) {
		ctx.JSON(ctx.Payload())
	},
})
```

The local Auth module signs Tokens, checks expiry, and keeps a process-local
revocation table. A persistent `auth.secret` should be configured when Tokens
must survive process restarts:

```yaml
auth:
  secret: replace-with-a-stable-secret
  cookie: dubi_token
  ttl: 24h
```
