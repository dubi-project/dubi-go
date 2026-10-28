# Trigger

Trigger 是 dubi-go 主包提供的进程内通知机制。注册使用逻辑名称，同名 Trigger
追加为多个监听器，普通组件仍按原有规则同名覆盖。

```go
dubi.Register(dubi.START, dubi.Trigger{
    Name: "Started",
    Action: func(ctx *dubi.Context) {
        // 所有模块都已完成 Start。
    },
})

dubi.Register("data.test", dubi.Trigger{
    Args: Vars{"table": {Type: "string", Required: true}},
    Action: func(ctx *dubi.Context) {
        ctx.Invoke("audit.test", ctx.Args)
    },
})
```

## 触发

```go
dubi.Toggle("data.test", Map{"table": "user"})     // 各监听器异步并发执行
dubi.SyncToggle("data.test", Map{"table": "user"}) // 按注册顺序同步执行
```

每个监听器有独立的 Meta、Context、Settings 和动态 Map/List/字节数据快照，
不会通过修改参数污染其他监听器。Action 固定为 `func(*dubi.Context)`，不返回 error
或业务数据；复用 Method 的 Args 映射和 Res 语义，需要标记失败时用 `ctx.Result(res)`。
Nullable 表示允许部分参数，不注入缺失参数的默认值。
触发方法不返回业务结果。参数映射失败、回调标记失败或 panic 会记录日志，继续执行
其余监听器；空 Action、没有监听器或尚未完成 Trigger 加载时触发均为空操作。

## Profile 和 Registry

Trigger 的 Scope 是 `core:trigger`，属于可由 Profile 选择的业务注册：

```go
dubi.Register("worker", dubi.Profile{
    Include: []string{"core:trigger:start", "core:trigger:stop", "core:trigger:data.*"},
})
```

同一逻辑名称下的监听器一起被 Include/Exclude 匹配。Entry.Name/Key 始终使用逻辑
名称，例如 `data.test` / `core:trigger:data.test`。Setup 为每个回调生成唯一内部
Method 名称，内部名称不进入 Registry，也不参与 Profile 筛选。

Registry.Entries 保留同名 Trigger 的注册顺序。同名的普通组件和 Trigger 可以共存；
Lookup 优先返回普通组件，没有普通组件时返回最后一个 Trigger，完整监听器列表
通过 Entries 获取。Trigger 仍受启动时 Registry 冻结的限制。

## 模块通过 Host 发出

模块保存 Mount 返回的 Host，自行定义通知名称和参数，统一由主包执行回调：

```go
const TEST = "data.test"

var host dubi.Host
var module = &Module{}

func init() { host = dubi.Mount(module) }

// 模块内需要通知业务时：
host.Toggle(TEST, Map{"table": "user"})
// 如果模块需要等待回调完成：
host.SyncToggle(TEST, Map{"table": "user"})
```

业务侧使用 `dubi.Register(data.TEST, dubi.Trigger{...})` 即可。这里的 TEST 是模块
自定义通知的示意，框架不会自动给模块生成通知名称或发出未定义的事件。
Host 与包级触发方法使用同一套匹配和执行逻辑。模块 Setup/Open/Start/Stop 时也可发出
已加载的通知；若回调需要使用当前模块资源，应在资源释放前用 SyncToggle 等待完成。

## 生命周期

- `START = "start"`：所有模块 Start 成功后自动异步触发。
- `STOP = "stop"`：停止各模块之前自动同步触发，回调仍可使用模块能力。
- 启动失败不会发出 START 或 STOP。
- 停机和启动回滚会取消未完成回调的 Context 并等待其结束。
- Close 开始后停止接收通知；Reset 清空监听器和内部 Method。

回调应响应 `ctx.Context.Done()`，并结束自己的工作。不要在回调内同步调用 End/Reset：
关闭流程需要等待该回调返回。程序退出由主调用方或其他独立的控制流程负责。
Trigger 只在当前进程执行，不跨节点分发，不持久化，也不自动重试。

完整模块示例：`go run ./examples/trigger`。
