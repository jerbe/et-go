# ET-Go 项目文档

本目录是 `github.com/jerbe/et-go` 的 Go 版本工程文档。主线按“可执行程序、运行时对象、消息路径、服务流程和领域状态”组织，不按原 ET/C# package 拆分章节。

分析基线：`2026-08-06`  
模块声明：`github.com/jerbe/et-go`  
Go 版本：`go 1.25.7`  
默认入口：`cmd/server`

## 先看结论

ET-Go 当前是一个单仓库、多 Fiber 的 Go 游戏服务端骨架。一个 OS 进程可以根据 JSON 启动配置创建多个业务 Scene/Fiber；同进程消息经过 Fiber 邮箱，跨进程消息由 `ProcessOuterSender` 和网络 Session 承载。

当前工程已经具备：

- ECS：`World → Scene → Entity → Component`；
- Fiber：每个 Fiber 拥有独立 Scene、消息邮箱、任务队列和帧循环；
- Actor：`ActorID + MailBox + MessageSender + RPC`；
- 网络：统一 Packet、Session、TCP、KCP、Router UDP/TCP；
- 服务流程：Location、Central、Realm、Gate、Map、Match、Room、HTTP、Router；
- 领域能力：Unit、AOI、Move、Numeric、StateSync、Inventory、Lockstep；
- 配置与持久化：启动 JSON、MongoDB、BSON、迁移函数；
- 自动化验证：默认包测试、流程测试、网络回环测试，以及真实
  `bin/server` 子进程上的全拓扑 HTTP/KCP/Router 拟真测试。
- 当前测试还覆盖 HTTP/Router 边界、CORS、Token 重放、非法 Unit/Transfer、
  Router 完整 UDP 转发，以及 Room 第 0 帧初始快照和 CheckHash 竞态。

最近的严格化还包括：未知 SceneType 启动前拒绝、Fiber 更新系统实例去重、
Crontab 生命周期幂等、重复玩家档案拒绝、KCP 并发监听保护、NetInner peer
HMAC handshake/重连，以及 Map transfer 的完整 Actor 路由和 RpcID 保真；
Inventory `SortBag` 对未知配置返回错误并保持原状态，ActorLocation 协议
编码器拒绝消息编号与 Go 类型不匹配，Router 缺少目标地址时显式失败。
最新一轮还补齐了未启动 Fiber 的不可重启 Stop、取消 Call 不执行任务、
ProcessOuterSender timeout pending 清理、AOI 重入去脏、ECS 组件替换生命周期、
Lockstep 空帧/原子恢复、Map transfer source ActorID 与 Unit snapshot 身份校验，
以及 MongoDB Increment 的两步原子语义。

当前不能标记为生产闭环的能力：

- Go/KCP peer 的连接发现、握手、重连和远端 Session 装配已有自动化闭环，但
  真实双 OS 进程、跨机器和客户端联调尚未验收；
- Map 的真实导航网格 Finder 尚未接入，寻路缺少 Finder 时必须报错；
- Lockstep 已有 Go `LSWorld` 确定性状态模型和 `RoomStateSnapshot v2`；
  快照包含单位位置、旋转、输入、世界帧和 ID watermark，但原 C#
  MemoryPack/TrueSync wire 与客户端恢复协议仍未完成；
- Lockstep 客户端 Room 消息已经按认证 Session→PlayerRoom→Room Actor
  绑定路由；缺少绑定或完整 ActorID 时失败，不能按 Unit Location 猜测转发；
- 密码已使用 Argon2id，旧 MD5 账号首次成功登录时升级；AccessToken 默认使用
  配置化 HMAC key ring、key-id 和轮换，只有显式 legacy 格式才生成旧
  SimpleToken；
- HTTP 已支持显式 TLS 证书配置和 `httpRequireTLS` 强制模式；生产配置的登录
  限流使用 MongoDB 原子 bucket，独立测试仍可显式使用进程内固定窗口组件；
  AccessToken 已提供撤销存储接口、MongoDB 共享适配器和启动 wiring；新 TLS
  握手会重新读取证书文件；密钥托管、审计保留/运营协议和真实生产验收仍需部署实现；
