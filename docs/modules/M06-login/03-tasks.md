# M06-login：登录认证与 Realm/Gate 流程

> `M06` 是原 ET 的兼容审计编号；Go 实现不按该编号拆 package。实际代码位于
> `module/login`、`module/gate`、`module/actorlocation`、`module/map_` 和
> `cmd/server`。

## 目标边界

目标项目证据：

- `cn.etetet.login/Scripts/Hotfix/Server/Realm/C2R_LoginHandler.cs`
- `cn.etetet.login/Scripts/Hotfix/Server/Gate/C2G_LoginGateHandler.cs`
- `cn.etetet.login/Scripts/Hotfix/Server/Gate/R2G_GateAssignHandler.cs`
- `cn.etetet.login/Scripts/Hotfix/Server/Gate/GateSessionKeyComponentSystem.cs`
- `cn.etetet.login/Proto/LoginOuter_C_2000.proto`
- `cn.etetet.login/Proto/LoginOuter_C_2100.proto`
- `cn.etetet.login/Proto/LoginInner_S_22000.proto`

Go 运行链：

```text
HTTP/Router 登录
  → AccessToken
  → Realm C2R_Login
  → Location Account Lock
  → Gate R2G_GateAssign
  → Gate Token
  → Gate C2G_LoginGate
  → Central PlayerProfile
  → Home Map Unit
  → Player/GateSession/Unit Location
  → Account Unlock
```

## 任务状态

| 任务 | Go 实现 | 验证 |
|---|---|---|
| 保持登录 MsgID、字段号和 RpcID | `module/login/messages.go`、`proto/login_*.proto`、`proto_codec.go` | `module/login` 编解码与流程测试 |
| Realm 校验 AccessToken、按 Zone 选 Gate、调用 Gate Assign | `handler_c2r_login.go`、`gate_registry.go` | `TestHandleC2RLogin`、`TestLoginFlowAlignment` |
| Gate Token 一次性消费、GateID 校验、Session 认证 | `gate_session_key_component.go`、`handler_c2g_login_gate.go` | `TestHandleC2GLoginGate`、无效 Token 测试 |
| Gate Token 随机性与历史 wire format | `token_helper.go`、`handler_r2g_gate_assign.go` | `TestGenerateGateTokenPreservesWireFormat`；crypto/rand 失败传播，空 Token 不注册 |
| Gate 调用 Central 取得 PlayerID | `handler_c2g_login_gate.go`、`module/gamelogin` | 登录端到端替身流程 |
| 缺少 Unit 时显式调用 Home Map | `ensurePlayerUnitLocation` | Home Map 缺失返回错误 |
| 登录失败路径释放 Account Lock | `HandleC2GLoginGate` 的 deferred unlock | `TestHandleC2GLoginGateUnlocksAccountWhenHomeMapFails` |
| Gate/Player/Session/Location 组件严格挂载 | `ensure*`、`registerLocations` | 组件类型和位置注册测试 |
| 旧 SimpleToken 兼容 | `token_helper.go` 的显式 `legacy` 模式 | `TestExplicitLegacyAccessTokenFormat` |
| 默认 Token 安全格式 | HMAC key ring、key-id、轮换 | `TestSignedAccessTokenAndKeyRotation` |
| Token 撤销 | Memory 测试 store；MongoDB 共享 store、TTL migration 和 cmd/server wiring | store 依赖边界测试；真实 MongoDB 未验收 |

## Token 兼容规则

目标项目的 `SimpleToken` 是固定 key 的 XOR/Base64/Hex 格式。Go 默认生成
`v2.<key-id>.<payload>.<signature>` HMAC Token；只有同时配置：

```json
{
  "accessTokenFormat": "legacy",
  "allowLegacyTokens": true,
  "legacyTokenKey": "whosyourdaddy"
}
```

才生成原 SimpleToken。旧 Token 的验证也必须显式允许。这样同时满足默认
安全策略和旧客户端迁移，不使用隐式历史 key fallback。密码算法迁移属于
`M07-central`，不改变本模块协议字段。

## 错误边界

- AccessToken 未配置：返回 `login.ErrTokenConfigRequired`；
- AccessToken 无效/过期：返回 `100009003/100009004`；
- Gate Token 无效：返回 `100009001` 并关闭 Session；
- Central、Location、Home Map、MessageSender 缺失：返回明确错误；
- Home Map 已修改但后续 Gate 初始化失败：释放 Account Lock，不伪造登录成功；
- Gate 发送失败、GateID 不一致、响应 Token 为空：登录失败。
- Gate Token 生成随机源失败：返回 `login.ErrGateTokenGeneration`，不伪造登录成功。

## 验证命令

```bash
go test -count=1 ./module/login ./module/gate ./module/actorlocation
go test -race ./module/login ./module/central ./module/router ./module/map_ ./module/actorlocation ./cmd/server
```

默认测试不能证明真实客户端、真实 MongoDB、跨 OS Process 和 KCP 客户端
兼容；这些证据统一记录在 `docs/IMPLEMENTATION_STATUS.md`。
