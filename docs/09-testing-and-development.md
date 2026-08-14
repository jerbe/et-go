# 测试、开发与验收

## 1. 本地命令

在仓库根目录执行：

```bash
# 编译全部包
go build ./...

# 默认测试
go test -count=1 ./...

# 静态检查
go vet ./...

# 竞态检测
go test -race -count=1 ./...

# 查看所有包
go list ./...

# 生成覆盖率
go test -coverprofile=/tmp/et-go.coverage ./...
go tool cover -func=/tmp/et-go.coverage
```

`-count=1` 用于避免测试缓存掩盖改动。

## 2. 测试层次

### 2.1 纯单元测试

不依赖外部端口或 MongoDB：

- ECS Entity/Component；
- Fiber Send/Call/Stop；
- 未启动 Fiber 的 Stop 会释放 Scene 且禁止重启；
- 已取消的排队 Call 不执行目标 handler；
- ActorID、RpcManager、envelope；
- codec；
- KCP 协议状态机；
- AOI Cell；
- Numeric；
- Move 插值；
- Cron parser；
- Inventory Bag；
- Inventory `SortBag` 对未知配置的严格失败和原状态保持；
- FrameBuffer/Replay。

### 2.2 组件集成测试

在进程内创建多个 Fiber/Scene：

- Location proxy → Location Fiber；
- Realm/Gate/Central/Map 登录对齐；
- Map Unit 创建和迁移；
- Match → Map → Room；
- Router UDP/TCP frame；
- HTTP Dispatcher + httptest。

这类测试证明同进程消息、依赖注入和生命周期，不能证明 OS 进程间网络。

### 2.3 外部依赖测试

MongoDB integration test 使用构建标签 `integration`，当前测试读取
`ETGO_MONGO_URI`，并固定使用隔离数据库名 `etgo_test`。执行：

```bash
ETGO_MONGO_URI='mongodb://127.0.0.1:27017' \
  go test -tags=integration ./db \
    -run 'Test(DBComponentCRUDAndIncrement|RunMigrationsSerializesOwners)Integration'
```

测试会清理 `account` 和 `increment` collection；不要指向生产数据库。
migration integration 包含两个 Client 并发执行同一版本 migration 的租约锁
测试；这些测试不会由普通 `go test ./...` 默认启用，也不能把跳过结果
写成 MongoDB 真实通过证据。

没有 MongoDB 时不能把 integration 失败写成代码失败，也不能把它跳过后写成真实数据库通过。
如果未设置 `ETGO_MONGO_URI`，integration test 只得到明确 `SKIP`，不能
写成 MongoDB 通过证据。

2026-08-06 已在本机 Docker MongoDB 上执行：

```bash
ETGO_MONGO_URI='mongodb://127.0.0.1:27017' \
  go test -tags=integration -count=1 ./db/...
```

结果为 `db` 和 `db/migrations` 均通过；同时使用
`data/config/json` 启动真实 HTTP Process，迁移 1–6、`/register`、`/login`、
错误密码、重复注册、CORS 白名单和 MongoDB 固定窗口限流均已完成验证。

### 2.4 真实流程测试

仓库已提供可重复的全拓扑拟真测试 `systemtest/blackbox_test.go`，使用
真实 `bin/server` 子进程、临时 HTTP/TCP/UDP 端口、临时 MongoDB 数据库、
真实 KCP/Protobuf 客户端和 Router wire，不连接或修改其他正在运行的服务。
执行前先构建服务：

```bash
go build -o bin/server ./cmd/server
go test -tags=system ./systemtest \
  -run TestSystemFullStackInterfaces -count=1 -v
```

当前测试拓扑包含 Location、Central、Home、Map2、Match、Realm、Gate、HTTP、
Router 和 RouterNode。覆盖范围：

- HTTP：注册、重复注册、非法注册、账号登录、错误密码、Area 列表、
  Router `/zone/last` 缺失/非法 Token；
- Router HTTP：`/router/list`、`/zone/list`、`/zone/last`、Router 登录；
- Realm/Gate：正常 `C2R_Login`、非法 AccessToken、正常/非法
  `C2G_LoginGate`、Gate Ping；
