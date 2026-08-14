# ET-9.0-Server Go 项目分析文档

> 本文是 `/mnt/data/github/jerbe/et-go` 的 Go 版本总览。内容按 Go 的可执行程序、运行时、消息、服务流程、领域状态和数据边界组织，不按原 ET/C# package 拆分。  
> 详细章节见 [`docs/README.md`](docs/README.md)。

分析日期：`2026-08-06`  
Go module：`github.com/jerbe/et-go`  
Go 版本：`go 1.25.7`  
默认入口：`cmd/server`

## 1. 结论

ET-Go 是一个单仓库、多 Fiber 的 Go 游戏服务端工程：

```text
cmd/server
  → 读取 Machine/Process/Scene/Zone 配置
  → 创建 World 和 FiberManager
  → 按配置创建 Scene/Fiber
  → 挂载服务组件和 MailBox
  → 通过 Actor/Network/Location 连接业务流程
```

项目当前已经形成以下运行基础：

- ECS：`World`、`Scene`、`Entity`、`Component`；
- Fiber：goroutine、mailbox、task queue、frame update；
- Actor：`ActorID`、`MailBox`、`MessageSender`、RPC；
- 网络：codec Packet、Session、TCP、KCP、Router UDP/TCP；
- 服务：Location、Central、Realm、Gate、HTTP、Router；
- 地图：Map、Unit、AOI、Move、StateSync；
- 领域：Inventory、Numeric、Match、Room、Lockstep、Crontab；
- 数据：MongoDB、BSON、DBManager、迁移函数；
- 协议：protobuf 源文件和生成 Go 类型。

近期严格化已覆盖 Fiber 回调 panic 隔离、Manager 关闭后的创建边界、
未启动 Fiber 的不可重启 Stop、取消 Call 不执行任务、Update/LateUpdate
注册去重、Crontab 生命周期、Location 非正 key、AOI 重入和销毁清理、
ECS 组件替换生命周期、Inventory snapshot 校验和 `SortBag` 未知配置保护、
ActorLocation 协议类型校验、Router 目标地址校验、DB Save 的实体 ID/`_id`
边界、MongoDB Increment 两步原子语义，以及玩家档案唯一索引迁移和重复
档案拒绝。

项目还不能标记为“生产完整游戏服”，因为以下能力仍缺少真实运行闭环：

1. peer Session、握手、重连和 Actor RPC 已有 Go/KCP 自动化闭环，但没有真实双 OS
   进程、跨机器和客户端验收证据；
2. Map target 已支持 `mapTargets` 配置和同进程 runtime ActorID 注入，但跨进程
   target discovery 尚未完成；
3. Pathfinding 只有严格 Finder 接口/工厂注册点，仓库没有生产 navmesh 实现；
4. Lockstep 已有 Go `LSWorld` 确定性状态模型和 `RoomStateSnapshot v2`；
   该快照覆盖 Go 侧单位位置、旋转、输入、世界帧和 ID watermark，但尚未
   证明与原 C# MemoryPack/TrueSync 二进制或客户端恢复协议 wire 兼容；
5. AccessToken 默认使用配置化 HMAC key ring、key-id 和轮换；只有显式
   `accessTokenFormat="legacy"` 才生成原 ET SimpleToken，未安装安全配置时
   直接报错，不隐式回落；HTTP TLS 强制、MongoDB 原子 bucket 登录限流和
   Token 撤销接口
   已实现；新 TLS 握手支持证书文件热轮换，Token 已接入 MongoDB 共享撤销
   wiring；MongoDB 原子 bucket 登录限流和 HTTP 登录审计写入已接入；真实
   MongoDB、限流故障策略、密钥托管和审计运营协议仍需部署验收；密码已切换
   到 Argon2id，新账号使用 Argon2id，旧 MD5 账号首次成功登录时升级；
