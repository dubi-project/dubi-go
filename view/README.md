# Dubi View

`view` 内置 Go `html/template`，提供页面、布局、局部模板和动态 Helper。
模块按进程加载，没有 Driver、Connection 或 Instance。
Web 引用 View，无需额外空引用；非 HTTP 程序也可以直接调用 `view.Parse`。

## Web 使用

```go
ctx.Data["user"] = user
ctx.View("order/detail", Map{"order": order})
```

`ctx.View` 只选择响应内容；Response Filter 执行后、写出响应前才渲染。
页面、布局、局部模板或 Helper 出错都会进入 Web Error Handler，不会输出部分 HTML。
Error Handler 可以选择另一个 View；如果它仍然渲染失败，返回普通文本 500，避免递归。

可选参数沿用 infrago 的动态风格：`Map` 为 Model，`int` 为 HTTP 状态，`string` 为 MIME。
默认输出 `text/html; charset=utf-8`：

```go
ctx.View("order/detail", Map{"order": order}, 200, "html")
```

Web 模板中的数据：

| 表达式 | 来源 |
| --- | --- |
| `.model` | 当前页面或局部模板的 Model |
| `.data` | `ctx.Data`，请求共享数据 |
| `.args` / `.value` | 校验后的参数 / 请求值 |
| `.locals` | 请求局部数据 |
| `.settings` | 当前路由合并后的 Settings |
| `.config` | 当前 Router 定义 |

## 布局和局部模板

页面 `asset/views/order/detail.html`：

```html
{% layout "layout" %}
{% title "订单详情" %}
{% style "/app.css" %}
<h1>{% .model.order.Number %}</h1>
{% render "shared/order" .model.order %}
```

布局 `asset/views/shared/layout.html`：

```html
<!doctype html>
<html>
<head>
  <title>{% title %}</title>
  {% metas %}
  {% styles %}
</head>
<body>
  {% body %}
  {% scripts %}
</body>
</html>
```

先执行页面和它的局部模板，收集标题、资源，再执行布局。
`layout "name" model` 可以单独传入布局 Model，未传时是空 Map；同时支持 JSON 对象字符串。
`render "name" model` 为局部模板传入独立 Model，同时共享请求 Data。
局部模板可以继续调用 `render`，相对目录跟随当前模板；嵌套深度最多 64 层。
一个页面选择一个布局，布局不能继续选择其他布局。

内置 Helper：

- `layout`、`body`、`render`：页面组合。
- `title`、`author`、`keywords`、`description`：传参数设置，不传参数读取普通文本。
- `meta name content [httpEquiv]` / `metas`：收集并输出 meta 标签。
- `style path [media]` / `styles`：收集并输出样式。
- `script path [type]` / `scripts`：收集并输出脚本，默认 `text/javascript`。
- `language`、`timezone`：本次渲染的语言、时区。
- `format`：通常使用 `fmt.Sprintf`；单个 `time.Time` 按当前时区和 Go 时间格式输出。
  为兼容 infrago，范围在 31507200–31507200000 的单个 `int64` 也按 Unix 秒处理。

普通文本、HTML 属性和资源 URL 由 `html/template` 做上下文转义。
`body`、`render` 只把已经渲染完成的内容作为 HTML 插入。
模板应由项目提供；自定义 Helper 返回 `template.HTML` 等可信类型时由业务保证内容安全。

Web 额外提供：`string`（`ctx.String`）、`uri`（`ctx.RouteUri`）、`url`（`ctx.RouteUrl`）、
`siteurl`（`ctx.SiteUrl`）、`ctx`（当前 Context）。语言状态 key 按原样传入。

## 注册 Helper

```go
dubi.Register("uppercase", view.Helper{
    Name: "大写",
    Alias: []string{"upper"},
    Action: strings.ToUpper,
})
```

模板中调用 `{% upper .model.name %}`。Action 支持 Go 模板函数的任意有效参数签名，
返回一个值，或返回一个值和 `error`。无效名称、签名和 nil 函数在 Setup 时返回错误。

Helper 的 Scope 是 `view:helper`，`Foundation()` 返回 true，所有 Profile 都会加载。
`Body.Helpers` 可对本次渲染覆盖已注册函数；引擎内置 Helper 保留，不能覆盖。
每次渲染都独立绑定函数，缓存不会持有请求 Context。

## 配置与查找

默认值如下，通常无需配置：

```yaml
view:
  root: asset/views
  shared: shared
  left: "{%"
  right: "%}"
  cache: true
  settings: {}
```

`settings` 兼容 `setting`；同时存在时按键合并，复数形式优先。
独立渲染未在 `Body.Data` 提供 `settings` 时，使用 View 的 Settings；Web 使用当前路由 Settings。

所有模板使用同一查找顺序，首个存在的文件生效：

1. 当前模板目录，仅布局和局部模板。
2. `root/site/language`，以及该目录的 `shared`。
3. `root/site`，以及该目录的 `shared`。
4. `root/language`，以及该目录的 `shared`。
5. `root`，以及 `root/shared`。

Site 或 Language 为空时跳过对应层。每个目录先尝试 `name.html`，再尝试 `name/index.html`；
名称已包含扩展名时按原名查找。模板名必须为相对路径，不接受 `..`、绝对路径或内联模板源码。

每个候选路径优先查 AssetFS，再查磁盘。文件系统在 Setup 时从 `dubi.AssetFS()` 获取，
因此请在 `init` 或启动之前设置。未启用 embed 时直接读取本地文件。
Root 可以是磁盘绝对路径；使用 AssetFS 时应使用相对资源路径。

缓存默认开启，首次使用时解析模板；更新模板后重启程序。开发时设 `cache: false`，
每次读取并解析当前文件。若模板已 embed，修改源码文件仍需重新编译程序。
缓存只保存模板结构，Model、Data、Helper 闭包、标题和资源列表均不跨请求共享。

## 独立渲染

在 Dubi 完成 Setup 后，即可生成邮件 HTML 等内容，无需 Web Context：

```go
html, err := view.Parse(view.Body{
    View: "mail/welcome",
    Language: "zh",
    Model: Map{"name": "Ada"},
    Data: Map{"brand": "Dubi"},
})
```

`Body` 还支持 Site、Timezone 和 Helpers。渲染失败时返回空字符串及原始错误链。

完整可运行示例在 [`examples/view`](../examples/view)。在该目录执行 `go run .`，访问
`http://localhost:18782/hello?name=Ada`。页面、布局、局部模板和 CSS 都内嵌到可执行文件中。