- Fiber 回调、网络组件和定时检查器已有关闭边界与 panic 隔离；
- UnitDumper 已在 Unit 所属 Fiber 内生成不可变 BSON snapshot，并采用
  “先写 durable queue、再写业务集合、成功后删队列”的顺序，支持下一次
  dump 重放未完成任务；Map transfer source journal 和配置化目标 durable
  ledger 已落地，source journal 和 target ledger 已提供显式
  `TransferRecoveryCoordinator` / `TransferLedgerRecoveryCoordinator` scanner
  编排；但跨组件一致性、跨进程状态查询、recovery token 和真实 ownership
  证明仍未完成；
- RouterNode 已实现目标小端 Router UDP 控制帧，以及带 little-endian
  `uint16` 长度前缀的外部 TCP transport；WebSocket 仅保留为明确 TODO；
- migration runner 已包含账号唯一索引、玩家档案 `(account_id, zone_id)` 唯一索引、
  账号 `password_algorithm` 标记、AccessToken 撤销 TTL index、登录审计时间
  索引，并使用 MongoDB lease lock 协调多个 Process；
- 生产规模 MongoDB、客户端、双进程部署和压力验收不等于 `go test ./...`；
  当前已有本机 Docker MongoDB 的 HTTP/migration 真实运行证据，但不能把它
  扩大为生产部署通过。

代码在可定位的实现边界使用 `TODO(scope)` 标出；全部未完成项统一在
`IMPLEMENTATION_STATUS.md` 维护，并写明不能安全猜测或尚未验收的原因。

## 入口、运行对象和证据摘要

| 能力 | 入口 | 所属运行对象 | 关键依赖 | 自动化证据 | 真实证据 | 当前边界 |
|---|---|---|---|---|---|---|
| HTTP 账号登录 | `POST /login` | HTTP Scene | DBManager 或显式测试 Store | HTTP/替身 + 全拓扑系统测试 | 本机 Docker MongoDB 注册/登录已验证 | HMAC key ring 配置和生产部署仍需验收 |
| Realm 登录 | `C2R_Login` | Realm Fiber | Location、Gate registry、Central、peer | alignment + 全拓扑系统测试 | 本机独立服务/KCP 客户端已验证 | 双 OS 进程/生产客户端未验收 |
| Gate 登录 | `C2G_LoginGate` | Gate Fiber/Session | GateToken、Central、Home Map | alignment + 全拓扑系统测试 | 本机独立服务/KCP 客户端已验证 | 生产客户端未验收 |
| 首次进入 Home Map | `G2M_EnterMap` | Gate → Map Actor | Unit、AOI、Location | 进程内流程 + 全拓扑系统测试 | 本机拟真登录链路已验证 | 生产客户端未验收 |
| 客户端地图入口 | `C2M_EnterMap` | Client → Map Fiber → Unit MailBox | 已登录 Unit、Map 组件 | handler + 全拓扑系统测试 | 本机拟真 KCP 客户端已验证 | 与 `G2M_EnterMap` 不是同一方向 |
| Map transfer | `C2M_TransferMap` | Map Fiber → Map Fiber | 同 Zone、同 Process target | 两阶段迁移、journal/ledger/recovery 编排 + 全拓扑系统测试 | Home→Map2、真实 KCP 通知已验证 | 双进程/崩溃恢复未验证；无真实跨进程协调器 |
| Lockstep | `C2G_Match`、Room 消息、`LSWorld` | Match/动态 Room Fiber | Map、Gate、SnapshotProvider | 流程、世界推进、checksum、原子恢复 + 全拓扑系统测试 | 两账号 Match/Room/FrameMessage 已验证；当前 `MatchCount=1` | 客户端恢复/跨语言 wire、压力未验证 |

端到端阶段关系是：

```text
默认 HTTP Scene `/login`
  → AccessToken
  → C2R_Login
  → GateToken + Gate 地址
  → C2G_LoginGate
  → G2M_EnterMap
  → Map Unit
```

上图中的 `/login` 特指默认 HTTP Scene 的账号登录接口，不是 RouterManager
提供的同名 `/login` 转发接口。`G2M_EnterMap` 是 Gate 在登录流程中调用
Map 的内部消息；`C2M_EnterMap` 是 `Client → Map` 的外部协议入口，Map
handler 再将请求派发到 Unit MailBox。两者可以共享 Unit 初始化规则，但
不能按同一个网络方向或同一个 MsgID 理解。

## 阅读路线

