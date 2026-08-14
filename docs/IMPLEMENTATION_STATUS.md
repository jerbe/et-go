# ET-Go 实现状态

状态基线：`2026-08-06`  
代码仓库：`/mnt/data/github/jerbe/et-go`  
判断方式：源码、初始化注册、配置启用、自动化测试、真实运行证据分开判断。

## 1. 当前结论

ET-Go 已经形成 Go 原生运行链：

```text
cmd/server
  → Config
  → World/FiberManager
  → Configured Fiber
  → Scene/Component/MailBox
  → Actor/Network/Domain flow
```

当前默认 Go 代码可以构建、测试、vet 和 race 检查；但“代码闭环”
不等于“生产部署闭环”。

明确未完成：

1. Go/KCP peer Session 发现、握手、重连和 Session 装配已有自动化闭环；
   真实双 OS 进程、跨机器网络、TLS 和故障演练仍未验收；
2. Map target 已支持配置和同进程 runtime ActorID 注入，跨进程 target
   discovery 未完成；
3. Pathfinding 已有严格 Finder 接口和工厂注册点，但仓库没有真实
   DotRecast/navmesh 实现；
4. Lockstep 已有 Go `Room/FrameBuffer/RoomPlayer/LSWorld` 版本化快照；
   原客户端 MemoryPack/TrueSync wire、客户端恢复和 replay/hash-mismatch
   协议仍没有验收；
5. 密码已使用 Argon2id，旧 MD5 账号首次成功登录时升级；AccessToken
   已支持配置化 HMAC key ring、key-id、轮换和显式旧 XOR 兼容；
   Gate Token 使用 crypto/rand 但保持原 `D5.accountID.D4` Hex wire format；
6. UnitDumper 已在 Unit 所属 Fiber 内先生成不可变 BSON snapshot，先写
   `unit_dump_queue` durable queue，再写业务集合并删除队列记录；下一次
   dump 会重放未完成任务。多组件一致性快照、transfer 事务恢复和 MongoDB
   故障恢复仍未完成；
7. MongoDB、客户端、双进程、压力和故障恢复没有真实验收证据。

## 2. 五级状态

| 状态 | 含义 |
|---|---|
| 源码存在 | 有类型、函数或 handler |
| 已注册 | FiberInit、MailBox、Gate 或 Event 注册可达 |
| 配置启用 | 启动 JSON 能创建并挂载 |
| 自动化测试通过 | 默认测试、替身或进程内流程有证据 |
| 真实运行通过 | 真实端口、DB、客户端、双进程或压力环境通过 |

## 3. 能力矩阵