6. DB migration runner 已纳入启动，包含账号/玩家唯一索引、密码算法标记和
   AccessToken 撤销 TTL index，并使用 MongoDB lease lock 协调多进程；
   UnitDumper 已实现 durable queue：先持久化不可变快照，再写业务集合，成功
   后删除队列记录，下一次 dump 会重放未删除任务；跨组件一致性和 transfer
   级别的崩溃恢复仍未完成；
7. `Config.RuntimeTopology` 已在 Fiber 创建前输出当前 Process 的 Machine、
   peer、显式/隐式 NetInner 和 Scene 拓扑摘要；组件级依赖诊断仍需继续细化；
8. MongoDB 生产规模、双进程、跨机器和压力环境没有验收证据；本机
   全拓扑拟真客户端验收已通过。

2026-08-06 已增加并通过本机全拓扑拟真验收：测试启动独立
`bin/server` 子进程，使用临时端口、唯一 MongoDB 数据库、真实
HTTP/KCP/Router 客户端和 protobuf payload。该证据证明本机单进程全拓扑
可运行，但不扩大为生产客户端、双 OS Process、跨机器、压力或崩溃恢复通过。

未完成项没有被伪装成成功。代码在可定位的实现边界使用 `TODO(scope)`，
文档统一使用 `TODO(distributed)`、`TODO(map)`、`TODO(lockstep)`、
`TODO(security)`、`TODO(persistence)`、`TODO(router-transport)` 标出原因；
peer 生命周期本身不再属于 TODO，剩余 distributed TODO 仅指 Map transfer 的
幂等和崩溃恢复。

## 2. Go 工程边界

```text
cmd/server/
  可执行入口、配置、进程生命周期

config/
  启动 JSON、拓扑校验、地址解析数据

engine/
  ECS、Fiber、Actor、RPC、基础网络、锁、Timer、数学

engine/network/peer/
  NetInner KCP peer、握手、Session 生命周期、ProcessOuterSender 装配

module/
  服务能力和游戏领域实现

db/
  MongoDB Client、DBManager、DBComponent、持久化模型

proto/
  .proto 源文件和生成 Go 类型

internal/
  仓库内部日志等工具
```

依赖方向：

```text
cmd → config + engine + module
module → engine + db + proto
db → config + engine/ecs
proto → protobuf runtime
engine 不依赖 module
```

目录名只是实现边界。真正的运行边界是当前进程中的 Scene/Fiber。

## 3. 启动和生命周期

### 3.1 启动顺序

```text
flag.Parse
  → config.Load
  → Config.Validate
  → DB migrations（DB 场景）
  → config.SetGlobal
  → ecs.NewWorld
  → fiber.NewManager
  → createConfiguredFibers
      → 有 peer 配置且未显式声明 NetInner 时创建隐式 NetInner
      → parseSceneType
      → CreateConfigured
          → 设置 Scene ID/Name
          → FiberInit
          → 注册 Manager
          → 启动 loop
  → 注入 Map target runtime ActorID
  → 等待信号
  → StopAll
  → World.Shutdown
```

`Config.Load` 必须找到：

- `startmachineconfig.json`；
- `startprocessconfig.json`；
- `startsceneconfig.json`；
- `startzoneconfig.json`。

使用 AccessToken 的 Process 还必须提供可选文件
`startsecurityconfig.json` 中的签名 key ring；HTTP Process 同时必须配置
精确 CORS allowlist。其他不使用登录 Token 的 Process 可以省略该文件。

配置有错误时启动失败。不会创建空配置运行时，也不会把缺少地址转为 `127.0.0.1:0`。
Realm、Gate、HTTP、Router、RouterNode 缺少 `outerPort` 和
`Process.InnerPort` 时会在 `Config.Validate` 阶段直接失败。

仓库已提供本地可运行的 HTTP 配置：`data/config/json/`。该配置使用
`127.0.0.1:18080`、MongoDB `etgo_dev` 和开发密钥。配合 MongoDB 可以直接执行：

```bash
docker run -d --name et-go-mongodb \
  -p 127.0.0.1:27017:27017 \
  -e MONGO_INITDB_DATABASE=etgo_dev \
  -v et-go-mongodb-data:/data/db \
  mongo:7.0

go build -o bin/server ./cmd/server
./bin/server --process=1 --config=data/config/json --log-level=info
```