| 顺序 | 文档 | 解决的问题 |
|---:|---|---|
| 1 | [01-project-overview.md](./01-project-overview.md) | 项目边界、启动拓扑、当前能力 |
| 2 | [02-go-package-architecture.md](./02-go-package-architecture.md) | Go 实现边界、运行对象和依赖方向 |
| 3 | [03-runtime-and-concurrency.md](./03-runtime-and-concurrency.md) | World、Fiber、Scene、Entity、并发所有权 |
| 4 | [04-messaging-and-network.md](./04-messaging-and-network.md) | MailBox、RPC、Packet、Session、KCP、Router |
| 5 | [05-service-workflows.md](./05-service-workflows.md) | 登录、Location、Map、HTTP、Lockstep 的实际调用链 |
| 6 | [06-game-domain.md](./06-game-domain.md) | Unit、AOI、Move、StateSync、Inventory、Lockstep 状态 |
| 7 | [07-data-persistence-and-configuration.md](./07-data-persistence-and-configuration.md) | JSON、地址、MongoDB、BSON、迁移 |
| 8 | [08-protocol-and-api-reference.md](./08-protocol-and-api-reference.md) | protobuf、MsgID、HTTP、错误响应 |
| 9 | [09-testing-and-development.md](./09-testing-and-development.md) | 构建、测试、静态检查、真实验收 |
| 10 | [IMPLEMENTATION_STATUS.md](./IMPLEMENTATION_STATUS.md) | 能力状态和阻塞项 |
| 11 | [11-roadmap.md](./11-roadmap.md) | 按闭环推进的整改顺序 |

兼容审计任务入口放在 [modules/](./modules/README.md)。其中 `Mxx` 只是原
ET 编号，任务内容按 Go 运行对象、消息路径和可验证证据书写，不代表 Go
package 目录结构。

兼容性资料放在 [reference/](./reference/README.md)。它只能回答“与原 ET 行为如何对照”，不能决定 Go 包的目录结构。

## 目录和责任边界

```text
cmd/server/        可执行程序、配置加载、Fiber 创建、进程生命周期
config/            启动 JSON 的结构、加载和拓扑校验
engine/            ECS、Fiber、Actor、Timer、锁、网络、数学等通用运行时
module/            服务和游戏领域实现
db/                MongoDB Client、DBManager、DBComponent、持久化实体
proto/              .proto 源文件及生成的 Go 类型
internal/log/      仓库内部日志实现
docs/              当前 Go 工程的架构、流程、协议、验证和路线图
```

依赖方向应保持为：

```text
cmd
 ├── config
 ├── engine
 └── module
      ├── engine
      ├── db
      └── proto

db/config/engine 不依赖 module。
proto 只承载协议类型，不承载业务服务状态。
```

## 启动命令

```bash
go run ./cmd/server \
  --process=1 \
  --config=/path/to/config-dir \
  --log-level=info
```

`--config` 接收的是配置目录，不是某一个 JSON 文件。目录结构必须是：

```text
config-dir/
├── startmachineconfig.json
├── startprocessconfig.json
├── startsceneconfig.json
├── startzoneconfig.json
└── startsecurityconfig.json  # 认证/HTTP Process 必须；其他 Process 可省略
```

仓库已提供可直接运行的本地 HTTP 拓扑：`data/config/json/`。它使用
`127.0.0.1:18080`、MongoDB 数据库 `etgo_dev` 和仅用于开发的
AccessToken 密钥。准备 MongoDB 后可以直接执行：

```bash
docker run -d --name et-go-mongodb \
  -p 127.0.0.1:27017:27017 \
  -e MONGO_INITDB_DATABASE=etgo_dev \
  -v et-go-mongodb-data:/data/db \
  mongo:7.0

go build -o bin/server ./cmd/server
./bin/server --process=1 --config=data/config/json --log-level=info
```

启动日志必须出现 `ET-Go 服务器启动完成`；`GET /` 返回统一
404 JSON，`POST /register`、`POST /login` 可验证真实 MongoDB 迁移、账号
写入、Argon2id 密码校验、审计和 AccessToken。该配置只用于本地验收，
不能直接作为生产密钥和生产拓扑。

## 全拓扑拟真验收

`systemtest/blackbox_test.go` 会创建独立临时配置、临时端口和唯一 MongoDB
数据库，启动一个真实 `bin/server` 子进程，再使用真实 KCP/HTTP/Router
客户端覆盖当前可执行的外部接口。它不会连接默认部署服务，也不会使用默认
`etgo_dev` 数据库。