| 能力 | 源码 | 注册 | 配置 | 自动化测试 | 真实运行 |
|---|---|---|---|---|---|
| ECS/World/Scene/Entity | 是 | 是 | 不适用 | 通过 | 未验证 |
| Fiber/Manager/Task Queue | 是 | 是 | 是 | 通过 | 未验证 |
| Actor/MailBox/RPC | 是 | 是 | 需 Scene | 进程内通过 | 跨进程未验证 |
| Actor envelope 接收 API | 是 | API | 需 peer 装配 | 通过 | 同机 peer 通过，真实部署未验证 |
| Process peer 生命周期 | 是（`engine/network/peer`） | NetInner FiberInit/隐式创建 | `Process.Peers` | handshake/RPC/重连通过 | 双 OS/跨机器未验证 |
| TCP/Session | 是 | 按服务挂载 | 需地址 | 回环通过 | 未验证 |
| KCP | 标准 KCP ARQ + ET 帧 | Realm/Gate | 需地址 | 回环、丢包、多客户端同编号连接通过 | 全拓扑 Realm/Gate KCP 已验证；生产客户端/跨机器未验证 |
| Router UDP/TCP | 是；目标小端方向帧、外部 UDP/TCP 与内部 UDP 已分离 | RouterNode | 需外部/内部地址 | UDP/TCP 回环、wire、状态机通过 | 真实客户端、跨机器、WebSocket 变体未验证 |
| HTTP | 是 | HTTP/Router | 需地址 | httptest + 本机 MongoDB HTTP 部署 + 全拓扑系统测试通过 | 生产 TLS/规模/故障演练未验证 |
| Location | 是 | Location Fiber | Scene 配置 | 端到端测试通过 | 未压测 |
| Central 账号/档案 | 是 | Central Fiber | DB 或显式测试 store | Argon2id/MD5 升级测试通过 | 本机 Docker MongoDB 已验证；生产集群未验证 |
| Realm/Gate 登录 | 是 | Realm/Gate | Location/Central/Map | alignment + 全拓扑系统测试通过 | 本机拟真客户端已验证；生产客户端/跨机器未验证 |
| Map Enter | 是 | Map MailBox | Unit/AOI/Location | 通过 | 本机拟真 KCP 客户端已验证 |
| Map target | 是 | Map + cmd 注入 | `mapTargets` | 配置/装配通过 | 跨进程未验证 |
| Map Transfer | 是 | Gate → Unit MailBox → Map | 同 Zone/同 Process target | 同进程两阶段迁移 + 全拓扑 Home→Map2 通过 | 双进程/崩溃恢复未验证 |
| AOI/StateSync | 是 | Map EventBus/Gate | Map 组件 | 通过 | 全拓扑模拟客户端已验证 CreateUnits/RemoveUnits；生产客户端未联调 |
| Pathfinding | Finder 接口/工厂 | Map 可注入 | `navMeshFile` 可触发启动加载 | 无配置 Finder 时请求失败通过 | 真实 Finder 未完成 |
| Inventory | 是 | Map MailBox/Gate | Unit/Bag | 通过 | 本机拟真 KCP 客户端已验证 |
| Match/Room | 是 | Match/Room Fiber | Map/Fiber Manager | 流程 + 全拓扑系统测试通过 | 当前 `MatchCount=1` 两个独立 Room 已验证；未压测 |
| Lockstep Room snapshot | Go `RoomStateSnapshot v2`，含 `LSWorld`、第 0 帧启动快照 | Room updater | 默认注入 | 世界推进、checksum、初始快照、连续帧、原子恢复测试通过 | 客户端恢复/跨语言 wire 未验证 |
| Lockstep 外部 Room 路由 | Gate Session→PlayerRoom→Room Actor | 已接入 | inner/outer 新增完整 ActorID 字段 | Gate/dispatcher/路由测试通过 | 真实客户端/跨进程未验证 |
| Lockstep LSWorld battle snapshot | Go `LSWorld` + snapshot v2 | Room updater | 默认注入 | 确定性移动、输入校验、世界恢复测试通过 | TrueSync wire/客户端恢复未验证 |
| Crontab | 是 | 手工挂载 | 任务配置未定义 | parser/component 通过 | 未验证 |
| MongoDB migrations | runner + version record（v1/v2/v3/v4/v5/v6）+ MongoDB lease lock | cmd/server 启动调用 | DB Scene 自动执行 | 本机 MongoDB 启动迁移和 integration 通过 | 生产故障切换/恢复未验证 |

## 4. 已完成的严格化

### 启动和配置

- 必需 JSON 文件缺失直接报错；
- 当前 Process 无 Scene 直接报错；
- Scene ID/Name 在 FiberInit 前写入；
- 未知 SceneType 在配置校验阶段直接报错，不创建空白 Fiber；
- Map target 只允许显式配置，不能猜地图；
- 配置了 `navMeshFile` 但未注册 Finder 工厂，或工厂加载资源失败时启动失败；
- 没有 `navMeshFile` 的 Map 可以启动，但寻路请求返回 `ErrFinderMissing`；
- 网络场景缺少 `outerPort` 和 `Process.InnerPort` 时配置校验失败；
- Process peer 缺少远端 ID、地址、secret、目标 Process 或本地 innerPort 时配置校验失败；
- Process 已拥有 NetInner 时，外部网络 Scene 不允许把 `innerPort` 作为
  `outerPort` 兜底，避免启动时端口冲突；
- 网络地址缺失直接报错；
- HTTP/NetComponent 不在 Awake 中吞掉启动错误；
- KCP Connect 不再自动监听 `127.0.0.1:0`。
- AccessToken 未安装 key ring 时直接返回配置错误；旧 XOR 只能通过显式
  legacy 配置或测试初始化启用。

