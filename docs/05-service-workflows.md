# 服务拓扑与业务流程

本文从请求和状态变化描述服务，不按 ET package 目录描述。

## 1. 服务初始化矩阵

服务实际运行的必要条件是：

```text
FiberInit 已注册
  + 当前 Process 的 Scene 配置选中了该类型
  + 依赖组件注入成功
  + 监听地址、数据库和远端服务等外部依赖可用
```

仅有 `FiberInit` 注册，不能证明服务已经启动；Room 还属于运行时动态创建的 Fiber，不等同于启动配置中的静态 Scene。

| 服务 | Scene | 入口 | 主要组件 | 外部依赖 |
|---|---|---|---|---|
| Location | `Location` | `module/actorlocation/fiber_init.go` | LocationManager、MailBox、CoroutineLock | 无 |
| Central | `Central` | `module/central/fiber_init.go` | DBManager、LocationProxy、MessageSender、MailBox | MongoDB、Location |
| Realm | `Realm` | `module/login/fiber_init.go` | KCP Net、DBManager、LocationProxy、MailBox | Gate registry、Central/Location |
| Gate | `Gate` | `module/login/fiber_init.go` | KCP Net、Player、GateSessionKey、LocationProxy | Central、Map、Location |
| RouterManager | `Router` | `module/router/fiber_init_router_mgr.go` | HTTP、MessageSender、MailBox | Central、配置地址 |
| RouterNode | `RouterNode` | `module/router/fiber_init_router.go` | RouterComponent、UDP/TCP transport | Machine/Process 配置 |
| Map | `Map` | `module/map_/fiber_init.go` | Unit、AOI、Location、Inventory、RoomManager | Location、DB、Map targets |
| Match | `Match` | `module/lockstep/fiber_init_match.go` | MatchComponent、Location sender | Map registry、Gate |
| Room | `Room` | `module/lockstep/fiber_init_room.go` | RoomServer、LSServerUpdater | Gate、Map、SnapshotProvider |
| HTTP | `HTTP` | `module/http/fiber_init.go` | DBManager、HTTP Component | MongoDB、监听地址 |

FiberInit 是“注册可达”的证据；只有启动配置创建该 Scene，服务才真正运行。

## 2. 登录链路

### 2.1 HTTP 注册

```text
POST /register
  → HttpPostRegisterHandler
  → 校验 username/password
  → accountRepositoryFromScene
  → 查询用户名
  → Central Zone Increment(account_id)
  → 写入 CAccount
  → 返回注册结果
```

仓储依赖：

- 可显式注入 `HTTPAccountRepositoryComponent`；
- 否则需要 Scene 的 `DBManagerComponent`；
- 当前 Zone DB 负责用户集合；
- Central Zone（固定配置 Zone 1）负责全局账号 ID。

缺少 DB 或 Zone 配置直接返回错误，不再切换到内存账号。

### 2.2 HTTP 登录

```text
默认 HTTP Scene `/login`
  → HttpPostLoginHandler
  → accountRepository.FindByUsername
  → central.VerifyPassword(password, password_hash, password_algorithm)
  → 旧 MD5 成功时回写 Argon2id
  → 验证账号
  → login.GenerateAccessToken(accountID)
  → 返回 AccessToken
```

当前安全边界：

- 新密码：Argon2id，随机 salt；
- 旧账号：无 `password_algorithm` 时按 MD5 验证，成功后升级并写入算法字段；
- AccessToken：生产使用配置化 HMAC key ring，Token 内包含 key-id；
- 旧 XOR Token 只有在 `allowLegacyTokens=true` 且显式配置
  `legacyTokenKey` 时才接受；只有 `accessTokenFormat="legacy"` 才生成旧格式；
- HTTP TLS 强制、证书文件热轮换、MongoDB 原子 bucket 登录限流和 Token 撤销
  接口已经实现；Token 已有 MongoDB 共享撤销适配器和启动 wiring；真实
  MongoDB、跨 Process 故障验收、审计保留策略、密钥托管和生产验收仍未完成。

密码算法迁移已落地；CORS 已使用显式 allowlist。HTTP TLS 强制、证书文件热
轮换、MongoDB 原子 bucket 登录限流、Token 撤销接口和 HTTP 审计写入已落地；
MongoDB 真实验收、限流故障策略、密钥托管和审计保留/运营协议仍属于安全 TODO。

### 2.3 Realm 分配 Gate

```text
C2R_Login
  → VerifyAccessToken
  → LocationProxy.Lock(Account, accountID)
  → 解析当前 Zone 的 Gate 列表
  → 选择 Gate
  → MessageSender.Call(Gate, R2G_GateAssign)
  → Gate 生成一次性 Gate Token
  → Realm 返回 Gate 地址、GateID、Token
```