```bash
go build -o bin/server ./cmd/server
go test -tags=system ./systemtest \
  -run TestSystemFullStackInterfaces -count=1 -v
```

2026-08-06 已通过。测试覆盖 HTTP 账号流程和错误 Token、Realm/Gate 正常与
失败登录、Map Enter/AOI/Stop、Inventory 查询与操作、Home→Map2 切图、
当前 `MatchCount=1` 契约下的两个独立 Room、ChangeSceneFinish、
FrameMessage、OneFrameInputs、AdjustUpdateTime、CheckHash 路由以及
Router UDP/TCP RouterSYN/ACK。`module/router` 常规 integration tests
另外覆盖普通 SYN/ACK/MSG/FIN 和 Reconnect wire。该结果仍不代表双 OS
Process、跨机器、生产客户端、压力和崩溃恢复验收通过。

启动是严格的：

1. 配置目录必须存在；
2. `startmachineconfig.json`、`startprocessconfig.json`、`startsceneconfig.json`、`startzoneconfig.json` 必须存在且可解析；
3. 当前逻辑 Process 必须在 Process 配置中；
4. 当前 Process 至少要有一个合法 Scene；
5. Scene 的 Process、Zone、Machine 引用必须完整；
6. Realm、Gate、HTTP、Router、RouterNode 必须有 `outerPort` 或
   `Process.InnerPort`；
7. 使用 AccessToken 的 Process 必须配置 `startsecurityconfig.json` 的签名
   key ring；HTTP Process 还必须配置精确 CORS allowlist；
8. Scene 初始化或监听失败会终止启动；
9. 不会创建“空配置运行时”，不会把 `127.0.0.1:0` 当生产监听地址。

开发测试可以在测试代码中明确使用端口 `0` 请求操作系统分配临时端口；这不是部署配置的默认值。

## 证据等级

文档中的“完成”必须拆成五种证据：

```text
源码存在
  ≠ 注册可达
  ≠ 配置启用
  ≠ 自动化测试通过
  ≠ 真实运行通过
```

例如，`ProcessOuterSender` 有发送和接收端 API，只说明“源码存在”；当前还需要
`StartProcessConfig.Peers`、`engine/network/peer` 的 handshake/重连和真实
双进程验收，才能把部署写成“跨进程已完成”。

Actor 不是与 Fiber 平行的另一种 goroutine。Actor 是运行时地址、MailBox
和消息路由边界；Actor 对应的 Entity/Component 状态仍由所属 Scene/Fiber
串行拥有。

## 合法状态与禁止 fallback

以下是明确的业务状态或测试行为，不是生产 fallback：

- Location 未分配 owner 时返回零 ActorID；真正发送前必须拒绝该地址；
- 锁步当前帧缺输入时按协议复用上一帧输入，再没有则使用空输入；
- 测试显式使用端口 `0` 请求操作系统分配临时端口；
- Map 没有 target 时仍可启动，但切图请求返回明确错误；
- `RoomStateSnapshot v2` 默认 provider 保存 Go `Room/FrameBuffer/RoomPlayer/LSWorld`
  状态，但它不等于原 C# 客户端 MemoryPack/TrueSync wire 快照。

以下行为禁止：

- DB 失败后切换到内存账号/Profile；
- Finder 缺失后生成 `[start,target]` 直线路径；
- SnapshotProvider 缺失后生成固定占位字节；
- 未知迁移组件静默跳过；
- 用普通 `[][]byte` 队列伪装 KCP 可靠传输；
- 缺少监听地址时自动绑定生产临时端口；
- 目标迁移未成功前销毁源 Unit。

## 文档维护规则

1. 先描述 Go 的运行对象和调用链，再描述兼容性。
2. 不用 ET package 名作为主目录；需要引用时放在兼容性附录。
3. 每个能力同时记录注册点、依赖、错误边界、测试证据和未完成 TODO。
4. 协议变更必须同步修改 `.proto`、生成代码、`proto_codec.go`、handler 和测试。
5. 代码中的默认常量只表示明确的运行策略；不能把缺配置、缺依赖、空地址转成成功状态。
6. 所有异步任务必须有错误处理、日志或可观察的失败状态。