- Map/StateSync：EnterMap、创建自身 Unit、AOI `CreateUnits`/`RemoveUnits`、
  无 Finder 的寻路错误、Stop；
- Inventory：背包/仓库查询、空容器交换/整理成功、非法操作和数量错误；
- Map transfer：Home → Map2、切图通知、目标 Unit 通知和源地图移除通知；
- Match/Room：两个账号的匹配、Gate Room 绑定、当前 `MatchCount=1` 契约下
  的两个独立 Room、ChangeSceneFinish、Room Start、FrameMessage、
  `OneFrameInputs`、`AdjustUpdateTime` 和 `CheckHash` 路由；
- Router wire：UDP/TCP RouterSYN/RouterACK；普通 SYN/ACK/MSG/FIN、
  Reconnect 的完整转发由 `module/router` integration/regression tests 覆盖。

2026-08-06 已执行并通过：

```text
go test -tags=system ./systemtest -run TestSystemFullStackInterfaces -count=1 -v
PASS

go test -tags=system ./systemtest -run TestSystemHTTPAndProtocolBoundaries -count=1 -v
PASS

go test -tags=system -count=1 ./...
PASS
```

本轮新增 `systemtest/interface_boundary_test.go`，以真实 `bin/server` 子进程
和拟真 HTTP/KCP/UDP 客户端验证边界行为：

- HTTP 根路径、未知路由、方法错误、Malformed JSON、CORS 白名单/拒绝、
  OPTIONS 预检，以及 Router HTTP 未知路由、方法错误、登录失败；
- Realm/Gate Token 重放必须关闭会话；
- Map/Inventory 非法 UnitID、目标不存在的 Map transfer；
- Router UDP 的 RouterSYN/RouterACK、普通 SYN/ACK/MSG/FIN 和
  ReconnectSYN/ReconnectACK；
- Lockstep Room 启动、帧输入、调整时间、哈希异常快照通知。

Room 启动会先写入第 `0` 帧初始快照，再广播 `Room2C_Start`；哈希检查使用
`FrameBuffer.CheckAndSetHash` 原子记录首次哈希，避免 Room 无序邮箱下两个
并发检查请求同时读到“未记录”而丢失 mismatch。第 `0` 帧快照只作为服务端
恢复锚点，不改变外部客户端帧从 `1` 开始的协议约束。

该结果证明本机单进程全拓扑在真实端口、真实 MongoDB、真实 KCP Session 和
真实 protobuf payload 下可运行；不等价于双 OS Process、跨机器网络、生产
客户端、压力测试或故障恢复通过。

生产验收至少需要：

```text
启动 JSON
  → 多个 OS Process
  → Location/Central/Realm/Gate/Map
  → 真实 MongoDB
  → 客户端或测试客户端
  → 登录
  → EnterMap
  → Pathfinding
  → TransferMap
  → Reconnect/Disconnect
  → 关闭和重启
```

双进程/跨机器验收仍需在此测试基础上增加真实 peer 配置和故障演练。

### 2.5 本地可运行部署

仓库默认本地部署配置位于 `data/config/json`，对应一个 HTTP Scene：

```text
Process 1
  └── HTTP :18080
        └── MongoDB mongodb://127.0.0.1:27017/etgo_dev
```

部署命令：

```bash
go build -o bin/server ./cmd/server
./bin/server --process=1 --config=data/config/json --log-level=info
```

最小运行检查：

```bash
curl -sS http://127.0.0.1:18080/
curl -fsS -X POST http://127.0.0.1:18080/register \
  -H 'Content-Type: application/json' \
  -d '{"Username":"deploy-user","Password":"DeployPass-2026!"}'
curl -fsS -X POST http://127.0.0.1:18080/login \
  -H 'Content-Type: application/json' \
  -d '{"Username":"deploy-user","Password":"DeployPass-2026!"}'
```

该配置使用开发密钥和固定端口，不是生产部署模板。

全拓扑系统测试使用临时配置并单独启动服务，不会复用默认部署的 PID；
因此可以在 PID 329959 仍运行时执行，不会占用 `18080` 或默认 `etgo_dev`
数据库。

## 3. 关键回归测试

### 3.1 Fiber/Actor

必须覆盖：