Realm 依赖：

- 当前 Scene 必须有有效 ActorID；
- LocationProxy 必须存在；
- Gate registry 或完整配置/运行时 Gate registry 必须存在；
- Gate endpoint 必须有正 GateID、有效 ActorID 和可连接 Address。

Gate 地址缺失不会变成只返回 host 的半成品 endpoint。

### 2.4 Gate 登录

```text
C2G_LoginGate
  → 校验 Gate Token
  → 校验 GateID 与当前 Scene ID
  → MessageSender.Call(Central, G2Game_Login)
  → Central 返回 PlayerID
  → LocationProxy.Get(Unit, PlayerID)
  → 若 Unit 未注册，Call(Home Map, G2M_EnterMap)
  → 建立 Player、Session、MailBox 绑定
  → 注册 Player/GateSession Location
  → Unlock(Account)
  → MarkAuthed
```

Unit Location 未注册是合法的“尚未进入地图”状态，Location 查询返回零 Actor；但需要发送消息时，`MessageLocationSender` 会拒绝无效 ActorID。

Gate 登录不会：

- 自动创建缺失的 PlayerComponent；
- 自动创建 Central/Map 依赖；
- 猜测第一张 Map；
- 在 GateID 不匹配时继续登录。

Home Map 必须以名称 `Home` 显式注册；如果没有，返回 `ErrMapActorMissing`。

### 2.5 登录阶段和两种 EnterMap 消息

完整阶段关系是：

```text
POST /login
  → AccessToken
  → C2R_Login
  → Gate 地址 + 一次性 GateToken
  → C2G_LoginGate
  → G2M_EnterMap
  → Home Map Unit
```

上图中的 `/login` 特指默认 HTTP Scene 的账号登录接口，不是 RouterManager
提供的同名 `/login` 转发接口。其中：

- `POST /login` 只完成账号密码校验并生成 AccessToken；
- `C2R_Login` 使用 AccessToken 分配 Gate，并返回 GateToken；
- `C2G_LoginGate` 在 Gate 上完成会话绑定；
- `G2M_EnterMap` 是 Gate → Map 的内部消息，用于首次创建/恢复 Home Map Unit；
- `C2M_EnterMap` 是客户端 → Map 的外部协议入口，不能与内部
  `G2M_EnterMap` 按同一方向或同一个 MsgID 理解。

## 3. Central 账号和玩家档案

Central 处理两类调用：

### 3.1 Realm/Router 账号登录

```text
R2CentralAccountLogin
  → DB/注入 AccountStore
  → 查询 username + password_hash
  → 账号不存在返回业务错误
  → GenerateAccessToken
```

`AccountStore` 是显式接口，测试可注入。生产默认要求 DBManager 和 Zone DB。

### 3.2 Gate → Central → Player Profile

```text
G2GameLogin
  → 校验 AccountID
  → PlayerProfileStore（测试/特殊部署显式注入）
     或 DBManager.GetZoneDB
  → 加载/创建 CPlayerProfile
  → 返回 PlayerID
```

持久化失败会返回错误。禁止把 DB 失败转成新的内存 Profile。

## 4. Location 流程

Location 是一个按类型分桶的 ActorID registry：

```text
LocationManager
 ├── Account
 ├── Player
 ├── GateSession
 ├── Unit
 └── ...
```

操作：

- `Add(type,key,actorID)`；
- `Get(type,key)`；
- `Lock(type,key,actorID,timeMs)`；
- `Unlock(type,key,oldActorID,newActorID)`；
- `Remove(type,key)`。

锁定查询不能阻塞 Location Fiber 本身，因此 handler 使用 `TryGet`：

```text
锁存在 → 返回 ErrorCodeLocationLocked
代理端 → 在 context 内短暂重试
锁释放 → 返回位置
```

未分配的 key 可返回零 ActorID；这是明确的“无 owner”状态，不是伪造本地地址。

## 5. Map 进入和切图

### 5.1 进入地图

```text
G2M_EnterMap
  → 校验 PlayerID
  → 要求 UnitComponent
  → CreatePlayer
  → 添加 Move/Numeric/AOI/MailBox
  → InitializeMapUnit
      → 要求 LocationProxy
      → 要求 AOIManager
      → AOI.Enter
      → Location.Add(Unit)
```

初始化任一步骤失败都会移除新 Unit，不保留半初始化对象。

### 5.2 客户端切图