### Fiber/Actor/Network

- `Fiber.Send` 返回关闭/满队列错误；
- `Fiber.WaitFrameFinish` 在 Fiber 未启动、停止或关闭时关闭等待 channel；
- 未启动 Fiber 的 `Stop` 会释放 Scene、关闭 done 信号并禁止后续 `Run`；
- 已取消的排队 `Fiber.Call` 不会执行目标任务；
- Fiber 回调 panic 被隔离，Fiber dispose 统一清理；Call task panic 返回
  `ErrFiberTaskPanic`；
- Update/LateUpdate 系统按实例去重；Crontab 在可用 Fiber 上自动注册并在
  销毁时注销，不会因重复 `Awake` 叠加 tick；
- FiberManager 关闭后拒绝新 Fiber；NetComponent、Session timeout/idle
  组件销毁后不会重新开启；
- 同进程 RPC 经过目标 Fiber task queue；
- 跨进程 envelope 有接收 API；
- Session、ProcessOuterSender、Gate Session dispatcher 传播发送错误；
- Session 发送与关闭共享状态边界，TCP Server 可在 Stop 后重新启动；
- KCP 使用标准 ARQ 状态机，不再使用内存队列伪装可靠 UDP；
- ET KCP 控制帧恢复 localConn/remoteConn 和 little-endian 语义。
- `engine/network/peer` 使用 KCP NetInner、版本化 HMAC handshake、ProcessID
  绑定、`ProcessOuterSender` Session 装配和固定间隔重连；
- `ProcessOuterSender` 注册到 Actor 进程级 registry，Realm/Gate/Map/Match 等
  业务 `MessageSender` 可动态解析跨进程外发器；
- 较小 ProcessID 主动拨号，避免同一 peer 双向建链导致 Session 竞争；
- peer Session 关闭时调用 `ProcessOuterSender.RemoveSession`，pending RPC
  fail-fast；
- peer handshake Request 拒绝零 RpcID；
- `ProcessOuterSender.Call` 使用配置的 RPC timeout，并在 timeout 后清理
  pending RPC；
- Session lifecycle context 已传递到 Gate/Realm/accepted KCP Session 业务调用；
- Router transport 缺失时请求失败，不注册节点或静默修改路由状态；
  RouterSYN/ACK、ordinary SYN/ACK、MSG、FIN、Reconnect 已按目标小端
  方向帧实现，生产 RouterNode 分离外部 UDP/TCP 与内部 UDP；TCP 使用
  little-endian `uint16` body length 并复用已接受连接。

### 登录和数据

- Central 不在 DB 缺失时创建内存 Profile；
- Login 依赖 Location/Central/Home Map 显式存在；
- GateID、Player mailbox、Session 组件类型严格校验；
- Inventory 缺失组件、无效数量、通知失败有错误码；`SortBag` 在整理前校验
  全部物品配置，未知配置返回 `ErrItemConfigNotFound`/`ERR_ItemConfigNotFound`
  且不改变原背包；
- ActorLocation 协议编码器校验消息编号与 Go 业务类型一致，Router 查找不到
  内外目标地址时返回明确错误，StateSync/Inventory 的 Unit 查询不再丢弃
  registry 命中状态；
- Bag/warehouse snapshot 校验 Slot、Item ID、配置和容量，并在跨容器失败
  时回滚；
- DB Save 校验实体 ID，并移除更新 BSON 中的 immutable `_id`；
- DB Increment 按“默认文档 upsert → 独立原子 `$inc`”两步执行，避免
  MongoDB 对同一路径的更新冲突；
- Memory/Mongo Token revocation 会校验 TokenID 与 AccountID 的绑定，拒绝
  跨账号复用；
- Gate Token 生成失败会传播 `ErrGateTokenGeneration`，不会注册空 Token；
- migration runner 在 DB 场景创建业务 Fiber 前执行，当前包含账号唯一索引、
  玩家档案 `(account_id, zone_id)` 唯一索引和旧账号密码算法标记。