2026-08-06 已验证 migration 1–6、HTTP 监听、`/register`、`/login`、
错误密码、重复注册、CORS、登录审计和 MongoDB 固定窗口限流。该配置只用于
本地验收，不能直接作为生产配置。

### 3.2 Fiber/Scene

每个 Fiber 有：

- 一个 Root Scene；
- 一个消息邮箱；
- 一个 task queue；
- Update/LateUpdate 系统；
- frame-finish task；
- context 和停止信号。

同一 Scene 的业务状态由所属 Fiber 串行拥有。跨 Fiber 调用必须使用 `Send` 或 `Call`。
Actor 不是与 Fiber 平行的另一种 goroutine；Actor 是 `ActorID`、MailBox
和消息路由边界，状态仍由所属 Scene/Fiber 拥有。
Fiber Stop 是单向生命周期：即使 Fiber 尚未 Run，也会释放 Scene 并禁止再次
启动；已经排队但 context 取消的 Call 不执行目标业务任务。

## 4. Actor 和网络

### 4.1 ActorID

```go
type ActorID struct {
    ProcessID  int
    FiberID    int64
    InstanceID int64
}
```

- ProcessID 是逻辑进程；
- FiberID 是执行上下文；
- InstanceID 是 runtime Entity；
- `StartSceneConfig.ID` 不是 InstanceID。

### 4.2 同进程消息

```text
MessageSender
  → ProcessInnerSender
  → FiberManager.Get
  → Fiber.Send/Fiber.Call
  → DispatchFiberMessage
  → MailBox
```

Fiber mailbox 满、Fiber 关闭、Actor 不存在都返回错误。

### 4.3 跨进程消息

```text
MessageSender
  → ProcessOuterSender
  → PacketSession
  → codec.Packet
  → 20 bytes Actor envelope
```

`ProcessOuterSender.HandlePacket` 已提供接收端：

- Message 解包后进入目标 Fiber；
- Request 通过目标 Fiber Call 生成 Response；
- Response 进入 RpcManager。

`engine/network/peer.ProcessPeerComponent` 已完成 Go 运行时编排：

- 从 `StartProcessConfig.Peers` 读取远端地址和共享 secret；
- NetInner 使用 KCP 监听，连接方向固定为较小 `ProcessID` 主动拨号；
- handshake 携带协议版本、发送方/目标 ProcessID、随机 nonce 和 HMAC-SHA256；
- 验证通过后调用 `ProcessOuterSender.AddSession`；
- 收包进入 `HandleSessionPacket`，Response 保留 RpcID；
- Session 关闭时调用 `RemoveSession`，pending RPC 立即返回
  `actor.ErrProcessSessionClosed`；
- 使用固定重连间隔重新建立 Session；
- `cmd/server` 在有 peer 配置且没有显式 NetInner Scene 时自动创建 NetInner Fiber。

当前边界是“自动化端到端已完成、真实部署未验收”：双 Process KCP/RPC 测试已经
通过，但真实双进程、跨机器地址、TLS、监控和故障演练仍未执行。

### 4.4 Session/KCP/Router

Session 有：

- ReadLoop；
- WriteLoop；
- 发送队列；
- RPC callback；
- 认证超时；
- 空闲检测。

`Session.Send` 返回队列满和关闭错误。发送入队和关闭共享同一状态边界，
关闭竞争不会把消息继续写入已关闭 Session。TCP Server 停止后清理
listener/context，可在新的 context 下重新启动。

KCP 使用标准 ARQ 状态机，并保留原 ET 控制帧语义：

```text
SYN/ACK/Router: Proto(1) + LocalConn(4) + RemoteConn(4)
FIN:             Proto(1) + LocalConn(4) + RemoteConn(4) + Error(4)
MSG:             Proto(1) + LocalConn(4) + Standard KCP Segment
```

Router 不是统一 13 字节大端帧，而是按方向使用原 ET C# little-endian
控制帧：