- mailbox 满返回 `ErrMailboxFull`；
- Fiber 停止后 Send 返回 `ErrFiberClosed`；
- Call 经过目标 Fiber task queue；
- RPC timeout 和 context cancel；
- 目标 MailBox 缺失返回 `ErrHandlerNotFound`；
- 外部 envelope Message/Request/Response。

### 3.2 网络

必须覆盖：

- 空 TCP/KCP 地址报错；
- KCP Connect 前未 Listen 报错；
- Session Send 队列满报错；
- Session 关闭与并发发送的边界；
- KCP 入站 buffer 满关闭 Channel；
- 标准 KCP segment 在乱序/丢包后重传并按序交付；
- 多个客户端使用相同 localConn 编号时，KCP 仍按远端地址和 conversation
  正确分流，且双向消息不串线；
- Process peer 使用共享 secret 完成 HMAC handshake；
- Process peer Actor envelope/RPC 经过真实 KCP Channel 到达目标 Fiber；
- handshake Request 缺少非零 RpcID 时被拒绝；
- 主动拨号方断线后重连，`ProcessOuterSender` Session 被替换；
- Timer `WaitAsync` 在销毁后关闭等待 channel，销毁后不再创建新 Timer；
- Router 目标地址为空报错；
- ActorLocation 消息编号与 Go 类型不匹配时报错；
- Router transport 缺失时不注册节点、不改变状态并返回失败；
- HTTP Serve 异常可观察；
- Fiber 消息/Update/帧结束回调 panic 被隔离，`Fiber.Call` task panic
  返回 `ErrFiberTaskPanic`；
- FiberManager 停止后拒绝创建新 Fiber，NetComponent、Session timeout、
  idle checker 不在销毁后重新启动。
- `ProcessOuterSender` 使用配置的 RPC timeout，并在 timeout 后移除 pending。

### 3.3 Location

必须覆盖：

- Lock 时 Get 不阻塞 Location Fiber；
- 代理端在 `ErrLocationLocked` 下重试；
- ownership old Actor 不匹配时拒绝 Unlock；
- Map transfer 使用 old/new Actor；
- 未分配 Unit key 返回零 Actor，不被 MessageSender 当作有效地址。
- Location proxy 和 MessageLocationSender 在本地拒绝非正 key。
- AOI `Enter` 重新接入其他 Cell 时不留下旧 Cell 引用。

同时覆盖组件关闭后的行为：`WaitFrameFinish` 关闭等待 channel，Timer、
MailBox、AOI、Move、Numeric、Bag、CoroutineLock、Crontab 和
MessageLocationSender 不会在销毁后重新打开内部状态。

### 3.4 Map

必须覆盖：

- 缺 UnitComponent；
- 缺 LocationProxy；
- 缺 AOIManager；
- unknown transfer component；
- target 已存在同 ID Unit；
- RPC 失败时源 Unit 保留；
- target ownership 切换失败时目标回滚；
- 成功后源 Unit 才 Dispose；
- 坐标和 Rotation 保持。
- Unit registry 拒绝重复/无效 Unit，失败时新 Unit 被销毁。
- transfer journal 拒绝无效 source ActorID；
- Unit snapshot 拒绝未知 UnitType、非正 ID 或非正 ConfigID；
- durable ledger 在提交完成记录后才向等待方返回成功。
- source journal recovery 在缺 token、目标不确定或 cleanup 失败时保留记录；
- target ledger recovery 只接受 RpcID/错误语义匹配的 terminal response，不执行
  Unit/AOI/Location 副作用。

### 3.5 Lockstep

必须覆盖：

- 动态 Room Fiber 创建失败；
- RoomServerComponent 缺失；
- 帧输入补齐；
- SnapshotProvider 成功；
- Room 启动时第 `0` 帧初始恢复快照；
- 无序 Room mailbox 下 CheckHash 的原子首次记录和 mismatch 广播；
- `LSWorld` 按 PlayerID 确定性推进、原 C# 移动公式和当前位置/旋转输出；
- 未知玩家输入拒绝且不部分修改世界；
- `LSWorld` 非法恢复状态拒绝且不部分替换单位；
- 默认 Room snapshot 版本和 checksum 校验；
- SnapshotProvider 缺失时不生成占位快照；
- Reconnect/Hash mismatch 缺少快照返回错误。
- Snapshot frame 保留连续帧号和空帧，恢复失败时不部分修改 Room；
- Bag snapshot 非法 Slot/ID/配置/容量被拒绝，恢复保留 SlotIndex 并推进
  Item ID watermark；