- `Config.RuntimeTopology` 在 Fiber 创建前生成确定性启动拓扑摘要，启动日志
  输出 Machine、peer、显式/隐式 NetInner 和当前 Process 的 Scene 清单；
  组件级依赖诊断仍在各 Fiber 初始化错误中逐项暴露。

### Map 和 Location

- Location lock handler 不阻塞 Location Fiber；
- Map Enter 失败会清理半初始化 Unit；
- Map transfer 目标成功前不销毁源 Unit；
- 未知迁移组件返回错误；
- 迁移保留位置和旋转；
- Location ownership old/new Actor 显式切换；
- transfer 源锁在普通失败路径释放。
- `C2M_TransferMap` 只能由 Gate 路由到当前 Unit 的真实 MailBox ActorID；
  Map handler 不再按第一个 Unit 猜测目标，Actor 地址不完全匹配直接失败；
- Location proxy/message sender 在本地拒绝非正 key；
- Unit registry 拒绝重复 Unit，创建/迁移失败时清理新对象。
- transfer journal 要求有效 source ActorID；
- Unit snapshot 拒绝未知 UnitType、非正 UnitID 或非正 ConfigID；
- durable ledger 完成记录持久化后才向等待方返回成功。

### Lockstep

- 动态 Room 创建要求显式 Fiber Manager；
- Room Fiber 缺少显式 Room 定义会失败；
- RoomServer 已注册 LateUpdate；全员离线会通知 Map 并回收所属 Fiber；
- 默认 Room Fiber 注入版本化 `RoomStateSnapshot v2` provider，包含 Go
  `LSWorld`；
- checksum 损坏和版本不支持返回错误；
- 缺少 SnapshotProvider 时不生成固定占位数据；Go LSWorld 已有真实状态模型，
  原 C# TrueSync wire 仍不在当前 snapshot 格式内。
- Room 启动时先生成第 `0` 帧初始快照，成功后才广播 `Room2C_Start`；快照生成
  失败时启动失败，不让客户端进入不可恢复的 Room。
- `FrameBuffer.CheckAndSetHash` 原子记录首个帧哈希，避免无序 Room mailbox
  下并发 CheckHash 请求丢失 mismatch。
- `LSWorld` 按 PlayerID 排序推进，使用原 `TSVector2 * 6 * 50 / 1000`
  移动规则，并在输入未知玩家、世界状态非法或 Room/Updater/World 帧不一致
  时返回错误；
- FrameMessage、CheckHash、Reconnect 会校验 FrameBuffer、Frame、Player 和
  Replay；多个 Map/Match 候选时返回歧义错误，不按 Go map 遍历顺序选择；
- `C2G_Match` 使用认证 Session 绑定的 PlayerID，拒绝请求体伪造其他玩家；
- Match 成功通知缺少 Gate sender 或发送失败时返回错误，不再把未通知的
  匹配当作成功；
- Match 成功通知携带完整三段式 Map/Room ActorID，Gate 写入
  `PlayerRoomComponent` 并转换成外部通知；
- `ChangeSceneFinish`、`FrameMessage`、`CheckHash` 已注册到 Gate，Gate
  只按认证玩家的 Room ActorID 发送并覆盖请求 PlayerID，缺少绑定或发送失败
  时直接报错，不转发到 Unit Location。
- Room snapshot 恢复会先完整校验再提交，关闭 Room Server 不会留下部分状态；
- Replay 保留从 1 开始的连续帧号和空帧，不压缩无输入帧；
- AOI `Enter` 重新接入其他 Cell 时会移动并清理旧 Cell 引用；
- ECS 同类型组件替换会清除旧组件的 Entity 引用；
- AOI manager 销毁时清除实体的 Cell 和可见性引用；
- StateSync 广播错误向 EnterMap/移动流程传播，不再静默成功。
- 已销毁的 AOI、Move、Numeric、Bag、MailBox、CoroutineLock、Crontab、
  MessageLocationSender 等组件不会通过重复 `Awake` 重新打开。

## 5. TODO 清单

### TODO(distributed)

剩余项仅为 Map transfer 的完整可靠恢复：

