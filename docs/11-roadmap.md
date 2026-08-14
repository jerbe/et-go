# Go 版本路线图

路线图按“可运行闭环”排序，不按 package 数量排序。

## Phase 0：启动和错误边界

状态：大部分完成。

已完成：

- 严格加载 Machine/Process/Scene/Zone；
- 当前 Process 无 Scene 直接失败；
- Scene ID/Name 在 FiberInit 前设置；
- 网络地址缺失失败；
- Fiber mailbox/task queue 返回错误；
- 未启动 Fiber 的 Stop 会完成 Scene dispose 且禁止重启；
- 已取消的排队 Call 不进入目标 handler；
- HTTP/Session/KCP 启动错误可观察；
- KCP 使用标准 ARQ 状态机，并恢复 ET local/remote connection 帧；
- 未知 SceneType 在配置校验阶段拒绝；
- Fiber Update/LateUpdate 注册去重，Crontab 生命周期注册/注销幂等；
- Map transfer 使用完整目标 Unit ActorID，地图间错误响应保留 RpcID；
- 生产路径移除主要 fallback。

剩余：

- 对每个组件依赖做启动期诊断，而不是等第一个请求失败。

统一启动日志和当前 Process 的完整拓扑摘要已经完成；`Config.RuntimeTopology`
在创建 Fiber 前输出 Machine、peer、显式/隐式 NetInner 和 Scene 清单。

## Phase 1：完成跨进程消息闭环

状态：Go/KCP 自动化闭环已完成；真实多进程部署验收未完成。

目标：

```text
Process A
  ↔ peer handshake
  ↔ Session
  ↔ ProcessOuterSender
  ↔ Actor envelope
  ↔ Process B Fiber
```

已完成：

1. `StartProcessConfig.Peers` 声明远端 Process、地址和共享 secret；
2. `engine/network/peer` 定义版本化 HMAC handshake；
3. 较小 ProcessID 主动拨号，另一端被动接受；
4. 绑定远端 ProcessID 并调用 `ProcessOuterSender.AddSession`；
5. 接入 `HandleSessionPacket`；
6. 断线时调用 `ProcessOuterSender.RemoveSession` 使 pending RPC fail-fast；
   handshake Request 的 RpcID 非零校验；
7. 固定间隔 reconnect；
8. 同机双 peer Actor RPC、Session 替换和错误路径测试；
9. `cmd/server` 对 peer 配置自动创建 NetInner Fiber。
10. Router UDP/KCP 控制帧按目标 little-endian、方向字段和 9/13 字节长度
    对齐；RouterNode 生产启动分离外部 UDP/TCP 与内部 UDP，TCP 使用目标
    little-endian `uint16` body length。

剩余验收：

- 真实双 OS 进程启动；
- 跨机器可达地址、secret 管理和防火墙；
- TLS/监控/健康检查；
- 客户端与真实 Realm/Gate/Map 多进程联调。
- 目标 C# Router 的 TCP 外部 transport 已按 little-endian `uint16` body
  length 和连接复用实现；WebSocket 外部 transport 仍需独立客户端协议、
  传输选择和版本矩阵，当前不能把 TCP/UDP 当作 WebSocket fallback。

## Phase 2：数据和认证安全

已完成：

1. 账号表增加 `password_algorithm`；
2. 新账号使用 Argon2id；
3. 旧 MD5 账号首次成功登录迁移；

剩余任务：

4. AccessToken 签名 Token、key-id 和 key ring 轮换已完成；
5. 旧 XOR Token 显式 legacy 迁移已完成；
6. AccessToken 撤销存储接口、MongoDB 共享适配器、TTL migration 和启动 wiring
   已完成，剩余真实 MongoDB/多进程验收；
7. HTTP TLS 强制模式和新连接证书热轮换已完成，剩余密钥托管、原子文件替换
   约束和生产验收；
8. MongoDB 原子 bucket 登录限流和 HTTP 审计写入已完成，剩余限流故障策略、
   审计保留策略和真实 MongoDB 验收。

Gate Token 已使用 crypto/rand 生成随机段，同时保持原
`D5.accountID.D4` Hex wire format；随机源失败会阻断登录，不注册空 Token。