- AOI manager 销毁后清理 Entity 的 Cell 和可见性引用；
- DB Save 的更新字段不含 immutable `_id`，实体 ID 不匹配时失败；
- `DBComponentCRUDAndIncrement` 在 MongoDB 可用时验证“先默认 upsert、后
  原子 `$inc`”的两步语义；
- Component 替换时旧组件的 Entity 引用被清除；
- UnitDumper durable queue 必须先入队再写业务文档，队列写失败时不得直接
  写业务文档，重启/下一次 dump 能重放未删除任务；
- migration 定义包含 account 唯一索引、player profile `(account_id, zone_id)`
  唯一索引和 account `password_algorithm` 标记；
- Map 默认场景在同 Zone 存在 `Home` 与其他地图时优先选择 `Home`，没有
  `Home` 且候选不唯一时仍返回歧义错误；
- Argon2id 新密码、旧 MD5 首次登录升级和错误哈希拒绝。
- AccessToken 未配置时拒绝生成/验证，key ring 轮换和显式 legacy 兼容；
- AccessToken 撤销的 Memory/MongoDB store 接口边界、TTL migration 和无数据库
  时的显式错误；
- Memory Token revocation store 拒绝同 TokenID 的跨 AccountID 混用；
- Gate Token 保持原 `D5.accountID.D4` Hex wire format，随机源失败时传播错误；
- HTTP TLS 启动严格校验、新连接证书热轮换和轮换文件错误；
- HTTP 登录审计成功/失败事件、审计 sink 写入错误传播；
- MongoDB 原子 bucket 登录限流组件、固定窗口边界和无数据库时的显式错误；
- HTTP CORS 只允许精确 allowlist，拒绝未配置来源；
- HTTP JSON `Content-Type`、目标 200 状态和 404/500 包体语义；
- Router UDP/TCP little-endian 方向帧、RouterSYN/ACK、ordinary SYN/ACK、
  MSG、FIN 和 Reconnect；
- Router TCP little-endian `uint16` body length、连接复用和 ACK 回写；
- Map transfer 进程内幂等 ledger 的重复请求、指纹冲突和错误重试；配置化
  target durable ledger 的状态写入和未完成记录拒绝重复创建；
- Match 通知部分失败时有限重试回收 Map Room、发送 Gate 补偿清理消息，
  并在补偿完整成功后恢复匹配队列。

以上“必须覆盖”是验收要求，不代表每一项都有真实环境证据。当前默认
代码基线已执行的证据是：

```text
go test -count=1 ./...
go test -race -count=1 ./...
```

关键回归包括 KCP 丢包重传、Map 两阶段 Transfer、Match→Room、Room
Fiber 回收、Room snapshot checksum、Gate→Room 完整 ActorID 路由和认证
PlayerID 覆盖、配置拓扑/端口校验、migration 定义校验、Argon2id/MD5
密码迁移、Fiber 停止/取消、Actor RPC timeout 清理、AOI 重入、ECS 组件替换、
严格 Unit/Transfer 身份校验、Lockstep 原子恢复、Token 撤销身份校验、
Gate Token wire format、Mongo Increment 两步语义、Location/Move/Timer/Actor/
HTTP/Router 关闭和缺依赖行为、登录 alignment，以及全拓扑
`systemtest/TestSystemFullStackInterfaces`；
它们仍属于进程内/替身测试，不替代真实 MongoDB、客户端和双进程验收。

## 4. 竞态检测

`go test -race -count=1 ./...` 重点关注：

- `RpcManager.callbacks/timers`；
- `Fiber` mailbox/task/stop；
- `LocationOneType.locations/lockInfos`；
- `FrameBuffer`；
- `Replay`；
- `RoomManager.rooms`；
- `MessageLocationSender.cache`；
- Session callbacks/sendCh。

Race detector 通过不等于业务 ownership 正确；它只能说明未发现数据竞争。