- source-side `map_transfer_transactions` journal 已实现 Pending →
  Committed → SourceDisposed 状态；
- `TransferJournalComponent.Recover` 已实现严格 scanner 编排：目标状态不确定
  时保留记录，目标 Committed 必须提供 recovery token 并完成幂等 source
  cleanup，失败后才允许进入 SourceDisposed；
- 目标端基于 `RpcID + OldActorID + payload digest` 的进程内幂等去重已完成；
- `TransferLedgerComponent.RecoverProcessing` 已实现 durable processing
  终态编排：只在协调器证明 Committed/Failed 且响应 RpcID、错误语义合法时
  持久化终态，不执行 Unit/AOI/Location 副作用；
  配置 DBManager 或显式 Store 时，`map_transfer_ledger` 会持久化
  processing/completed/failed 状态；
- 跨进程目标状态查询、recovery token 签发和真实 Location ownership 证明；
- 将部署协调器注入 `TransferRecoveryCoordinator` 与
  `TransferLedgerRecoveryCoordinator`；
- 双进程/崩溃故障测试。

peer transport 本身已完成：`StartProcessConfig.Peers`、HMAC handshake、
较小 ProcessID 主动拨号、Session 装配、断线重连和同机 Actor RPC 均已有
实现与测试。Map target 已增加进程内幂等账本、请求指纹冲突检测和可选 durable
ledger；source journal 和 target ledger 都已有可注入协调器的恢复编排，但
跨进程目标状态查询、recovery token、真实 Location ownership 证明和完整
跨组件原子性仍缺失，不能把当前 journal/ledger 组合扩写成 crash-safe 迁移。

### TODO(map)

接入生产导航实现：

- 选择并固定 Go Recast/navmesh 库；
- 实现 `move.FinderFactory`；
- 加载 `StartSceneConfig.NavMeshFile`；
- 按原 C# 约束验证 `DtMeshSetReader.Read32Bit(br, 6)` 资源版本、
  `(-x, y, z)` 坐标转换、查询 extents `(15,10,15)` 和最大多边形数 `256`；
- 实现 nearest-poly、path、closest-point、straight-path 查询；
- 加入真实寻路和寻路失败测试。

原因：原项目使用 DotRecast，当前 Go 仓库没有等价 reader/query 实现；
代码已提供严格工厂接口，缺失时返回错误，不生成直线兜底。

### TODO(lockstep)

Go LSWorld 内部模型、`RoomStateSnapshot v2` 和 Go 内部原子恢复已完成。
当前 TODO 只剩跨端协议与运营验收：

- TrueSync fixed-point wire 编码和客户端恢复协议；
- migration/replay 兼容；
- hash mismatch recovery；
- Map/Match 的 Zone/Name 显式解析已完成；负载选择策略和跨进程注册仍未定义；
- Gate 已部分通知后的 PlayerRoom 绑定持久化补偿事务；
- 动态 Room 压力测试，以及跨进程/故障场景下的回收验收。

原因：Go 侧 `RoomStateSnapshot v2` 已覆盖 Room/FrameBuffer/RoomPlayer/LSWorld，
版本、checksum、连续帧、单位身份和原子恢复已实现；但 Go JSON 与原 C#
MemoryPack/TrueSync fixed-point wire 不是同一格式，客户端状态恢复、Replay
重放和完整 recovery 协议仍未完成。
当前 Match/Map registry 已支持显式 Zone/Name 解析；在仍缺少目标名称或存在
多个候选时继续返回歧义错误，不按 Go map 遍历顺序选择。请求协议和部署拓扑
尚未提供可验证的负载选择、跨进程注册和失效恢复策略。Match Room 已增加
`Match2Map_CancelRoom` 回收消息；Map 回收和 Gate PlayerRoom 补偿发送均使用
有限、幂等的重试，只有补偿全部成功时才把玩家重新放回匹配队列。跨进程
持久化补偿队列和 Gate 已成功部分通知后的故障恢复协议仍未定义，因此不能
宣称跨服务全事务回收完成。

### TODO(security)