密码迁移不改变客户端登录字段；Token 和传输/运营安全仍需要独立兼容窗口。

## Phase 3：数据库 migration 和持久化闭环

Schema migration runner 已实现：

- 独立 migration runner；
- `schema_migrations` 版本记录；
- `schema_migration_locks` MongoDB 租约锁、owner、心跳续租和上下文取消；
- v1 account username unique index；
- v2 player profile `(account_id, zone_id)` unique compound index；
- v3 account `password_algorithm` marker for legacy MD5/Argon2id documents；
- migration 失败阻止 DB Scene 启动；
- `cmd/server` 按 Zone 执行 migration 并关闭 Client。

以下仍未完成：

- startup migration 租约锁、超时和取消的真实 MongoDB 验收；
- 业务数据 backfill、密码和 Token 迁移；
- UnitDumper 多组件一致性快照和 MongoDB 故障恢复（单组件 durable queue、
  Fiber 内 BSON snapshot、有限 retry 和内存 dead-letter 已完成）；
- Map transfer source journal 和目标 ledger 持久化已完成；Go 已提供
  `TransferRecoveryCoordinator` / `TransferLedgerRecoveryCoordinator` 的严格
  状态收敛编排；跨进程查询、recovery token、ownership 证明和完整跨组件事务
  仍未完成；
- DB Increment 已对齐原 C# 的“默认 upsert 后独立 `$inc`”语义，避免 MongoDB
  同路径 update conflict；
- 多组件一致性快照。
- 重复玩家档案的历史数据清理和唯一索引上线后的 MongoDB 验收。

## Phase 4：Map 领域闭环

任务：

1. 定义 Map target 配置（同进程已完成）；
2. 定义跨进程 Map Actor discovery（peer transport 已完成，但 Map 注册/选择协议
   仍未定义）；
3. 接入 Recast/navmesh Finder；
4. 给 Unit 初始化注入 Finder（接口已完成，生产工厂未完成）；
5. 完成真实客户端 EnterMap；
6. 完成双进程 TransferMap；
7. 增加 transfer crash/retry/rollback 测试；source journal 和目标 durable
   ledger 已有实现，仍缺跨进程状态查询、恢复扫描器和完整事务原子性；
8. Inventory 持久化和可靠通知。

当前已完成：

- Unit/AOI/Location 严格初始化；
- 两阶段迁移；
- 未知迁移组件失败；
- 坐标保持；
- Pathfinding 缺 Finder 失败。
- Map handler 严格验证 Gate 路由传入的 Unit ActorID，不按第一个 Unit 猜测；
- source journal 要求有效 source ActorID，Unit snapshot 拒绝未知身份；
- durable ledger 在完成记录持久化后才返回成功；

阻塞原因：原项目的 DotRecast 资源格式和 Go 依赖尚未确定；Map target 还缺少
跨进程远端 Actor 注册、负载选择和失效恢复协议。同进程 Zone/Name 显式解析
已经完成。

## Phase 5：Lockstep 可运营化

任务：

1. Go LSWorld 确定性状态模型（已完成：单位、位置、旋转、输入、帧和
   ID watermark）；
2. 序列化版本（`RoomStateSnapshot v2` 已完成，包含 Go LSWorld）；
3. snapshot checksum（已完成）；
4. snapshot provider 注入（默认 Room Fiber 已完成）；
5. replay 持久化；
6. reconnect 恢复；
7. hash mismatch recovery；
8. 房间资源和动态 Fiber 回收；
9. 帧推进压力测试。
10. Room snapshot 恢复先校验后提交，Replay 保留连续帧和空帧；
11. AOI re-entry 清理旧 Cell；同类型 ECS component 替换清理旧 Entity 引用；
12. 注入 Map/Match 负载选择策略，保留 Zone/Name 显式解析，并替代当前
    缺少目标信息时的歧义失败保护。
13. 补全 Match Room 创建与 Gate 通知的事务/取消协议；当前已增加
    `Match2Map_CancelRoom`，Map 回收和 Gate PlayerRoom 补偿使用有限幂等重试，
    仅在补偿全部成功后重新入队；剩余跨进程持久化补偿和故障恢复协议。

