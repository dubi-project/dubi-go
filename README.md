# dubi-go

The Go runtime and SDK for DUBI applications.

The framework uses a dynamic registration model:

```go
dubi.Register("user.get", dubi.Service{})
dubi.Register("worker", dubi.Profile{})
```

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