- AccessToken MongoDB 共享撤销存储的真实连接、故障恢复和多进程验收；
- TLS 密钥托管、证书文件原子替换约束和生产部署验收；
- MongoDB bucket 登录限流的真实连接、跨 Process 故障验收、审计保留策略和
  真实审计集合验收。

原因：AccessToken 签名、key-id、key ring 轮换和显式 legacy 迁移已经实现；
HTTP CORS 已改为精确 allowlist，HTTP 已支持显式 TLS 证书和
`httpRequireTLS` 强制模式，新 TLS 握手会重新读取证书文件，登录已支持
进程内测试组件和生产 MongoDB 原子 bucket 限流，Token 已提供 MongoDB
共享撤销适配器、TTL migration 和启动 wiring。Memory
revocation store 和进程内限流只用于显式测试/单进程场景；本机 MongoDB
启动迁移、HTTP 注册/登录、审计和限流已验收，生产 MongoDB 验收、
密钥托管、证书文件原子替换约束和审计事件协议仍需要部署策略与持久化实现。

### TODO(persistence)

- UnitDumper 多组件一致性快照；
- Map transfer 跨进程响应查询、recovery token、ownership 证明和完整事务
  原子性；Go source/target recovery 编排已完成，目标协调器仍需部署注入；
- MongoDB 故障恢复和可观测性。

migration runner 已使用 `schema_migration_locks` 的 MongoDB 租约锁、心跳、
上下文取消和 owner 校验协调多个 Process；2026-08-06 已在本机 Docker
MongoDB 上执行 `ETGO_MONGO_URI='mongodb://127.0.0.1:27017' go test
-tags=integration -count=1 ./db/...`，db 和 db/migrations 均通过。UnitDumper
已避免异步 goroutine 读取 live Component，并在关闭
时取消上下文、等待有限时间、执行有限次重试并保留内存 dead-letter；durable
queue 可以在下一次 dump 时重放未完成的单组件写入；Map transfer source
journal 和目标 durable ledger 已记录迁移状态，但没有跨组件事务或 recovery
scanner，因此仍不能保证完整业务状态原子提交。

### TODO(router-transport)

目标 C# Router 在 WebGL 构建使用 WebSocket。Go RouterNode 已实现非 WebGL
所需的外部 UDP/TCP 与内部 UDP：TCP 使用 little-endian `uint16` body length
前缀，按接受到的远端连接复用写回，不在目标未知时隐式主动拨号。

当前未完成项只有 WebSocket：原 C# 只确认 WebGL 客户端在 Router 层选择
WebSocketTransport/WebSocketChannel，Go 仓库没有服务端 listener、传输选择
配置、连接生命周期和客户端版本矩阵。没有这些协议边界，不能把 TCP/UDP
直接标记为 WebSocket 兼容。

## 6. 当前验证证据

截至 `2026-08-06`，当前代码基线已执行并通过：

```bash
go build ./...
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
```

这只证明编译、默认测试、静态检查和竞态检测通过，不代表真实部署闭环。
此外已执行真实全拓扑系统测试：

```bash
go build -o bin/server ./cmd/server
go test -tags=system ./systemtest \
  -run TestSystemFullStackInterfaces -count=1 -v
```

该测试启动独立 `bin/server` 子进程，使用临时端口、唯一 MongoDB 数据库和
真实 KCP/HTTP/Router 客户端，2026-08-06 通过。覆盖 HTTP 正常/错误 Token、
Realm/Gate 正常/失败登录、Map Enter/AOI/Stop、Inventory 查询与操作、
Home→Map2 Transfer、Match/Room、ChangeSceneFinish、FrameMessage、
OneFrameInputs、AdjustUpdateTime、CheckHash mismatch 快照通知和 Router
UDP/TCP RouterSYN/ACK；新增 `TestSystemHTTPAndProtocolBoundaries` 还覆盖
HTTP/Router 方法、Malformed JSON、CORS、Token 重放、非法 Unit/Transfer 和
Router 完整 UDP 转发；`module/router` integration tests 另覆盖普通
SYN/ACK/MSG/FIN 和 Reconnect。不覆盖双 OS Process、生产客户端、压力或
崩溃恢复。
本轮还执行了 `go test -tags=integration -v ./db
-run 'Test(DBComponentCRUDAndIncrement|RunMigrationsSerializesOwners)Integration'`；由于未设置
`ETGO_MONGO_URI`，该测试明确 `SKIP`，不构成 MongoDB 通过证据。

