# 项目总览：从进程启动到业务状态

## 1. 项目定位

ET-Go 是一个 Go 游戏服务端工程。它不是“每个业务 package 启动一个服务”，而是：

```text
一个 cmd/server 进程
  → 加载一份启动拓扑
  → 创建当前 Process 的多个 Fiber
  → 每个 Fiber 拥有一个 Scene
  → Scene 挂载服务组件、Entity、MailBox 和 Update 系统
```

同一份代码可以通过配置运行不同拓扑，例如：

- 一个进程承载 Location、Central、Realm、Gate、Map；
- 多个进程分别承载 RouterNode、Realm、Gate、Map；
- Map 和 Room 在运行中动态创建；
- HTTP/Router 作为独立 Scene 监听配置地址。

因此，理解项目的第一单位是“运行中的 Scene/Fiber”，不是目录名。
目录和包只是 Go 实现定位手段；本文档及后续章节优先按启动拓扑、状态所有权、
消息路径和持久化边界组织，不按原 ET package 列表展开。

## 2. 工程边界

```text
cmd/server
  ├── 读取 config
  ├── 设置 config global
  ├── 创建 World/FiberManager
  ├── 按 StartSceneConfig 创建当前进程的 Fiber
  └── 等待信号并关闭

engine
  ├── ECS / Event / Timer
  ├── Fiber / MailBox / Actor / RPC
  ├── TCP / Session / KCP / Router transport
  ├── network/peer：NetInner peer handshake、Session 生命周期和重连
  ├── lock / pool / math / id
  └── 日志和网络 codec

module
  ├── 服务：Location、Central、Realm、Gate、HTTP、Router
  ├── 地图：Map、MapRPC、Unit、AOI、Move、StateSync
  ├── 领域：Inventory、Numeric、Lockstep、Crontab
  └── 协议边界 handler 和业务状态

db
  ├── MongoDB Client
  ├── DBManagerComponent
  ├── DBComponent
  └── CAccount/CPlayerProfile 等持久化模型

config
  └── Machine / Process / Scene / Zone / Area 的启动拓扑

proto
  └── .proto 源文件与生成 Go 类型
```

## 3. 启动链路

`cmd/server/main.go` 的启动顺序：

```text
flag.Parse
  → config.Load(configDir)
  → Config.Validate(processID)
  → Config.RuntimeTopology + 启动拓扑摘要
  → DB migrations（当前 Process 含 DB Scene 时）
  → config.SetGlobal
  → ecs.NewWorld
  → fiber.NewManager
  → createConfiguredFibers
      → 有 peer 配置且未显式声明 NetInner 时创建隐式 NetInner
      → parseSceneType
      → Manager.CreateConfigured
          → 设置根 Scene ID/Name
          → 执行 FiberInitHandler
          →（动态 Room 使用 CreateWithSetup 注入运行时依赖）
          → 注册 Fiber
          → 启动 Fiber loop
  → 等待 SIGINT/SIGTERM
  → FiberManager.StopAll
  → World.Shutdown
```

关键约束：

1. Scene ID 和 Name 在业务初始化前写入，网络地址解析、Actor registry 和服务查找不会看到临时身份。
2. 配置文件缺失、拓扑引用错误、Scene 初始化错误、监听错误都会阻止进程启动。
   `RuntimeTopology` 会在创建 Fiber 前输出当前 Process 的 Machine、peer、
   显式/隐式 NetInner 和 Scene 清单。
3. `createConfiguredFibers` 不会创建没有配置的业务 Scene。
4. Process 启用 peer/NetInner 后，外部网络 Scene 必须使用独立 `outerPort`，
   不会把 `innerPort` 当作冲突端口的兜底。
5. `go run ./cmd/server` 不是“空配置演示”；必须给出完整启动 JSON。

## 4. Scene/Fiber 类型

当前 `engine/ecs.SceneType` 定义了以下类型：

