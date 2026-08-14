# Go 工程架构与依赖边界

## 1. 设计目标

Go 工程的包边界应回答四个问题：

1. 这段代码是可执行程序、通用运行时、服务能力还是领域状态？
2. 它拥有哪类状态，谁负责创建和销毁？
3. 它通过什么接口依赖外部服务？
4. 缺少依赖时是返回错误、返回业务错误响应，还是记录异步失败？

不要按原 ET 的 package 数量复制目录。Go 版本应该按运行时和依赖方向组织。

## 2. 顶层依赖图

```text
cmd/server
  ├── config
  ├── engine/ecs
  ├── engine/fiber
  ├── engine/actor
  ├── engine/network/peer
  └── module/* 的 FiberInit 注册

module/*
  ├── engine/*
  ├── db
  └── proto/*pb

db
  ├── engine/ecs（持久化组件嵌入 BaseComponent）
  └── config

engine
  ├── engine/ecs
  └── engine/network/codec

engine/network/peer
  ├── engine/network
  ├── engine/actor
  ├── engine/fiber
  └── config

proto
  └── 只依赖 protobuf runtime
```

建议保持以下规则：

- `engine` 不导入 `module`；
- `db` 不导入具体业务 handler；
- `config` 不知道 Scene 的业务实现；
- `proto` 不持有 Entity、Scene、DB 或网络连接；
- `module` 通过最小接口使用 Location、MessageSender、AccountStore 等服务；
- 测试替身实现接口，不把内存 map 偷塞进生产默认路径。

## 3. 实现边界与职责

下面的目录说明只用于定位代码，不是业务文档的主叙事方式。业务能力应先
从运行中的 Process、Scene、Fiber、Actor、消息和持久化状态理解，再回到
具体 Go 包查找实现。

### 3.1 `cmd/server`

职责：

- 解析 `--process`、`--config`、`--log-level`；
- 调用 `config.Load` 和 `Config.Validate`；
- 创建 `World`、`fiber.Manager`；
- 按 `StartSceneConfig` 调用 `CreateConfigured`；
- 负责进程级关闭。

不应放入：

- 账号查询；
- Map 单位逻辑；
- 具体协议 codec；
- 业务默认数据。

### 3.2 `config`

职责：

- 定义 Machine、Process、Scene、Zone、Area；
- 加载必须存在的 JSON 文件；
- 校验 ID 唯一性和引用关系；
- 为网络地址解析提供数据。

配置缺失是启动错误，不是空配置。

### 3.3 `engine/ecs`

职责：

- `Entity` 父子树；
- `Component` 生命周期；
- `Scene` 对 Fiber 根实体的封装；
- `World` 的全局实体边界；
- Update/LateUpdate 接口。

ECS 不应知道登录、Inventory 或 MongoDB 业务。

### 3.4 `engine/fiber`

职责：

- 每个 Fiber 的 goroutine；
- mailbox；
- task queue；
- frame finish task/waiter；
- Update 和 LateUpdate；
- Fiber Manager 生命周期。

Fiber 是串行执行边界。跨 Fiber 业务不能直接调用目标 Scene 的状态方法。

### 3.5 `engine/actor`

职责：

- `ActorID`；
- `MailBox`；
- `DispatchFiberMessage`；
- `ProcessInnerSender`；
- `ProcessOuterSender`；
- Process 级 `ProcessOuterSender` registry；
- RPC callback 管理；
- Scene registry。

`MessageSender` 只是路由门面，不应成为业务状态容器。

### 3.6 `engine/network`

职责：

- `codec.Packet`；
- Session；
- TCPServer；
- NetComponent；
- KCP service/channel；
- 网络地址解析。

业务模块只关心 packet handler 或 MessageSender，不直接操作 UDP frame。

### 3.7 `engine/network/peer`

职责：

- NetInner KCP listener 和主动连接；
- ProcessID/协议版本/nonce/HMAC handshake；
- `network.Session` 的读写循环和关闭回调；
- `ProcessOuterSender.AddSession/RemoveSession`；
- 业务 `MessageSender` 的进程级外发器解析；
- 固定方向的 reconnect/backoff；
- NetInner FiberInit，以及 `cmd/server` 隐式 NetInner 所需的运行时组件。

该包位于基础网络包之上，依赖 `engine/actor`；基础
`engine/network` 不反向依赖 Actor，避免 Go 测试包产生 import cycle。

### 3.8 `db`

职责：