代表性自动化证据包括：

- `engine/network/kcp/TestKCPRetransmitsDroppedSegment`、`TestKServiceLoopbackConnectAndSend`；
- `module/map_/TestHandleUnitTransferAndTransferAtFrameFinish`；
- `module/lockstep/TestMarshalRoomSnapshotIsVersionedAndVerifiable`、
  `TestVerifyRoomSnapshotRejectsCorruption`、`TestHandleMatchCreatesRoom`、
  `TestRoomServerOfflineRoomRemovesOwnedFiber`；
- `cmd/server/TestConfigureMapTargets`；
- `config/TestValidateRejectsNetworkSceneWithoutListenPort`；
- `config/TestValidatePeerTopology`；
- `engine/network/peer/TestProcessPeerHandshakeAndOuterRPC`；
- `engine/network/peer/TestProcessPeerReconnectReplacesSession`；
- `config/TestValidateRejectsUnsupportedSceneType`；
- `db/TestValidateMigrations`、`TestRunMigrationsRequiresExplicitDependencies`、
  `TestAllMigrationsDefinitionIsValid`、`TestNormalizeMigrationOptions`、
  `TestSaveUpdateFieldsExcludesID`；
- `central/TestUniquePlayerProfileRejectsDuplicates`、
  `TestHashAndVerifyArgon2id`、`TestHandleAccountLoginUpgradesLegacyMD5`；
- `engine/network/kcp/TestKServiceListenIsSingleAssignment`；
- `engine/network/kcp/TestKServiceAcceptsSameClientLocalConnFromDifferentAddresses`；
- `module/map_/TestHandleUnitTransferRequiresValidRequestCorrelation`、
  `TestUnitForActorRequiresExactActorAddress`；
- `module/lockstep/TestResolveMapSceneRejectsAmbiguousCandidates`、
  `TestResolveMatchSceneRejectsAmbiguousCandidates`、
  `TestNotifyMatchSuccessRejectsMissingSender`、
  `TestHandleMatchCompensatesAfterPartialGateNotification`、
  `TestGateRoutesRoomMessagesToAuthenticatedRoomActor`、
  `TestGateSessionDispatcherBindsRoomAndEmitsExternalNotification`、
  `TestRoomServerSetPlayerIDsKeepsPlayerStateConsistent`；
- `module/http/TestRegisterHandlerRecognizesWrappedDuplicateError`、
  `module/http/TestApplyCrossDomainHeadersAllowsConfiguredOrigin`、
  `TestApplyCrossDomainHeadersRejectsUnconfiguredOrigin`、`TestDispatcher`、
  `TestHTTPComponentReloadsTLSCertificateForNewConnections`、
  `TestLoginHandlerRecordsAuditEvents`、
  `TestLoginHandlerPropagatesAuditFailure`、
  `TestDBManagerLoginRateLimiterFailsWithoutDatabase`；
  `module/router/TestRouterFrameUsesTargetLittleEndianLayout`、
  `TestRouterForwardFullFlow`、
  `TestTCPTransportUsesTargetOuterLengthPrefixAndRouterWire`；
- `engine/actor/TestResolveSceneActorsSkipsDisposedScene`、
  `module/actorlocation/TestLocationOneTypeRejectsOperationsAfterClose`、
  `module/actorlocation/TestLocationProxyRejectsNegativeKey`、
  `module/actorlocation/TestMessageLocationSenderRejectsNegativeKey`、
  `module/actorlocation/TestMessageLocationSenderComponentDoesNotReopenAfterDestroy`、
  `module/move/TestMoveDestroyCancelsPendingWaiter`、
  `engine/timer/TestTimerWaitAsyncClosesOnDestroy`、
  `engine/fiber/TestFiberSpec_MessageHandlerPanicDoesNotBreakLoop`、
  `engine/fiber/TestFiberSpec_CallConvertsTaskPanicToError`、
  `engine/fiber/TestManagerSpec_RejectsCreateAfterStopAll`、
  `engine/network/TestSessionSendRejectsCloseRaceBoundary`、
  `engine/network/TestSessionAcceptTimeoutDoesNotReopenAfterDestroy`、
  `engine/network/TestSessionIdleCheckerDoesNotReopenAfterDestroy`、
  `engine/network/TestTCPServerCanRestartAfterStop`；
