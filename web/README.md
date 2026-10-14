# dubi-go Web

`web` 是 Dubi 的标准 HTTP 请求入口模块。它负责监听 HTTP、匹配 Server/Site/Router、执行 Filter 和 Handler，并把请求参数转换成 `base.Map`。

## 注册路由

```go
dubi.Register("user.show", web.Router{
	Uri: "/users/{id}",
	Args: base.Vars{
		"id": {Type: "string", Required: true},
	},
	Action: func(ctx *web.Context) {
		ctx.Answer(dubi.OK, base.Map{"id": ctx.Args["id"]})
	},
})
```

Router 是 HTTP 入口逻辑。业务操作仍然通过 `ctx.Invoke` 或 `ctx.Execute` 调用 `Method`/`Service`。

## 配置

最简单的配置只有一个 HTTP Server：

```yaml
web:
  port: 8080
```

一个 Server 可以配置多个 Site：

```yaml
web:
  port: 8080
  sites:
    www:
      hosts:
        - www.example.com
    api:
      hosts:
        - api.example.com
```

需要多个监听端口时使用 `servers`：

```yaml
web:
  servers:
    web:
      port: 8080
      sites:
        www:
          hosts: [www.example.com]
        api:
          hosts: [api.example.com]
    internal:
      port: 8081
```

`setting`、`cross` 可以出现在 `web`、Server 和 Site 层级。下层配置覆盖上层同名键。

Server 还支持请求边界和连接配置：

```yaml
web:
  port: 8080
  request_timeout: 10s
  read_timeout: 5s
  read_header_timeout: 2s
  write_timeout: 30s
  idle_timeout: 60s
  shutdown_timeout: 5s
  max_header_bytes: 1048576
  max_body_bytes: 10485760
  max_concurrency: 256
  compress: true
  trusted_proxies: ["127.0.0.1", "10.0.0.0/8"]
  tls:
    cert: ./certs/server.crt
    key: ./certs/server.key
```

`request_timeout` 会绑定到业务调用使用的 Context。`read_header_timeout` 用于限制读取请求头的最长时间，防止慢连接长期占用连接资源。`max_body_bytes` 超限返回 413。TLS 需要同时配置证书和私钥；`trusted_proxies` 为空时不会信任请求头中的转发地址。
`max_concurrency` 限制单个 Server 同时处理的请求数，超限立即返回 503，并带有 `Retry-After: 1`。

## 路由归属

路由注册名使用前缀表达 Server/Site：

```go
dubi.Register("web.www.index", web.Router{Uri: "/"})
dubi.Register("api.user", web.Router{Uri: "/users/{id}"})
```

没有显式 Server/Site 时，默认 Server 接收该路由。注册名使用 `*.name` 时，会复制到当前 Server 的所有 Site。

`Routing` 用于按 HTTP 方法覆盖同一个路由的默认配置：

```go
web.Router{
	Uri: "/users/{id}",
	Routing: web.Routing{
		"GET":  {Action: getUser},
		"POST": {Action: createUser},
	},
}
```

## 参数、Loading 和响应

- `Args` 校验并转换路径、查询、表单和 JSON Body 参数。
- Query 和 Form 支持 `user.name`、`users[0].id`、`tags[]` 这样的嵌套键。
- Multipart 文件可通过 `ctx.Upload(name)` 或 `ctx.Uploads(name)` 读取。
- `Loading` 在路由 Action 前调用 Method/Service，并把结果放入 `ctx.Locals`。
- `Data` 在 `Answer` 输出前映射响应数据。
- `ctx.Answer(result, data)` 使用 Dubi 的 `Res` 生成统一 JSON 结果。

```go
web.Router{
	Uri: "/users/{id}",
	Loading: web.Loading{
		"user": {Invoke: "user.get", Required: true},
	},
	Action: func(ctx *web.Context) {
		ctx.Answer(dubi.OK, ctx.Locals["user"].(base.Map))
	},
}
```

HTTP 状态码和 Dubi 业务结果码是两套值。`NotFound`、`Error`、`Failed`、`Unsigned`、`Unauthed`、`Denied` 会设置对应的 HTTP 状态和默认 `Res`；传入自定义 `Res` 时优先使用业务结果。

## 当前请求能力

`web.Context` 继承 `dubi.Meta`，可以直接调用当前 Runtime 的能力：

```go
ctx.String("user.created")
ctx.Marshal("json", value)
ctx.Mapping(args, values)
ctx.Invoke("user.get", values)
ctx.ContentType()
ctx.Accepts("application/json")
ctx.ClientIP()
ctx.Elapsed()
ctx.CacheControl("public, max-age=60")
ctx.ETag("user-1-v2")
```

文件响应自动生成 ETag 并支持 `If-None-Match`。文本和 JSON 响应在配置开启压缩且请求声明支持 gzip 时自动压缩。Filter 的 `Response` 阶段可作为访问日志和指标的统一出口；可以通过 `ctx.Id()`、`ctx.StatusCode()` 和 `ctx.Elapsed()` 记录请求标识、响应状态和耗时。`ctx.Id()` 是当前请求的唯一 ID，并通过 `X-Request-ID` 返回给客户端。
