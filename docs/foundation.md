# 基础资源

注册项实现 `dubi.Foundation` 并返回 `true`，就会在所有 Profile 中加载，
不受 `include` 和 `exclude` 影响。Scope 仍用于区分注册项和查找。

```go
type Resource struct{}

func (Resource) Scope() string    { return "example:resource" }
func (Resource) Foundation() bool { return true }

func init() {
    dubi.Register("shared", Resource{})
}
```

内置的 Type、Language、Mime、Status、Statuses、Codec、Library，以及
Data 的 Table、View、Model 均为基础资源。MIME 定义使用专用类型，例如
`dubi.Register("example", dubi.Mime{"application/example"})`。

也可以使用 `Mimes` 批量注册，每个扩展名都会成为独立的 `Mime` 注册：

```go
dubi.Register(dubi.Mimes{
    "json": "application/json",
    "jpg":  "image/jpeg",
})
```

自定义类型也可以实现 `dubi.RegistrationSet`，通过 `Registrations()` 返回按
名称索引的注册值，交给 `dubi.Register` 展开。

`dubi.Types`、`dubi.Codecs`、`dubi.Statuses` 和 `dubi.Regulars` 同样可以批量注册。
`Regulars` 每个名称可以包含多个正则，启动时编译；`dubi.Match(name, value)`
在任一正则匹配时返回 true，`dubi.Expressions(name)` 获取原始表达式。

Table 定义全部加载后，如果开启 Data 启动时自动迁移，迁移会看到全部加载的
表定义；请按实际部署需要配置迁移。
