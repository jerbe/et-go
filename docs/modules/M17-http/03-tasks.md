# M17-http：HTTP 账号接口与 Area 列表

> `M17` 是原 ET 的兼容审计编号；Go HTTP 实现按 `HttpComponent → Dispatcher
> → Handler → AccountRepository` 组织在 `module/http`。

## 目标边界

目标项目证据：

- `cn.etetet.http/Scripts/Model/Server/HttpComponent.cs`
- `cn.etetet.http/Scripts/Model/Server/HttpDispatcher.cs`
- `cn.etetet.http/Scripts/Hotfix/Server/HttpComponentSystem.cs`
- `cn.etetet.http/Scripts/Hotfix/Server/Http/HttpPostLoginHandler.cs`
- `cn.etetet.http/Scripts/Hotfix/Server/Http/HttpPostRegisterHandler.cs`
- `cn.etetet.http/Scripts/Hotfix/Server/Http/HttpGetAreaListHandler.cs`
- `cn.etetet.http/Scripts/Core/Server/Helper/HttpHelper.cs`

## 任务状态

| 任务 | Go 实现 | 验证 |
|---|---|---|
| `/login`、`/register`、`/get_area_list` 注册 | `HttpComponent.ensureDispatcherLocked` | `TestHTTPComponentIntegration` |
| POST 方法检查与统一 JSON 响应 | 三个默认 Handler、`response.go` | handler/dispatcher 测试 |
| 目标字段名 `RequestId/Error/Message/AccessToken/Areas` | Go JSON tags | HTTP response 解码测试 |
| 目标错误包体和有效 HTTP 状态 | `WriteNotFound`/`WriteInternalServerError` 保持目标 `ResponseJson` 的最终 200 | `TestDispatcher`、Router Manager 集成测试 |
| 目标 JSON Content-Type | `application/json; charset=utf-8` | `TestDispatcher` |
| 账号注册 ID 从中央 DB 递增 | `dbAccountRepository` | repository/集成入口 |
| 旧密码读取，新密码 Argon2id | `central.VerifyPassword/HashPassword` | login/register 测试 |
| HTTP 组件监听、停止和 Serve 错误可见 | `HttpComponent.Start/OnDestroy/ServeError` | component 测试 |
| CORS | 配置化精确 allowlist | allow/reject 测试 |
| TLS | `httpRequireTLS` 严格启动校验；新连接通过 `GetCertificate` 重新读取证书文件 | TLS 启动/轮换测试 |
| 登录限流 | 生产 HTTP Fiber 使用 MongoDB 原子 bucket；独立测试可使用进程内组件 | 组件边界测试；真实 MongoDB 未验收 |
| 登录审计 | `LoginAuditSink` + DBManager 懒加载 Mongo sink，写入失败向上返回 | 成功/失败事件和错误传播测试 |

## HTTP 语义

目标 `HttpHelper.ResponseJson` 在写 JSON 时会把 HTTP status 统一写为
`200`；因此目标的 404/500 是 JSON 包体中的 `Error`，不是最终网络状态码。
Go 保持这个行为：

```json
{
  "RequestId": "",
  "Error": 404,
  "Message": "404 Page Not Found"
}
```

同时已对齐目标的 `Content-Type: application/json; charset=utf-8`。

## 安全策略边界

目标 HTTP helper 使用 `Access-Control-Allow-Origin: *`。Go 默认不复制这个
安全风险，要求 `startsecurityconfig.json` 提供精确 `corsAllowedOrigins`，
并拒绝 `*`。这不改变业务 JSON、路由、字段或错误码，是显式部署策略；
未配置来源时不发送 CORS 头。AccessToken 默认使用 HMAC，旧客户端需要
显式 `accessTokenFormat=legacy`，见 M06。

## 错误边界

- 缺少请求、响应写入器、Scene/DB/Repository：返回明确 Go error；
- 空用户名或密码：返回 JSON `400` 业务错误；
- 数据库错误：返回统一 500 包体，不切换内存账号；
- 重复用户名：返回目标错误码 `100016001`；
- Handler panic：Dispatcher 记录并返回统一 500 包体；
- 请求体受 `1 MiB` 限制，超限不进入业务逻辑。

## 验证命令

```bash
go test -count=1 ./module/http ./module/router
go test -race ./module/login ./module/central ./module/router ./module/map_ ./module/actorlocation ./cmd/server
```

真实 MongoDB、跨进程 Central、TLS 密钥托管、限流故障策略和审计保留协议仍
属于部署与运营能力，状态见 `docs/IMPLEMENTATION_STATUS.md`，不在 HTTP wire
对齐结论中伪装为已验收。
