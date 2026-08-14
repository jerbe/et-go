# ET-Go 行为兼容性审计

本文是兼容性附录，不定义 Go 包架构。主架构请看：

- [项目总览](./01-project-overview.md)
- [Go 工程架构与运行边界](./02-go-package-architecture.md)
- [服务流程](./05-service-workflows.md)

## 1. 审计对象

审计关注：

- ActorID 三段结构；
- Fiber/Scene/Entity 生命周期；
- MailBox/MsgID/RpcID；
- Location lock/get/unlock；
- Realm/Gate/Central 登录；
- Map enter/transfer；
- AOI/StateSync；
- Inventory；
- Match/Room/Lockstep；
- protobuf field number；
- HTTP response/error semantics。

审计不把“目录名相似”当作兼容证据。

## 2. 结果摘要

| 领域 | 当前 Go 行为 | 结论 |
|---|---|---|
| ActorID | ProcessID/FiberID/InstanceID | 已保持三段语义 |
| Scene identity | 配置 ID + runtime InstanceID | 已明确分离 |
| 同进程 RPC | 经过 Fiber task queue | 比直接跨 goroutine 更严格 |
| 跨进程 envelope | 20 bytes big endian ActorID；handshake Request 要求非零 RpcID | `ProcessOuterSender` + `engine/network/peer` 同机 KCP/RPC 通过，真实部署未验收 |
| Location Get | 锁时返回稳定错误，未分配 key 可返回零 Actor | 需要客户端确认零值语义 |
| Login Token | 默认 HMAC-SHA256 `v2.<key-id>.<payload>.<signature>`；Gate Token 使用 crypto/rand 保持 `D5.accountID.D4` Hex；`accessTokenFormat=legacy` 显式生成原 SimpleToken | Go 默认安全格式、MongoDB 共享撤销适配器、HTTP TLS 强制、证书热轮换、MongoDB 原子 bucket 限流和 HTTP 审计写入已实现；真实 MongoDB、限流故障策略、密钥托管和审计运营未完成 |
| Password | 新账号 Argon2id；旧 MD5 首次成功登录升级 | 密码迁移已实现，历史数据需真实 MongoDB 验收 |
| Map transfer | BSON Unit + components；严格校验 source ActorID、UnitID/ConfigID/UnitType | Gate 路由到完整 Unit ActorID；同 Zone/同 Process 两阶段提交；source journal/target durable ledger 和显式 recovery 编排已实现，真实跨进程协调与崩溃恢复未验收 |
| Unknown component | 返回错误 | 与“数据不完整不可静默成功”一致 |
| Pathfinding | Finder 缺失返回错误 | 不再返回直线兜底 |
| RoomStateSnapshot v2 | Provider 缺失返回错误；连续帧保留空帧；Go LSWorld、单位身份和 Room/World 帧先校验后原子提交 | Go 内部世界恢复已实现；原 C# MemoryPack/TrueSync wire、客户端恢复未验收 |
| Inventory | 背包/仓库独立错误码 | 新增错误码需客户端矩阵 |
| HTTP | JSON response + Error/Message；目标最终 HTTP status 为 200；Content-Type 为 `application/json; charset=utf-8` | Go 已对齐 JSON 包体、状态和 Content-Type；CORS 改为显式 allowlist |
| Router UDP/TCP | little-endian、方向相关 9/13 字节控制帧；RouterSYN/ACK、ordinary SYN/ACK、MSG、FIN、Reconnect；TCP 外层为 little-endian `uint16` 长度前缀 | Go 已对齐 UDP/KCP 数据面，生产外部 UDP/TCP 与内部 UDP 分离；WebSocket 外部变体仍未实现 |

## 3. 重点流程回归

### 3.1 登录

```text
AccessToken
  → Realm Lock(Account)
  → Gate Token
  → Gate Verify
  → Central PlayerProfile
  → Map Unit
  → Location Player/GateSession/Unit
```

需要同时验证：

- GateID；
- PlayerID；
- Location owner；
- account unlock；
- session close；
- disconnect notification。

### 3.2 Map transfer

当前只证明同 Zone、同 Process 的 Map transfer；Map target 的跨进程注册/选择、
以及 transfer 崩溃恢复仍未完成。source journal 的
`TransferJournalComponent.Recover` 与 target ledger 的
`RecoverProcessing` 只负责调用显式协调器并在 token/ownership/terminal-response
证明充分时收敛状态，不提供默认跨进程实现。peer handshake、真实 KCP channel
和 Actor RPC 已有同机自动化测试，但真实 OS 进程仍未验收。

客户端请求必须先由 Gate 按认证 Session 的 UnitID 路由到真实 Unit MailBox。
Map handler 会再次校验 ProcessID、FiberID、InstanceID 三段 ActorID；不
按第一个 Unit 或仅按 PlayerID 猜测目标。内部 transfer request 要求非零
RpcID 和有效旧 owner ActorID，拒绝响应保留 RpcID。

```text
source lock
  → serialize
  → target prepare
  → target ownership switch
  → source dispose
```

失败时必须保证：

- source Unit 仍存在，或有明确恢复状态；
- target 不留下半初始化 Unit；
- Location 不指向不存在 Actor；
- 客户端收到明确失败。

### 3.3 Lockstep

需要确认客户端协议对以下情况的定义：

- 无 snapshot；
- hash mismatch；
- replay tail；
- Room dispose；
- 玩家离线；
- 动态 Room ActorID。

当前 Go 代码选择“无 RoomStateSnapshot 直接报错”，不使用默认字节。默认
快照是 `RoomStateSnapshot v2`，包含 Go `LSWorld` 的确定性单位状态；它仍
不是原 C# MemoryPack/TrueSync fixed-point wire。

客户端 Room 消息已按原 C# 的 `Session → PlayerRoomComponent → RoomActorId`
语义接入 Gate。Lockstep inner/outer protobuf 新增完整三段式 `ActorId` 字段，
同时保留旧数值字段；Gate 在 Match 成功通知时建立绑定，并将
ChangeSceneFinish、FrameMessage、CheckHash 直接发送到绑定的 Room Actor。
路由缺失或 ActorID 不完整时失败，不按 Unit Location 兜底。
AOI 重新 Enter 其他 Cell 时先移动并清理旧 Cell；同类型 ECS 组件替换时先
清理旧 Entity 引用。DB Increment 使用“默认 upsert 后独立原子 `$inc`”，避免
MongoDB 同路径 update conflict。

## 4. 兼容性限制

以下差异不能只用 Go 单测解决：

1. 原客户端是否把零 ActorID 当作“未登录/未进入地图”；
2. 旧 MD5/XOR 数据迁移与 Token 升级；
3. Inventory 新错误码；
4. Snapshot/Replay 二进制版本；
5. 跨进程 Session handshake 的真实双进程/跨机器验收；
6. Router 外部地址和内网地址选择；
7. 真实 KCP packet timing。

这些项目需要客户端协议、部署配置或原运行时行为证据。

## 5. 审计规则

协议或行为改动必须更新：

```text
proto/*.proto
  → generated pb.go
  → module message struct
  → proto_codec.go
  → handler
  → Go tests
  → docs/08-protocol-and-api-reference.md
```

如果缺少足够证据，不写“兼容”，写出具体 TODO 和阻塞原因。