```text
普通 SYN/MSG：Proto(1) + ConnA(4) + ConnB(4) + Payload
RouterSYN：   Proto(1) + Outer(4) + Inner(4) + ConnectID(4) + InnerAddress
ACK：         Proto(1) + Inner(4) + Outer(4)
FIN：         Proto(1) + ConnA(4) + ConnB(4) + Error(4)
```

`RouterSYN → RouterACK` 只完成外部 RouterNode 注册；普通 SYN/ACK、
MSG、FIN 才在外部 UDP/TCP 与内部 UDP 之间转发。生产 RouterNode 分别创建
外部 UDP、外部 TCP 和绑定 `innerIP` 的内部 UDP，不能把测试中的共享 socket
当作部署拓扑。外部 TCP 使用 little-endian `uint16` body length，并沿
RouterSYN 建立的已接受连接回写。监听地址必须显式提供。测试可以主动使用
端口 `0`，生产配置不能依赖临时端口。

## 5. 服务流程

### 5.1 Location

Location 按类型保存：

```text
Account / Player / GateSession / Unit / ...
```

支持 Add/Get/Lock/Unlock/Remove。

Location handler 使用非阻塞 `TryGet`，避免在 Location Fiber 内等待自身锁。代理端在锁竞争时用 context 重试。

未分配 key 可以返回零 ActorID，表示“没有 owner”；`MessageLocationSender` 会拒绝无效 ActorID。
Location Manager/OneType 关闭后不会重新创建内部状态，后续操作返回
`actorlocation.ErrLocationClosed`。

### 5.2 HTTP 注册和登录

```text
POST /register
  → HTTP Repository
  → 查询用户名
  → Central Zone Increment
  → 写入 CAccount

POST /login
  → 查询账号
  → VerifyPassword(password, password_hash, password_algorithm)
  → 旧 MD5 成功时生成 Argon2id 并回写
  → GenerateAccessToken
```

账号文档新增 `password_algorithm`。新注册账号保存 Argon2id；没有算法字段
的旧账号按 MD5 验证，密码正确后立即升级为 Argon2id。AccessToken 默认使用
`v2.<key-id>.<payload>.<signature>` 的 HMAC-SHA256 格式；旧 XOR Token 仅在
显式开启 legacy 配置并提供 `legacyTokenKey` 时接受。

### 5.3 Realm/Gate/Central/Map 登录

```text
C2R_Login
  → VerifyAccessToken
  → Lock(Account)
  → 选择有效 Gate
  → GateAssign
  → 返回 GateID/Address/GateToken

C2G_LoginGate
  → Verify GateToken
  → 校验 GateID
  → Central G2Game_Login
  → 查询/创建 PlayerProfile
  → 查询 Unit Location
  → Home Map G2M_EnterMap
  → 建立 Player/Session/MailBox
  → 注册 Player/GateSession Location
  → Unlock Account
```

Home Map 必须显式命名为 `Home`。缺少 Central、Location、Map、PlayerComponent 或 MailBox 都返回错误。
Realm 生成的 Gate Token 使用 crypto/rand，但保持原 C# 的
`D5.accountID.D4` UTF-8 Hex wire format；随机源失败会阻断登录，不注册空
Token。

### 5.4 Map Enter/Transfer

EnterMap：

```text
CreatePlayer
  → Move/Numeric/AOI/MailBox
  → InitializeMapUnit
  → AOI.Enter
  → Location.Add(Unit)
```

Transfer：

```text
Lock(Unit)
  → Serialize Unit/Components
  → RPC target Map
  → target restore
  → target AOI/notification
  → target Location Unlock(old,new)
  → source Dispose
```

迁移状态是：

```text
Prepared
  → TargetRestored
  → OwnershipCommitted
  → SourceDisposed
```