| SceneType | 主要职责 | 初始化来源 |
|---|---|---|
| `Main` / `Launch` | 主进程或启动编排预留 | 当前无完整业务初始化 |
| `NetInner` / `NetClient` | NetInner peer KCP / 客户端网络 | `engine/network/peer`（NetClient 仍按服务挂载） |
| `Location` | ActorID 定位、锁、Add/Get/Remove | `module/actorlocation` |
| `Router` | HTTP RouterManager | `module/router/fiber_init_router_mgr.go` |
| `RouterNode` | UDP/TCP Router 中继 | `module/router/fiber_init_router.go` |
| `Realm` | AccessToken 校验和 Gate 分配 | `module/login/fiber_init.go` |
| `Gate` | 客户端 Session、Gate 登录、中继 | `module/login` + `module/gate` |
| `Match` | 匹配和 Map 房间请求 | `module/lockstep` |
| `Room` | 房间玩家状态、帧同步推进 | `module/lockstep` |
| `Map` | Unit、AOI、移动、Inventory、切图 | `module/map_` |
| `Central` | 账号校验和玩家档案 | `module/central` |
| `HTTP` | 默认 HTTP 业务接口 | `module/http` |

Map/Room 的运行时实例不是按 ET package 数量创建，而是由配置和动态房间创建逻辑共同决定。

## 5. 当前可验证的拓扑

### 5.1 单进程拓扑

单进程可以创建多个 Fiber：

```text
Process 1
 ├── Location Fiber
 ├── Central Fiber
 ├── Realm Fiber
 ├── Gate Fiber
 ├── Map Fiber
 ├── Match Fiber
 └── HTTP/Router Fiber
```

这是当前自动化测试主要覆盖的模式。跨 Fiber RPC 会经过目标 Fiber 的任务队列，避免直接跨 goroutine 访问目标 Scene。

### 5.2 多进程拓扑

逻辑上可拆成：

```text
Process A: Realm / Gate
Process B: Location / Central
Process C: Map / Match
Process D: RouterNode
```

当前 Go 运行时已经提供上述跨进程闭环：

- `StartProcessConfig.Peers` 声明远端 Process、NetInner 地址和共享 secret；
- `engine/network/peer.ProcessPeerComponent` 负责 KCP 监听、主动/被动连接、
  HMAC handshake、ProcessID 绑定、Session 装配、断线重连；
- 较小的 ProcessID 主动拨号，避免两个 Process 同时建链造成 Session 竞争；
- `ProcessOuterSender` 负责 Actor envelope、Fiber 投递和 RPC Response；
- `cmd/server` 在存在 peer 配置且未显式声明 NetInner 时自动创建 NetInner Fiber。

双 Process KCP/RPC 集成测试已通过；真实双进程、跨机器和客户端联调仍未验收。

当前 peer handshake 还要求非零 Request RpcID；已取消的 Fiber Call 不进入
目标 handler，ProcessOuterSender 使用配置 timeout 并在超时后移除 pending。

## 6. 构建与运行

```bash
go build ./...
go test -count=1 ./...
go vet ./...
go test -race -count=1 ./...
```

服务启动：

```bash
go run ./cmd/server \
  --process=1 \
  --config=/absolute/path/to/config-dir \
  --log-level=info
```

`--config` 是配置目录，目录中必须同时包含四个必需启动 JSON；使用
AccessToken 的 Process 还必须有 `startsecurityconfig.json`，HTTP Process
必须在其中配置精确 CORS allowlist。它不是某一个 JSON 文件路径。

开发测试中的 `127.0.0.1:0` 只能表示显式的临时端口。部署配置必须写真实可达地址和端口。

## 7. 当前边界

| 能力 | 当前结论 |
|---|---|
| Go 编译 | 已验证 |
| 默认包测试 | 已验证 |
| Fiber 内部消息 | 已验证，含队列错误 |
| 单进程登录流程 | 已有流程测试 |
| Map 单位迁移 | 已有两阶段提交测试 |
| KCP/Router 回环 | 已有本地回环测试 |
| MongoDB 真实连接 | 本地 Docker MongoDB 的 migration/HTTP 登录已验证；生产拓扑和故障演练未验收 |
| 双进程 Actor RPC | Go/KCP peer、envelope、RPC 和重连自动化通过，真实部署未验收 |
| 客户端登录/切图/帧同步 | 没有真实客户端验收证据 |
| 生产安全 | 未完成，见安全 TODO |

## 8. 阅读代码时的判断规则

看到一个类型时，按下面顺序确认它是否真的可用：

```text
类型是否存在
  → 是否在 init 注册
  → 是否有 FiberInit/Scene 挂载
  → 是否被启动配置选中
  → 所需组件是否显式注入
  → handler 是否注册到 MailBox/Session
  → 是否有自动化测试
  → 是否有真实运行证据
```

只满足第一项，不能把能力写成“已实现”。
