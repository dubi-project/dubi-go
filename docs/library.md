# Library

Library 是带名称前缀和默认 Settings 的本地 Method 集合，属于主包的基础资源。
和 Type、Codec 一样，注册后始终加载，不受 Profile 的 Include/Exclude 影响。

```go
dubi.Register("mail.provider", dubi.Library{
    Name: "Mail provider",
    Settings: Map{"account": "default"},
    Methods: dubi.Methods{
        "send": {
            Args: Vars{"subject": {Type: "string", Required: true}},
            Action: func(ctx *dubi.Context) Map {
                return Map{"account": ctx.Settings["account"], "subject": ctx.Args["subject"]}
            },
        },
    },
})
```

模块初始化时使用 `dubi.Register` 注册，Runtime 启动后即可在 Method、Service、
Web Context 等所有嵌入 Meta 的上下文中调用：

```go
mail := ctx.Library("mail.provider", Map{"account": "tenant-a"})
data := mail.Invoke("send", Map{"subject": "hello"})
result := mail.Result()
```

也可以使用 `dubi.NewMeta().Library(...)`。完整示例：

```sh
go run ./examples/library
```

## 注册与调用

- `Library` 包含 `Name`、`Desc`、`Settings`、`Methods`。
- Scope 为 `core:library`，是全局基础资源，Profile 无法排除。
- `Methods` 为 `map[string]dubi.Method`，成员注册为 `<Library 名称>.<成员名称>`。
- Library 前缀和成员名称统一转小写，去除首尾空白及点号，允许点分层级名称。
- 成员使用与普通 Method 相同的动态 Action 签名、Args 和 Data 映射。
- 显式注册同名的完整 Method 可以覆盖 Library 提供的默认实现。
- `lib.Invoke` 始终本地执行 Method，不会调用 Service 或通过 BUS 远程分发。
- 缺失方法返回失败的 Res，并记录到 `lib.Result()` 和 `ctx.Result()`。

Library 共用 core 模块，无需单独 Mount，也不引入驱动或外部依赖。

## Settings

调用时按以下顺序逐字段覆盖，后者优先：

1. Method 自身的 Settings。
2. Library 的默认 Settings。
3. `ctx.Library(name, settings...)` 的覆盖值，多个 Map 按传入顺序覆盖。

每个 invoker 保存独立配置快照，每次调用再复制给 `ctx.Settings`。
请求内修改嵌套 Map/List 不会污染 Library 默认值或其他请求。

成员也可以通过完整名称 `ctx.Invoke("mail.provider.send", args)` 调用；这种普通
Method 调用只使用 Method 自身的 Settings。需要 Library 配置时使用 `ctx.Library`。

## Meta 与结果

Library 调用沿用当前 Meta，保留请求 ID、Context、认证信息、语言等请求状态。
每次调用同时更新 invoker 与 Meta 的结果；`Result()` 读取不清空结果，下一次调用
覆盖旧结果。首次调用前 `lib.Result()` 为 `dubi.OK`。

提供 `InvokeOK` 和 `InvokeFail` 便捷方法。每个请求创建自己的 invoker，不要跨请求
共享持有 Meta 的 LibraryInvoker；Library 定义本身可以被所有请求复用。