- `module/inventory/TestBagRestorePreservesSlotsAndAdvancesItemID`、
  `TestBagRejectsInvalidSnapshot`、
  `TestBagSortRejectsUnknownConfigWithoutMutation`、
  `module/aoi/TestAOIManagerDestroyClearsEntityState`；
- `module/actorlocation/TestMarshalRequestRejectsMismatchedMessageType`；
- `module/map_/TestUnitDumperPersistsQueueBeforeBusinessDocument`、
  `TestUnitDumperDoesNotWriteBusinessDocumentWhenQueueInsertFails`、
  `TestUnitDumperRecoversPendingQueue`；
- `module/login/TestLoginFlowAlignment`；
- `module/login/TestAccessTokenRevocationStore`、
  `TestNewMongoAccessTokenRevocationStoreRequiresDatabase`、
  `TestDBManagerAccessTokenRevocationStoreFailsExplicitlyWhenManagerClosed`；
- `module/login/TestMemoryAccessTokenRevocationStoreRejectsAccountMismatch`、
  `TestGenerateGateTokenPreservesWireFormat`；
- `engine/fiber/TestFiberSpec_StopBeforeRunDisposesAndCannotRestart`、
  `TestFiberSpec_CanceledCallDoesNotExecuteTask`；
- `engine/actor/TestProcessOuterSenderRemovesPendingCallAfterRPCManagerTimeout`；
- `engine/ecs/TestReplacingComponentClearsPreviousEntityReference`；
- `module/aoi/TestAOIEnterMovesAlreadyRegisteredEntityWithoutLeavingStaleCell`；
- `module/map_/TestTransferJournalRequiresValidSourceActor`、
  `TestTransferJournalRecoverCommittedRequiresTokenAndCleansSource`、
  `TestTransferJournalRecoverPreservesUncertainState`、
  `TestTransferLedgerRecoverProcessingCommitsDurableRecord`、
  `TestTransferLedgerRecoverProcessingPreservesUnknownState`、
  `TestTransferLedgerRecoverProcessingRejectsMismatchedResponse`、
  `TestDeserializeUnitRejectsUnknownIdentity`；
- `module/lockstep/TestLSWorldMovementIsDeterministicAcrossPlayerOrder`、
  `TestLSWorldRejectsUnknownInputWithoutPartialMutation`、
  `TestLSWorldRestoreStateIsAtomic`、
  `TestLSWorldUpdateRejectsInvalidUnitWithoutPartialMovement`、
  `TestLSServerUpdaterAdvancesLSWorld`；
- `module/lockstep/TestRestoreRoomSnapshotDoesNotPartiallyMutateClosedServer`、
  `TestRoomSnapshotPreservesEmptyFramesForReplayAlignment`；
- `config/TestLoadAndValidateSecurityConfig`、
  `config/TestValidateRejectsWildcardCORSOrigin`、
  `config/TestRuntimeTopologyIsDeterministicAndReportsImplicitNetInner`、
  `cmd/server/TestConfigureAccessTokenInstallsSignedKeyRing`、
  `cmd/server/TestCreateConfiguredHTTPFiberInstallsSharedSecurityComponents`、
  `module/login/TestAccessTokenRequiresExplicitConfiguration`。

这些测试名称用于说明当前 Go 代码的复核入口；它们仍不等于生产规模
MongoDB、客户端、双进程和压力验收。

真实验收仍需：

- 生产规模 MongoDB；
- 启动 JSON；
- 至少两个 OS Process；
- 与原协议一致的 KCP/客户端；
- 测试客户端；
- 可观测日志和端口检查；
- 丢包、重连、进程崩溃恢复场景。