- MongoDB Client；
- DBManager 按 Zone 建立 DBComponent；
- CRUD、Increment、Save/Remove；
- 持久化实体接口。

DB 不应把 MongoDB 连接失败转换为默认内存成功。

### 3.8 `module`

`module` 按服务边界和领域能力分组：

| 分组 | 包 |
|---|---|
| 服务 | `actorlocation`、`central`、`login`、`gate`、`http`、`router` |
| 地图运行时 | `map_`、`maprpc`、`unit`、`aoi`、`move`、`statesync` |
| 领域 | `inventory`、`numeric`、`lockstep`、`crontab` |
| 辅助 | `gamelogin` |

这里的包名是实现边界，不是文档的主叙事单位；业务流程文档应从请求和状态
流转描述它们。不要把每个 ET/C# package 机械映射成一个 Go 服务或一个
独立运行单元。

### 3.9 `proto`

包含：

- `.proto` 源文件；
- `proto/*pb/*.pb.go`；
- `proto/types.go`；
- 各业务包中将业务结构映射到 wire 结构的 `proto_codec.go`。

生成类型是协议边界，不直接暴露给领域状态。

## 4. 初始化注册模式

Fiber 初始化使用：

```go
func init() {
    fiber.RegisterFiberInit(ecs.SceneTypeMap, initMapFiber)
}
```

静态启动使用 `Manager.CreateConfigured`；动态 Room 使用
`Manager.CreateWithSetup`。两者的顺序分别是：

```text
New Fiber
  → 设置根 Scene ID/Name
  → FiberInitHandler
  → 动态 setup（仅 CreateWithSetup）
  → 注册到 Manager
  → 启动 loop
```

`CreateWithSetup` 用于在 Fiber goroutine 启动前注入动态 Room、Map target
等运行时依赖，避免调用方在 Fiber 已经运行后直接读写根 Scene。

因此 FiberInit 可以安全使用：

- `scene.ID()`；
- `scene.Name()`；
- `network.ResolveSceneListenAddr`；
- `actor.UpdateSceneRegistry`。

业务包的 `init()` 只负责注册函数，不代表运行时一定创建了对应 Scene。

## 5. 依赖注入约束

### 5.1 生产依赖

生产路径必须显式拥有：

- `MessageSender`；
- `LocationProxyComponent`；
- `UnitComponent`；
- `AOIManagerComponent`；
- `DBManagerComponent`；
- `RoomManagerComponent` 的 Fiber Manager；
- HTTP/Network 的监听地址；
- Pathfinding 的真实 Finder；
- Lockstep 的 SnapshotProvider。

缺少依赖直接返回错误。不要写成：

```go
if component == nil {
    component = NewInMemoryComponent()
}
```

### 5.2 测试依赖

测试可以注入：

- `PlayerProfileStore`；
- `AccountStore`；
- `MessageSender` stub；
- `LocationProxy` stub；
- 测试中的 `Finder` stub；
- `SnapshotProvider` stub。

测试替身必须通过显式组件或函数参数注入，不能让生产代码自动识别“测试环境”。

## 6. 处理器边界

一个协议处理器的推荐结构：

```text
wire bytes
  → unmarshal
  → 校验 req/依赖/权限
  → 调用领域服务
  → 处理错误
  → marshal response
```

错误分三类：

1. **协议/依赖错误**：返回 Go `error`，由 MailBox/RPC 层处理；
2. **业务拒绝**：返回带 `Error/Message` 的协议响应；
3. **异步失败**：记录日志并通过已有消息通道回报，不能用 `_ =` 丢掉。

## 7. 测试边界

| 包层 | 测试重点 |
|---|---|
| `engine/ecs` | Entity 树、组件生命周期、Dispose |
| `engine/fiber` | 邮箱背压、task queue、帧结束任务、停止 |
| `engine/actor` | Actor 地址、同进程 RPC、跨进程 envelope/接收 |
| `engine/network` | Packet、Session、TCP/KCP 回环、地址错误 |
| `engine/network/peer` | peer handshake、KCP Session、Actor RPC、重连 |
| `module/actorlocation` | lock、TryGet、ownership 切换、锁超时 |
| `module/login` | Realm→Gate→Central→Map 对齐 |
| `module/map_` | 进入地图、迁移两阶段提交、组件完整性 |
| `module/lockstep` | 匹配、动态 Room、确定性 `LSWorld`、帧输入、v2 快照与恢复 |
| `db` | 配置错误、CRUD、真实 MongoDB integration |

测试通过只说明对应边界的证据，不自动证明整套部署可用。