KCP 还覆盖并发 `Listen` 只有一个调用可以完成绑定；其余调用必须得到
明确的“已监听”错误，避免两个 goroutine 在检查和 UDP bind 之间形成竞态。

## 5. 验收清单

### 启动前

- [ ] 四个必需 JSON 文件存在；
- [ ] 当前 Process 有 Scene；
- [ ] Scene ID/Name/Zone/Process 合法；
- [ ] Machine 有实际地址；
- [ ] 监听端口不冲突；
- [ ] Zone DBAddr/DBName 可达；
- [ ] Home Map 已配置；
- [ ] Map target 已显式注入；
- [ ] Pathfinding Finder 已注入；
- [x] Go `RoomStateSnapshot v2` SnapshotProvider 默认注入；
- [x] Go LSWorld 状态已纳入快照并支持原子恢复；
- [ ] 原 C# MemoryPack/TrueSync wire 与客户端 LSWorld 恢复已验收；
- [ ] 密钥和 CORS/TLS 策略已配置。
- [ ] MongoDB 撤销、限流 bucket 和登录审计集合的 migration 已执行。

### 启动后

- [ ] 每个配置 Scene 都创建成功；
- [ ] Fiber/Scene registry 可查；
- [ ] Location、Central、Map ActorID 有效；
- [ ] Realm/Gate KCP 正常监听；
- [ ] HTTP/Router 正常监听；
- [x] 同机双 peer Session 完成握手；
- [x] 同机发送和接收 Actor envelope；
- [x] 同机 RPC response RpcID 对齐；
- [ ] 真实双 OS 进程、跨机器网络和客户端联调。

### 业务流程

- [ ] 注册；
- [ ] HTTP 登录；
- [ ] Realm 登录；
- [ ] Gate 登录；
- [ ] Central profile；
- [ ] Home Map enter；
- [ ] Unit Location；
- [ ] AOI/StateSync；
- [ ] Pathfinding；
- [ ] Inventory；
- [ ] Map transfer；
- [ ] Match/Room；
- [ ] Disconnect/reconnect；
- [ ] Lockstep snapshot/recover。

Map transfer 的进程内回归还必须验证：请求只能使用真实 Unit MailBox 的
完整 ActorID；地图间 request 的 RpcID/OldActorID 非法时直接拒绝，错误
响应保留 RpcID。Lockstep 回归必须验证多 Map/Match 候选返回歧义错误，
以及认证 Session 伪造其他 PlayerID 被拒绝。

### 关闭后

- [ ] Session 已关闭；
- [ ] KCP/HTTP listener 已关闭；
- [ ] Fiber 已停止；
- [ ] Room 动态 Fiber 已清理；
- [ ] DB client 已关闭；
- [ ] 异步 Save 已结束或有失败记录；
- [x] registry 不再返回 disposed Scene。

## 6. 开发流程

```text
先定位运行边界
  → 写/更新接口和错误
  → 修改生产路径
  → 补缺依赖测试
  → gofmt
  → go test -count=1 ./...
  → go vet ./...
  → 必要时 go test -race
  → 更新 docs/IMPLEMENTATION_STATUS.md
```

不要先把测试改成接受隐式 fallback。若协议或配置语义不足：

1. 先返回明确错误；
2. 写 `TODO(scope)`；
3. 说明缺失信息和风险；
4. 等协议/配置决策后实现。

## 7. 常见误判

| 误判 | 正确判断 |
|---|---|
| `go test ./...` 通过就能上线 | 只证明默认 Go 测试通过 |
| 有 `ProcessOuterSender` 就是跨进程完成 | 还需要配置启用、真实双进程验收和部署安全 |
| `Location.Get` 返回零 Actor 是异常 | 未分配 Unit 是合法状态；发消息时必须拒绝 |
| 端口 `0` 是生产默认 | 只适合测试临时端口 |
| Map transfer RPC 返回就算成功 | 必须检查目标 response 和 ownership |
| Lockstep 有 FrameBuffer 就有完整快照 | 必须有真实 Go LSWorld 状态和版本化恢复；当前还需跨语言 TrueSync wire |
| 组件 factory 找不到时跳过 | 会造成数据不完整，应返回错误 |
