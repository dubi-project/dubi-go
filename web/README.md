# dubi-go Web

`web` 是 Dubi 的标准 HTTP 请求入口模块。它负责监听 HTTP、匹配 Server/Site/Router、执行 Filter 和 Handler，并把请求参数转换成 `base.Map`。

启动成功后，每个 Web Server 会通过 Go 标准 logger 输出实际监听地址和 Site 数量。
所有模块启动完成后，Runtime 输出一条 `dubi: ready` 摘要，包含模块数、选中注册数、
启动耗时及节点身份；启动失败不会输出就绪摘要。可以用 `log.SetOutput` 统一重定向日志。

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

### 模板页面

```go
ctx.Data["user"] = user
ctx.View("order/detail", Map{"order": order})
```

`ctx.View` 使用内置 [View 模块](../view/README.md)，默认读取 `asset/views`，
支持布局、局部模板、多 Site、多语言、Helper 和 AssetFS。
Response Filter 修改完数据后再渲染，渲染错误进入 Error Handler。
参见 [`examples/view`](../examples/view) 中可直接运行的内嵌页面示例。

### 静态文件与内嵌资源

`static` 指定静态文件根目录，可在 Web、Server、Site 层级设置；Site 未设置时继承 Server。
配置静态文件即可启动 Web，无需额外注册 Router。

```yaml
web:
  port: 8080
  static: public
```

默认读取磁盘文件。要把资源编进程序，在业务项目中添加 `assets.go`：

```go
package main

import (
    "embed"

    dubi "github.com/dubi-project/dubi-go"
)

//go:embed public
var assets embed.FS

func init() {
    dubi.AssetFS(assets)
}
```

Web 根据 `static` 路径优先读取 AssetFS，未命中时回退到磁盘。`/` 对应根目录的
`index.html`；已匹配的路由优先于静态文件。内嵌文件支持 MIME、HEAD、Range、ETag
和条件请求，并沿用静态响应的 Filter、Handler 和 `settings.cache_control`。
`ctx.File` 继续读取磁盘文件。

`dubi.AssetFS` 是进程级资源，不受 Profile 影响。`AssetFile`、`AssetDir`、`AssetStat`
也可供业务代码直接使用；参数使用相对资源路径和 `/` 分隔符。未设置文件系统时返回错误。
使用 `fs.Sub(assets, "public")` 去掉目录前缀时，应同时把 `static` 配为 `"."`。

完整示例位于 [`examples/assets`](../examples/assets)。从该目录执行 `go run .`，
访问 `http://localhost:18781/`。也可编译后通过命令行参数指定静态路径，无需携带配置文件：

```sh
go build -o /tmp/dubi-assets ./examples/assets
/tmp/dubi-assets --config.web.port=18781 --config.web.static=public
```

### 监听与域名

最简单的配置只有一个 HTTP Server：

```yaml
web:
  port: 8080
```

`host` 表示监听绑定地址，默认 `0.0.0.0`，不用于匹配业务域名。

单域名可以直接写 `web.domain`；不配置域名且没有 Site 时，接受任意请求域名：

```yaml
web:
  port: 8080
  domain: example.com
```

配置 Site 后，未单独配置域名的 Site 自动使用 `<site key>.<web.domain>`。
例如 `sites.api` 使用 `api.example.com`；Site 的显式域名可以覆盖这个默认值。

一个 Server 可以配置多个 Site：

```yaml
web:
  port: 8080
  sites:
    www:
      domains:
        - www.example.com
    api:
      domains:
        - api.example.com
```

Site 使用 `domains` 指定匹配域名，支持字符串、列表和 `*.example.com` 通配符。
兼容单数 `domain` 和旧配置键 `hosts`；同时出现时优先级为 `domains` > `domain` > `hosts`。
Go 字段统一为 `Config.Domains`、`ServerConfig.Domains`、`SiteConfig.Domains`。

Web/Server 和 Site 都没有配置域名时，按请求域名首段匹配 Site key：

```yaml
web:
  port: 8080
  sites:
    api: {}
    www: {}
```

`api.abcd.com`、`api.test.abcd.com` 都会命中 `sites.api`。
只有未配置域名的 Site 才参与自动匹配；IP 地址不会被拆成 Site 名称。
匹配优先级为：显式精确域名、显式通配子域名、自动 Site 名称、显式 `*` 兜底。
未命中任何 Site 时返回 404，不会自动回退到第一个 Site。

多 Server 继承根级 `web.domain` / `web.domains`，可以各自覆盖；显式空值取消继承，
恢复按 Site key 自动匹配。域名匹配只发生在当前监听 Server 内。
URL 生成优先使用当前请求域名；自动 Site 之间跳转时替换 Site 前缀，
保留其余域名和请求端口，例如从 `api.test.abcd.com` 生成 `www.test.abcd.com`。

需要多个监听端口时使用 `servers`：

```yaml
web:
  servers:
    web:
      port: 8080
      sites:
        www:
          domains: [www.example.com]
        api:
          domains: [api.example.com]
    internal:
      port: 8081
```

`settings`、`cross` 可以出现在 `web`、Server 和 Site 层级。下层配置覆盖上层同名键。
`settings` 会与 Router 的 `Settings` 合并后提供给 `ctx.Settings`。

Server 还支持请求边界和连接配置：

```yaml
web:
  port: 8080
  options:
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

多 Server 时，`web.options` 提供默认值，`web.servers.<name>.options` 逐字段覆盖。
显式的 `0`、`false`、空代理列表可以覆盖继承值。Site 不单独设置监听服务选项。
Go 配置对象对应 `Config.Options` / `ServerConfig.Options`，类型为 `ServerOptions`。

配置键统一使用 `options`、`settings`，同时兼容单数 `option`、`setting`。
同层单复数配置按字段合并，复数键优先；旧的顶层超时、限制等字段也保留兼容，
优先级为 `options` > `option` > 旧顶层字段。Go 字段统一为 `Options`、`Settings`。

```yaml
web:
  options:
    read_header_timeout: 2s
    max_concurrency: 256
  servers:
    public:
      port: 8080
    internal:
      port: 8081
      options:
        max_concurrency: 0
      settings:
        visibility: internal
```

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
- `data` 为 nil 或空 Map 时省略该字段；未传 data 时使用 `ctx.Data`，响应过滤器仍可修改。
- `ctx.Answer(nil, Map{})` 表示成功，只返回 `code: 0` 和 `time`，省略 `text`、`data`。
  显式传入 `dubi.OK` 时保留成功状态的多语言文本。

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

## 认证

`Router.Sign` 要求请求携带有效 Token，`Router.Auth` 要求 Token 已认证，`Router.Kind` 可进一步限制身份类型。登录路由可以只设置 `Kind`，然后在 Action 中调用 `ctx.Auth`；它会使用当前 Router 的 Kind 生成 Token，并写入 Auth Cookie。

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

Token 可以通过 `Authorization: Bearer <token>` 或默认 Cookie 发送。`ctx.Kind()`、`ctx.Authed()` 和 `ctx.Payload()` 只能读取通过验证的 Token。

## 当前请求能力

`web.Context` 继承 `dubi.Meta`，可以直接调用当前进程的 Dubi 能力：

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