当前实现范围是同 Zone、同 Process 的 Map 之间切图；它是进程内两阶段
迁移，不是跨进程 crash-safe 迁移协议。source 已持久化
`map_transfer_transactions` 的 `Pending → Committed → SourceDisposed`
状态；目标 Map 使用 `TransferLedgerComponent` 以
`RpcID + OldActorID + payload digest` 做进程内幂等去重，并在配置 DBManager
或显式 Store 时持久化 `map_transfer_ledger` 的 processing/completed/failed
状态。`RecoverProcessing` 可在显式协调器证明目标终态后持久化 completed/failed，
但不执行 Unit/AOI/Location 副作用；真实跨进程响应查询、ownership 证明和完整
事务原子性仍未完成。

```text
C2M_TransferMap
  → Gate 根据认证 Session 的 UnitID 定位当前 Unit MailBox
  → Map handler 校验入参 ActorID 与真实 Unit MailBox 完全一致
  → MapUnitManager 显式解析 target
  → 添加 frame-finish task
  → TransferAtFrameFinish
      → Lock(Unit)
      → SerializeUnit
      → SerializeTransferComponents
      → RPC target Map
      → 目标 Deserialize/建立 AOI
      → 目标发送切图通知
      → 目标 Unlock(old,new)
      → 源地图移除并 Dispose
```

迁移状态可以明确表示为：

```text
Prepared
  → TargetRestored
  → OwnershipCommitted
  → SourceDisposed
```

`OwnershipCommitted → SourceDisposed` 之间存在响应丢失和进程崩溃窗口。
当前实现有普通失败路径的回滚、源锁释放、进程内幂等 ledger 和请求指纹冲突
检测，也有 source journal、目标 durable ledger 和
`TransferJournalComponent.Recover` 的严格 scanner 编排；但真实跨进程恢复
token、目标状态查询、Location ownership 证明和协调器注入仍未完成，因此不能
宣称具备崩溃恢复能力。

失败语义：

- 序列化失败：源保留；源侧 defer 会释放本次取得的旧 owner 锁；
- 目标 RPC/响应失败：源保留并释放旧 owner 锁；
- 目标组件未知：目标回滚；
- 目标通知失败：目标回滚，源侧释放旧 owner 锁；
- Location ownership 切换失败：目标回滚，源侧释放旧 owner 锁；
- 只有目标返回成功后，源才销毁。

如果目标已经完成 ownership 切换但成功响应在网络或进程崩溃中丢失，source
journal 和目标 ledger 可以保留未知状态。Go scanner 只有在注入的
`TransferRecoveryCoordinator` 证明目标已提交并提供 token 后，才会执行幂等
source cleanup；没有协调器时记录保持原状态。跨进程 token、目标查询和真实
crash/retry/rollback 演练仍是 TODO，不能宣称具备崩溃恢复能力。

未知迁移组件现在返回 `ErrTransferComponentUnsupported`，不再 silently skip。

`C2M_TransferMap` 不再通过 `firstUnit`、Go map 遍历顺序或 PlayerID
猜测当前玩家。Gate/Actor dispatch 传入的完整 `ActorID` 是路由事实；缺失、
跨 Fiber、跨 Process 或 InstanceID 不一致时，Map 直接返回
`ErrTransferUnitMissing`。地图间内部 transfer request 还要求非零 RpcID
和有效旧 owner ActorID，并在所有业务拒绝响应中保留 RpcID。

source journal 的 `Begin` 也要求有效的 source ActorID；无效地址不能被写入
迁移记录后再等待恢复。Unit snapshot 会校验正 ID、正 ConfigID 和受支持的
UnitType（Player/Monster/NPC），未知身份直接拒绝，避免目标端恢复出无法被
业务识别的 Unit。

目标 durable ledger 的 `processing` 记录只有在目标 ownership、AOI、通知和
成功状态全部完成后才转为 `completed`；完成记录先持久化，再向等待方返回
成功。这样重复请求可以读取已提交结果，但仍不能把它描述成跨进程 crash-safe
恢复：响应丢失后的状态查询、恢复 token 和 scanner 仍属于 TODO。

### 5.3 Map target

`StartSceneConfig` 已提供：

```json
{
  "sceneType": "Map",
  "name": "Map1",
  "mapTargets": ["Map2"],
  "navMeshFile": "maps/map1/navmesh.bin"
}
```

启动时 `cmd/server.configureMapTargets` 把目标 Scene 的 runtime `ActorID`
注入 `MapUnitManagerComponent.Targets`。配置校验要求目标：

