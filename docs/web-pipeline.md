# Web 请求执行线

动态路由的处理顺序：

1. Serve Filters：请求入口。
2. 匹配路由，填充 Name、Config、合并后的 Settings 和路径参数。
3. Request Filters：解析和处理请求信息之前。
4. 解析请求参数、验证 Token 和路由认证、执行 Args 映射和校验、执行 Loading。
5. Execute Filters，随后执行 Router.Actions 和 Router.Action。
6. 路由与站点的结果处理，Response Filters，准备并写出响应。

Execute 中调用 `ctx.Next()` 会继续下一个 Execute Filter 或路由 Action，
返回后可以读取 Action 设置的结果。没有调用 Next 则不会继续执行 Action。

Execute 开始时即可读取 `ctx.Name`、`ctx.Config`、`ctx.Settings`、
认证信息、映射后的 `ctx.Args` 和 Loading 写入的 `ctx.Locals`。
同一个请求中的 Locals 会持续保留，请求开始时才清空。

未匹配路由的 404、方法不匹配的 405、路由规范化重定向、静态文件，
以及认证、参数校验或 Loading 失败的请求，
均不进入 Execute。HTTP 响应尚未写出时，使用 `ctx.StatusCode()` 读取当前
预计的 HTTP 状态；`ctx.Code` 可能仍为零。业务结果使用 `ctx.Result()`。

Request 在解析前就可以读取路由元信息。Response 在拿到 Action 或错误处理结果后，
写回客户端前执行，可以调整响应内容和响应头；其 Next 继续到写出响应。
正常调用链在 Request 返回后进入 Response，完成响应后才返回最外层 Serve。