source 已持久化 `map_transfer_transactions`，记录 `Pending → Committed →
SourceDisposed` 状态；目标端使用 `RpcID + OldActorID + payload digest`
进行进程内幂等，并在配置 DBManager 或显式 Store 时持久化
`map_transfer_ledger` 的 processing/completed/failed 状态；source journal 和
target ledger 都提供显式 recovery coordinator 的状态收敛编排。
`OwnershipCommitted → SourceDisposed` 之间仍存在响应丢失和进程崩溃窗口，需要
外部协调器提供跨进程查询、recovery token、terminal response 和 ownership
证明；Go scanner 编排不能替代这些协议，UnitDumper 的 durable queue 也不能替代。

source journal 的 Begin 要求有效 source ActorID；Unit snapshot 拒绝非正
ID/ConfigID 和未知 UnitType。目标 durable ledger 先持久化 completed 记录，
再向等待方返回成功；source journal 已提供显式
`TransferRecoveryCoordinator` scanner 编排，但跨进程状态查询、恢复 token、
Location ownership 证明和真实 scanner 注入尚未完成。

目标失败时源 Unit 保留。未知迁移组件、重复 Unit、Location 切换失败均是错误。

客户端 `C2M_TransferMap` 必须由 Gate 根据认证 Session 的 UnitID 路由到
当前 Unit MailBox。Map handler 会校验完整 ActorID 的 ProcessID、FiberID、
InstanceID，不按 Map 中第一个 Unit 或仅按 PlayerID 猜测目标；地图间
transfer request 要求非零 RpcID 和有效旧 owner ActorID。

### 5.5 Match/Room/Lockstep

```text
C2G_Match
  → MatchComponent
  → 已注册 Map
  → RoomManager.CreateRoom
  → 动态 Room Fiber
  → RoomServer
  → Match success notification
```

Room 不再返回没有实际 Fiber 的占位 ActorID。

帧同步按固定间隔推进 `FrameBuffer`、`LSWorld` 和 `Replay`。默认 Room Fiber
注入版本化 `RoomStateSnapshot v2` provider，包含 Room/玩家/连续帧输入、
`LSWorld` 单位状态和 SHA-256；缺少 provider 时不生成固定字节。房间启动时先
生成第 `0` 帧初始快照，成功后才广播 `Room2C_Start`；运行中每 1200 帧生成
一次快照，存入 FrameBuffer 和 Replay；`VerifyRoomSnapshot` 和
`RestoreRoomSnapshot` 会严格校验 Room/FrameBuffer/RoomPlayer/LSWorld 的帧、
玩家、单位身份和 checksum，并在校验完成后提交恢复。Reconnect 使用当前
世界位置/旋转、最近快照及 Replay 尾部，Hash mismatch handler 仍只负责服务端
选择恢复起点并构造失败响应；客户端状态恢复、Replay 重放和完整 hash mismatch
recovery 仍未完成或验收。当前快照是 Go 内部状态格式，不应宣称为原 C# 客户端
MemoryPack/TrueSync wire。
服务端恢复锚点允许为第 `0` 帧，但外部客户端输入仍从第 `1` 帧开始；后续
快照帧号连续保留，空输入帧不从 Replay 中压缩。`CheckHash` 使用
`FrameBuffer.CheckAndSetHash` 原子记录首次哈希，适配无序 Room mailbox 的
并发请求。恢复先完整校验再提交，不会在关闭 Room Server 上留下部分恢复状态。

`RoomServerComponent` 已注册 Room Fiber 的 `LateUpdate`。全员离线后先通知
Map，再通过所属 `FiberManager` 停止并移除动态 Room Fiber；不存在 Manager
的独立测试 Fiber 只发出非阻塞停止请求。

外部 `C2Room_ChangeSceneFinish`、`FrameMessage`、`C2Room_CheckHash` 已接入
Gate。Match 成功通知携带完整三段式 Room ActorID，Gate 写入玩家的
`PlayerRoomComponent`；后续客户端 Room 消息只允许路由到该绑定的
`RoomActorID`，并在 Gate 覆盖客户端提交的 `PlayerId`。缺少绑定、ActorID
不完整或跨 Actor 发送失败都会返回错误，不按 Unit Location 兜底。通知失败
时会有限重试幂等回收 Map Room 和 Gate PlayerRoom 补偿；只有补偿全部成功
才重新放回匹配队列，跨进程持久化补偿仍是 TODO。