- 在同一 Zone；
- `sceneType=Map`；
- 不是自身；
- 与源 Map 位于同一 Process；
- 当前最多一个目标，因为 `C2M_TransferMap` 没有目标名字段。

没有配置 target 时 Map 仍可启动，但切图请求返回 `ErrMapTargetAmbiguous`；
代码不会猜 `Map1 ↔ Map2`。跨进程 peer transport 已经具备 handshake 和
Actor envelope，但 Map target 仍没有跨进程远端注册、负载选择和失效恢复协议；
同进程 Zone/Name 显式解析已经完成，因此跨进程 target discovery 继续保留为
未完成项。

## 6. HTTP/Router 流程

### 6.1 RouterManager

```text
Router Scene
  → ResolveSceneListenAddr(preferInner=false)
  → NewBareHttpComponent
  → 注册 /login、/router/list、/zone/list、/zone/last
  → Start HTTP
```

`/router/list` 只使用有效 Machine IP 和正端口；配置不完整直接返回 handler error。

### 6.2 RouterNode

```text
外部 RouterSYN
  → 解析内部地址
  → 创建 RouterNode
  → 外部 RouterACK
  → 外部 ordinary SYN
  → 内部 SYN（附带外部地址）
  → 内部 ACK
  → 外部 ACK
  → Msg 状态
  → 外部/内部双向转发
```

RouterSYN 是注册握手，不是内部转发消息。普通 SYN/MSG/FIN 才进入
内部 UDP；内部 ACK、RouterReconnectACK、MSG、FIN 再按相反方向返回。
发送失败会让当前动作失败，连接超时会销毁节点。FIN 在 Router 层先转发，
显式清理或超时才销毁 RouterNode。

## 7. Match/Room 流程

```text
C2G_Match
  → Gate 转发 Match
  → MatchComponent.Match
  → 选择已注册 Map Scene
  → RoomManager.CreateRoom
  → 动态创建 Room Fiber
  → RoomServer 初始化玩家
  → 通知 Gate
```

Room 创建不再返回占位的 Map/Room ActorID。缺少 Fiber Manager、Map Scene、RoomServer 或 Scene ActorID 时返回错误。
动态 Room 使用 `fiber.Manager.CreateWithSetup`，在 Fiber loop 启动前注入
真实 Room、玩家和 Map ActorID，避免初始化阶段与 Room 清理并发。
`RoomServerComponent` 注册到 Room Fiber 的 `LateUpdate`；全员离线后先通知
Map，再通过所属 `FiberManager.Remove` 停止并回收 Room Fiber，避免只
Dispose 根 Scene 但把 goroutine 留在 Manager 中。

Match 成功通知依赖 Match Scene 的 Gate sender。sender 缺失或任一玩家通知
发送失败时，Match handler 返回业务错误，不把“房间已创建但客户端未收到
通知”伪装成成功；当前会用有限重试幂等回收已经创建的 Room，并在 Map
回收与 Gate 补偿都确认成功后重新入队。跨进程持久化补偿队列和故障恢复
协议仍需定义。

当前 Room 内部 Actor handlers 已覆盖 Map/Match/Room 之间的消息。匹配成功
通知携带完整三段式 `RoomActorID`，Gate 为玩家创建/更新
`PlayerRoomComponent`；`C2Room_ChangeSceneFinish`、`FrameMessage`、
`C2Room_CheckHash` 由 Gate 按该绑定直接发送到 Room。Gate 覆盖客户端
`PlayerID`，缺少绑定或发送依赖时失败，不按 Unit Location 猜测目标。

## 8. 当前流程验收边界

| 流程 | 进程内替身/测试 | 真实运行 |
|---|---|---|
| Central 账号登录 | 已有 | 2026-08-06 独立全拓扑服务 + Docker MongoDB 已验证 |
| Realm→Gate 分配 | 已有 | 2026-08-06 独立全拓扑服务 + KCP 客户端已验证；双进程未验证 |
| Gate→Central→Map | 已有 alignment 测试 | 2026-08-06 独立全拓扑服务 + KCP 客户端已验证；生产客户端未验证 |
| Location lock/get | 已有 | 2026-08-06 登录、Map Enter/Transfer 全拓扑链路已验证；高并发和故障恢复未压测 |
| Map transfer | 已有迁移测试 | 2026-08-06 Home→Map2、真实切图通知已验证；双进程/崩溃恢复未验证 |
| Router HTTP | 已有 HTTP 测试 | 2026-08-06 独立全拓扑 Router HTTP + UDP/TCP wire 已验证 |
| Match→Room | 已有流程测试 | 2026-08-06 两账号独立 Room、Room Start、FrameMessage 已验证；压力未验证 |
