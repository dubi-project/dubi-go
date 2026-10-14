# dubi-go

The Go runtime and SDK for DUBI applications.

The framework uses a dynamic registration model:

```go
dubi.Register("user.get", dubi.Service{})
dubi.Register("worker", dubi.Profile{})
```

Shared dynamic values live in [`base`](base): `base.Any`, `base.Map`,
`base.List`, and `base.Res`.

The Web entry module is documented in [`web`](web/README.md).