当前明确不做：

```text
跨语言协议未确认时，不把 Go JSON snapshot 伪装成 C# TrueSync wire。
```

当前 Go snapshot 已覆盖 `Room/FrameBuffer/RoomPlayer/LSWorld`，并严格检查
Room authority frame、Updater frame 和 LSWorld frame 一致；完整客户端战斗
恢复仍需 TrueSync fixed-point wire、客户端 replay/hash-mismatch 协议和版本
迁移规则，不能把 Go JSON 快照当作原客户端二进制。

当前外部 Room 消息路由已完成协议和 Gate 绑定：Match 成功通知携带完整
三段式 ActorID，Gate 写入 `PlayerRoomComponent`，随后将客户端 Room 消息
直接发送至绑定 Room Actor。剩余工作是跨进程真实验证和客户端联调，不是
缺少路由实现。

## Phase 6：可观测性和工程化

任务：

- Prometheus metrics；
- Fiber/mailbox queue metrics；
- RPC latency/timeout；
- Location lock contention；
- DB latency/error；
- KCP packet loss/reconnect；
- structured trace ID；
- health/readiness；
- graceful shutdown report；
- CI 中 build/test/vet/race；
- real integration profile。

### 6.1 外部实践对照

以下对照只用于确定工程任务，不改变 ET-Go 按 Go 运行对象、消息路径和
领域状态组织代码与文档的原则。

| 对照项目 | 可借鉴的边界 | ET-Go 的落地结论 |
|---|---|---|
| [ET](https://github.com/egametang/ET) | 原 ET 关注分布式服务、MongoDB/BSON、同步工具和客户端/服务端协同 | 保留协议兼容资料与 Go 实现资料分离；不能把原 C# package 名称当作 Go 目录设计依据 |
| [Nakama](https://github.com/heroiclabs/nakama) | 将账号、匹配、实时多人和社交后端能力作为可运营服务能力 | ET-Go 继续维护自己的 Realm/Gate/Match/Room 领域边界；把账号、匹配、房间恢复和客户端版本矩阵列为独立验收项 |
| [Leaf](https://github.com/name5566/leaf) | 模块独立运行、消息/RPC 作为模块间边界、强调生命周期和稳定性 | 继续坚持一个 Fiber/Scene 的所有权模型；跨 Fiber 只能通过 MailBox/RPC，不允许共享业务指针 |
| [Pitaya](https://github.com/topfreegames/pitaya) | 集群模式使用服务发现和 RPC 传输，并把 unit/e2e 测试作为运行入口 | ET-Go 的 peer handshake 已完成，但远端 Map/Room discovery、跨进程 e2e 和故障重连仍必须由部署层补齐 |
| [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/) | 以统一 API 采集 traces、metrics、logs，并由 exporter/collector 解耦后端 | 优先为 Actor RPC、Map transfer、登录、KCP reconnect 和 Room snapshot 建立 trace/span 语义，不直接把 slog 改成业务自定义协议 |
| [Prometheus client_golang](https://github.com/prometheus/client_golang) | 应用埋点和 Prometheus API 客户端职责分离，支持独立 registry | 增加独立 metrics HTTP 入口和 registry；首批指标覆盖 mailbox 深度、RPC 延迟/超时、Location lock、KCP 重连、MongoDB 错误和 Room snapshot 失败 |

对照后的优先级调整：

1. 先完成 health/readiness、metrics 和 trace ID 传播，再做生产压测；
2. 为双 OS Process 增加可重复的 Compose/脚本配置，测试 peer discovery、
   Actor RPC、Map target discovery、断线重连和进程重启；
3. 为 Lockstep 增加跨语言 golden packet/snapshot，确认 MemoryPack/TrueSync
   wire 后再实现客户端恢复；
4. 不引入 QUIC/WebSocket 作为未经协议确认的替代传输；先完成现有 KCP、
   Router UDP/TCP 的观测与故障指标。

## Phase 7：协议和客户端兼容

任务：

1. 维护 MsgID/field/error code registry；
2. 生成协议兼容报告；
3. 客户端版本矩阵；
4. golden packet；
5. 旧 Token/旧登录响应迁移；
6. snapshot/replay 版本兼容。