## 6. 领域状态

### Unit/AOI

Unit 是 Map 领域对象。AOI 通过 Cell 管理进入、离开和移动事件，StateSync 根据
AOI 事件发送 CreateUnits/RemoveUnits。实体重复 Enter 到同一 Cell 时只更新
位置；从其他 Cell 重新接入时先 Move，避免旧 Cell 残留引用。

### Move/Pathfinding

MoveComponent 只接受有效路径和正速度；销毁时会取消进行中的移动并释放
等待 channel。PathfindingComponent 要求真实 Finder：

```text
没有配置 navMeshFile 的 Map 没有 Finder → ErrFinderMissing
路径少于两个点 → ErrPathfindingFailed
```

配置了 `navMeshFile` 的 Map 会在 Fiber 初始化时调用部署层注册的
`move.FinderFactory`；工厂未注册或资源加载失败会直接阻止启动。仓库没有
内置 DotRecast 等生产实现。原 C# 读取 Recast mesh 使用
`DtMeshSetReader.Read32Bit(br, 6)`，坐标按 `(-x, y, z)` 转换，查询 extents
为 `(15,10,15)`、最大路径多边形数为 `256`，并执行 nearest-poly、path、
closest-point、straight-path；Go 尚未有等价 reader/query 实现。未配置导航
网格时才会在请求阶段返回 `ErrFinderMissing`。这属于 `TODO(map)`，不能生成
直线兜底。

### Inventory

Bag/ Warehouse 使用 BSON snapshot。操作前校验 Unit、组件、数量和容器类型；通知失败返回独立错误码。
物品配置必须先注册；未知物品配置不会被当作容量为 1 的合法物品。

### Crontab

CrontabComponent 支持五段 cron，并记录 handler 缺失、任务重复、任务 panic。当前没有默认业务任务。

HTTP handler 对 nil request/response、缺少全局配置和空账号凭证返回明确
错误；Router transport 缺失时不注册路由节点、不改变节点状态并返回失败。

## 7. 数据和协议

### MongoDB

DBManager 按 Zone 缓存 DBComponent。DBComponent 提供 FindOne/Query/Insert/Save/Remove/Increment。

DB 连接、Zone 配置、Collection 缺失都返回错误。玩家档案不会在生产 DB 失败时降级为内存数据。

`Increment` 先以 `$setOnInsert` upsert 默认文档，再执行独立原子 `$inc` 返回
结果；不在同一 upsert 中对同一路径同时使用 `$setOnInsert` 和 `$inc`，避免
MongoDB 更新路径冲突。

同一账号和 Zone 查询到多条玩家档案时直接返回
`ErrPlayerProfileDuplicate`，不会任意取第一条；唯一索引只阻止新增重复，
历史数据仍需迁移清理。

`cmd/server` 在创建 DB 场景 Fiber 前执行 `db.RunMigrations`。当前已注册
`account_username_unique_index`（版本 1）和
`player_profile_account_zone_unique_index`（版本 2），以及
`account_password_algorithm_marker`（版本 3）；migration 失败会
阻止启动。Runner 会在版本检查和 `Up` 前获取 MongoDB 租约锁，按心跳续租；
上下文取消、租约丢失或释放失败都会返回错误。2026-08-06 已在本机 Docker
MongoDB 上执行 `ETGO_MONGO_URI='mongodb://127.0.0.1:27017' go test
-tags=integration -count=1 ./db/...`，`db` 和 `db/migrations` 均通过。

### Protobuf

`.proto → generated pb.go → proto_codec.go → handler` 是完整边界。修改字段必须同步 wire、业务结构、测试和客户端矩阵。

### 错误传播

项目统一区分：

1. Go error：依赖、网络、序列化、Actor 失败；
2. Protocol Error/Message：业务拒绝；
3. 异步错误：日志、状态或消息报告。

禁止：

