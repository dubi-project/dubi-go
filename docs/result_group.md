# ResultGroup

按业务分组定义结果，自动生成失败编号和状态前缀，用法与 infrago 相同：

```go
var user = dubi.ResultGroup("user", 1000)

var (
    Created      = user.Result(true, "created", "创建成功")
    InvalidEmail = user.Result(false, "invalid.email", "无效电子邮箱")
    NotExists    = user.Result(false, "not.exists", "用户不存在")
)
```

对应结果：

| 定义 | code | status |
| --- | --- | --- |
| Created | 0 | user.created |
| InvalidEmail | 1000 | user.invalid.email |
| NotExists | 1001 | user.not.exists |

- 不指定起始编号时默认从 `1000` 开始。
- 成功始终为 `0`，不消耗失败编号。
- status 已带相同组名前缀时，不重复添加；空组名不添加前缀。
- 与 `dubi.Result` 一样，自动注册状态和默认文本，支持多语言和 `With(...)` 参数。
- `group.Name()` 返回组名。

在包初始化期间定义结果。编号按各组的定义顺序递增，不保证跨组唯一，调整定义
顺序会影响编号；需要稳定的业务标识时使用 status，需要固定编号时使用 `dubi.Result`。
