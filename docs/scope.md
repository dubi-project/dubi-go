# Scope

`dubi.Scope` 简化一组注册中重复的业务名称前缀，用法与 infrago 相同：

```go
var user = dubi.Scope("user")

func init() {
    user.Register("get", dubi.Method{Action: getUser})
    user.Register("create", dubi.Service{Action: createUser})
}
```

上面分别注册 `user.get`、`user.create`，调用和 Profile 匹配使用完整名称。
它只拼接注册名称；组件自己的 `Scope()` 不变，例如 Method 的完整匹配键仍是
`core:method:user.get`。Library、Type 等基础资源仍始终加载。

规则：

- 前缀和名称去除首尾空白并转小写。
- 已带前缀的 `user.get` 不重复添加 `user.`。
- `.shared`、`*.access` 原样注册。
- 省略名称时，以前缀本身为注册名称，例如 `user.Register(dubi.Library{...})`。
- 空前缀可用于不添加前缀的注册，注册名称仍不能为空。
- `Register(args ...any)` 的最后一个字符串参数作为名称，其他参数作为注册值。
  普通组件同名覆盖，最终保留最后一个值；Trigger 同名追加为多个监听器。

所有注册仍写入同一个全局 Registry，启动后受相同的冻结规则约束。