- `return nil, nil` 表示缺依赖成功；
- nil 业务消息编码成空 payload；
- 未知组件 `continue`；
- 网络发送 `_ =`；
- RPC 失败后销毁源 Unit；
- 快照缺失时返回占位字节；
- KCP 用普通发送队列伪装可靠 UDP。

## 8. 当前验证和未完成项

已验证：

- `go build ./...`；
- `go test -count=1 ./...`；
- `go vet ./...`；
- `go test -race -count=1 ./...`；
- Fiber/Actor/Session/KCP/Router/Location/Timer/Move/登录/Map/Inventory/Lockstep
  的默认测试，含 Router UDP/TCP wire、UnitDumper durable queue 和 Match
  补偿路径；
- Fiber Stop/取消 Call、Actor RPC timeout pending 清理、ECS 组件替换、AOI
  重入、Map source ActorID/Unit snapshot 严格校验、Lockstep 原子恢复/空帧、
  Token revocation AccountID 校验和 Gate Token wire format 回归；
- MongoDB Increment 两步语义、迁移租约锁和完整 migration 定义的 integration
  测试；
- 登录对齐和 Map transfer 流程测试。
- 全拓扑拟真系统测试：

  ```bash
  go build -o bin/server ./cmd/server
  go test -tags=system ./systemtest \
    -run TestSystemFullStackInterfaces -count=1 -v
  ```

  2026-08-06 通过，覆盖 HTTP 注册/登录及错误 Token、Realm/Gate KCP 正常与
  失败登录、Map Enter/AOI/Stop、Inventory 查询与操作、Home→Map2 Transfer、
  Match/Room、ChangeSceneFinish、FrameMessage、OneFrameInputs、
  AdjustUpdateTime、CheckHash 路由和 Router UDP/TCP RouterSYN/ACK；
  `module/router` integration tests 另覆盖普通 SYN/ACK/MSG/FIN 和
  Reconnect。不覆盖双 OS Process、生产客户端、压力或崩溃恢复。

- 新增全边界拟真测试：

  ```bash
  go test -tags=system ./systemtest \
    -run TestSystemHTTPAndProtocolBoundaries -count=1 -v
  ```

  覆盖 HTTP/Router 方法和 malformed JSON、CORS、Token 重放、非法
  Unit/Transfer、Router UDP 完整转发及 Lockstep 初始快照恢复边界。

仍未验证：

- 生产规模 MongoDB、故障切换和数据恢复；
- 真实双 OS 进程 Actor RPC（当前已有同机 KCP 双组件集成测试）；
- 生产客户端完整登录、进入地图、切图（拟真测试客户端已覆盖上述流程）；
- 原客户端 KCP/Router 真实网络；
- 原 C# TrueSync wire 与客户端 LSWorld snapshot/reconnect；
- 压测和故障恢复。

本轮还在本机 Docker MongoDB 上执行：

```bash
ETGO_MONGO_URI='mongodb://127.0.0.1:27017' \
  go test -tags=integration -count=1 ./db/...
```

`db` 和 `db/migrations` 均通过，并使用默认本地配置完成了真实 HTTP
注册、登录、错误密码、重复注册、CORS、审计和限流链路。生产规模 MongoDB、
故障切换和数据恢复仍未验收。

## 9. 推荐阅读

- [项目总览](docs/01-project-overview.md)
- [Go 包架构](docs/02-go-package-architecture.md)
- [运行时与并发](docs/03-runtime-and-concurrency.md)
- [消息与网络](docs/04-messaging-and-network.md)
- [服务流程](docs/05-service-workflows.md)
- [游戏领域](docs/06-game-domain.md)
- [数据与配置](docs/07-data-persistence-and-configuration.md)
- [协议参考](docs/08-protocol-and-api-reference.md)
- [测试与开发](docs/09-testing-and-development.md)
- [Go 模块任务与兼容审计](docs/modules/README.md)
- [实现状态](docs/IMPLEMENTATION_STATUS.md)
- [路线图](docs/11-roadmap.md)
