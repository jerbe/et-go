# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目简介

ET-Go 是一个 Go 语言分布式游戏服务器框架，从 C# ET-9.0-Server 框架重构而来。采用 ECS（Entity-Component-System）+ Actor 模型 + Fiber 轻量线程的架构，面向 MMO 游戏场景。

原始 C# 项目路径：`/mnt/data/github/jerbe/ET-9.0-Server`
原始项目分析文档：`ET-9.0-Server项目分析文档.md`

## 构建与运行

```bash
go build ./...                                            # 构建所有包
go build -o bin/server ./cmd/server                       # 构建服务器二进制
go run ./cmd/server --process=1 --config=data/config/json --log-level=debug  # 运行
go test ./...                                             # 全部测试
go test ./engine/ecs/...                                  # 单包测试
go vet ./...                                              # 静态检查
```

## 架构概览

```
应用层 (module/*)        ← 业务逻辑：登录、地图、背包、数值、AOI 等
Actor 消息层 (engine/actor) ← Actor 寻址、MailBox 消息队列、RPC
Fiber 执行层 (engine/fiber) ← goroutine + channel 实现的轻量线程
ECS 框架层 (engine/ecs)    ← Entity/Component/System 数据驱动架构
网络传输层 (engine/network) ← TCP/KCP Session、Protobuf 编解码
基础设施层 (db/, config/)  ← MongoDB、JSON 配置、日志
```

## 目录约定

- `cmd/` — 可执行程序入口
- `engine/` — 框架层（ECS、Fiber、Actor、网络、定时器、对象池、协程锁、ID 生成）
- `module/` — 业务模块（login、gate、map_、central、inventory、numeric、aoi 等）
- `db/` — 数据库层（MongoDB）
- `config/` — 配置加载
- `proto/` — Protobuf 协议定义
- `data/` — 运行时配置数据（JSON、Excel）
- `internal/` — 内部工具包（日志等）

`module/map_` 用下划线后缀避免与 Go 内置 `map` 冲突。

## 编码规范

- 日志使用 `log/slog` 结构化日志（`internal/log`），不要使用 `fmt.Println`
- 组件必须实现 `ecs.Component` 接口，嵌入 `ecs.BaseComponent`
- 错误变量使用 `Err` 前缀（如 `ErrHandlerNotFound`），定义在各包的 `errors.go` 中
- 所有公开类型和函数必须有中文注释说明用途
- 跨 goroutine 共享状态使用 `sync.RWMutex`，单生产者-消费者场景用 channel
- 数值类浮点统一用 int64 存储（乘以 10000），读取时除以 10000

---

# 模块接口规范

以下为各模块必须实现的接口、数据结构、协议和业务逻辑，与 C# ET-9.0-Server 对齐。

---

## M01. engine/ecs — ECS 框架核心

### Entity

```go
type Entity struct {
    id          int64                   // 实体 ID（可序列化持久化）
    instanceID  int64                   // 实例 ID（全局唯一，运行时分配）
    parent      *Entity
    children    map[int64]*Entity       // key=instanceID
    components  map[string]Component    // key=Type()
    status      EntityStatus            // 状态标志位
}
```

**EntityStatus 标志位**（对齐 C#）：
```go
const (
    StatusNone                EntityStatus = 0
    StatusIsFromPool          EntityStatus = 1
    StatusIsRegister          EntityStatus = 1 << 1
    StatusIsComponent         EntityStatus = 1 << 2
    StatusIsNew               EntityStatus = 1 << 3
    StatusIsSerializeWithParent EntityStatus = 1 << 4
)
```

**Entity 方法要求**：
- `AddComponent(c Component)` — 设置 Entity 引用，调用 `AwakeSystem.Awake()`
- `AddComponentWithID(id int64, c Component)` — 指定 ID 添加组件
- `GetComponent(typeName string) (Component, bool)`
- `RemoveComponent(typeName string)` — 调用 `DestroySystem.OnDestroy()`
- `AddChild(child *Entity)` / `AddChildWithID(id int64, child *Entity)`
- `RemoveChild(instanceID int64)`
- `Dispose()` — 递归销毁子实体和组件，从父实体移除

**生命周期顺序**（严格对齐）：
1. 创建 Entity，分配 InstanceID
2. 设置 Parent，注册到 Scene
3. `AwakeSystem.Awake()` 调用
4. 每帧：`UpdateSystem.Update()` → `LateUpdateSystem.LateUpdate()`
5. 销毁：`DestroySystem.OnDestroy()` → 清理引用

### System 接口

```go
type AwakeSystem interface { Awake() }
type UpdateSystem interface { Update() }
type LateUpdateSystem interface { LateUpdate() }
type DestroySystem interface { OnDestroy() }
type SerializeSystem interface { Serialize() ([]byte, error) }
type DeserializeSystem interface { Deserialize(data []byte) error }
type TransferSystem interface { /* 跨进程迁移时序列化 */ }
```

### Scene

```go
type Scene struct {
    Entity
    sceneType SceneType
    zone      int
    name      string
    fiber     *fiber.Fiber  // 所属 Fiber 引用
}
```

### SceneType 枚举（完整对齐）

```go
const (
    SceneTypeMain      SceneType = 1001  // PackageType.Core(1)*1000+1
    SceneTypeLaunch    SceneType = 1002
    SceneTypeNetInner  SceneType = 1003
    SceneTypeNetClient SceneType = 1004
    SceneTypeRealm     SceneType = 9003  // PackageType.Login(9)*1000+3
    SceneTypeGate      SceneType = 9004  // PackageType.Login(9)*1000+4
    SceneTypeMap       SceneType = 18001 // PackageType.Map(18)*1000+1
    SceneTypeCentral   SceneType = 20001 // Central
    SceneTypeRouter    SceneType = 9001  // RouterManager
    SceneTypeRouterNode SceneType = 9002 // Router
    SceneTypeHTTP      SceneType = 16001
    SceneTypeLockStep  SceneType = 11001
    SceneTypeMatch     SceneType = 11002
    SceneTypeRoom      SceneType = 11003
)
```

### World

全局单例注册中心，管理 `map[string]any`。必须线程安全。

---

## M02. engine/fiber — Fiber 轻量线程

### Fiber

```go
type Fiber struct {
    ID        int64
    Zone      int
    ProcessID int
    SceneType ecs.SceneType
    Root      *ecs.Scene
    Mailbox   chan Message        // 容量 1024
    ctx       context.Context
    cancel    context.CancelFunc

    updateSystems     []ecs.UpdateSystem
    lateUpdateSystems []ecs.LateUpdateSystem
}
```

**Fiber 主循环**（对齐 C# Fiber.Update/LateUpdate）：
1. 从 Mailbox channel 取消息 → 分发到 MailBox handler
2. 遍历 updateSystems 执行 Update()
3. 遍历 lateUpdateSystems 执行 LateUpdate()
4. 处理 frameFinishTasks 队列

### FiberManager

```go
func (m *Manager) Create(sceneType, zone, processID int, handler MessageHandler) *Fiber
func (m *Manager) Get(id int64) (*Fiber, bool)
func (m *Manager) Remove(id int64)
func (m *Manager) StopAll()
```

**FiberInit 模式** — 每种 SceneType 注册初始化函数：
```go
type FiberInitHandler func(f *Fiber) error
var fiberInitHandlers map[ecs.SceneType]FiberInitHandler
```

---

## M03. engine/actor — Actor 模型与消息

### ActorID

```go
type ActorID struct {
    ProcessID  int    // 进程 ID
    FiberID    int64  // Fiber ID
    InstanceID int64  // Entity 实例 ID
}
type Address struct {
    ProcessID int
    FiberID   int64
}
```

### MailBox

```go
type MailBoxType int
const (
    MailBoxTypeUnOrderedMessage MailBoxType = 1001 // Core*1000+1
    MailBoxTypeGateSession      MailBoxType = 9001 // Login*1000+1
)
```

**MailBox 组件**：
- 每个拥有 MailBox 的 Entity 可接收 Actor 消息
- `Add(fromAddress Address, msg MessageObject)` — 入队并触发分发
- 按 MailBoxType 路由到不同 handler

### 消息分发链

```
同进程: MessageSender.Send(actorID, msg)
  → ProcessInnerSender → MessageQueue → Fiber.Mailbox → MailBoxComponent.Dispatch

跨进程: MessageSender.Send(actorID, msg)
  → 包装为 A2NetInner_Message → ProcessOuterSender → Session.Send → 网络传输
  → 远端 NetInner Fiber → ProcessOuterSender → ProcessInnerSender → 目标 Actor
```

### RPC 系统

- 每个 Session/Sender 维护 `map[uint32]RpcCallback` 记录待回复请求
- RpcID 递增，超时 40 秒返回 `ERR_MessageTimeout`
- 响应通过 RpcID 匹配

---

## M04. engine/network — 网络传输层

### Session

```go
type Session struct {
    ID           int64
    Conn         net.Conn
    RpcID        uint32                       // 递增
    Callbacks    map[uint32]chan *codec.Packet // RPC 回调
    LastRecvTime int64
    LastSendTime int64
    UserData     any
}
```

**Session 生命周期**：
1. Accept → 添加 `SessionAcceptTimeoutComponent`（5 秒认证超时）
2. Accept → 添加 `SessionIdleCheckerComponent`（每 2 秒检查，40 秒无活动断开）
3. 认证成功 → 移除 AcceptTimeout
4. 断开 → `SessionPlayerComponent.OnDestroy()` 发送 `G2M_SessionDisconnect` 通知 Map

### 协议编解码

```
包格式: [PacketType(1) | MsgID(2) | RpcID(4) | PayloadLen(4) | Payload(...)]
HeaderSize = 11 bytes
MaxPayloadSize = 16MB
```

### KCP 参数（对齐 C#）

```go
// Inner（进程间通信）: NoDelay=1, Interval=10, Resend=2, NC=1, WndSend=1024, WndRecv=1024, MTU=1400, MinRTO=30
// Outer（客户端通信）: NoDelay=1, Interval=10, Resend=2, NC=1, WndSend=256, WndRecv=256, MTU=470, MinRTO=30
// ConnectTimeout = 20000ms
```

### KCP 协议类型

```go
const (
    KcpSYN              = 1  // 连接请求
    KcpACK              = 2  // 连接确认
    KcpFIN              = 3  // 断开连接
    KcpMSG              = 4  // 数据消息
    KcpRouterReconnSYN  = 5  // 路由重连请求
    KcpRouterReconnACK  = 6  // 路由重连确认
    KcpRouterSYN        = 7  // 路由连接
    KcpRouterACK        = 8  // 路由确认
)
```

---

## M05. engine/event — 事件总线

```go
type Bus struct {
    handlers map[EventID][]Handler
}
func (b *Bus) Subscribe(id EventID, h Handler)
func (b *Bus) Publish(id EventID, args any)
```

---

## M06. module/login — 登录认证系统

### 完整登录流程（5 步）

```
Step1: 客户端 → Central (HTTP /login)
  R2Central_AccountLogin{Username, Password}
  → 查询 MongoDB "account" 集合
  → MD5 哈希密码比对
  → 生成 AccessToken: "{randomUInt64}|{accountId}:{startSec}:{expireSec}"
  → 加密: UTF8 → Base64(key="whosyourdaddy") → Hex
  → 返回 AccessToken

Step2: 客户端 → Realm (C2R_Login)
  C2R_Login{AccessToken, ZoneId}
  → SimpleTokenHelper.Verify(token) 解密验证
  → CoroutineLock 锁定 AccountId（防并发登录）
  → 计算 GateID = zoneGates[accountId % gateCount]
  → RPC 调用 R2G_GateAssign 到 Gate
  → 返回 R2C_Login{Address=gate地址, GateId, Token}
  → 1 秒后关闭 Session

Step3: Gate 处理 R2G_GateAssign
  → 生成 Gate Token: "{5位随机}.{accountId}.{4位随机}"
  → Hex 编码存入 GateSessionKeyComponent
  → 20 秒自动清理过期 Token
  → 返回 G2R_GateAssign{GateId, Token}

Step4: 客户端 → Gate (C2G_LoginGate)
  C2G_LoginGate{Token, GateId}
  → 验证 Token（从 GateSessionKeyComponent 查找）
  → 查询/创建玩家 Player 实体
  → 注册 3 个位置:
    - LocationType.Unit(9001)   → 当前地图 ActorID
    - LocationType.Player(9002) → 当前 Player FiberID
    - LocationType.GateSession(9003) → 当前 Session FiberID
  → 关联 Session ↔ Player (双向引用)
  → 移除 SessionAcceptTimeoutComponent
  → 返回 G2C_LoginGate{PlayerId, CharacterCount}

Step5: 客户端 → Gate (C2G_Ping)
  → 返回 G2C_Ping{Time=当前服务器时间}
```

### Proto 消息定义

```protobuf
// --- LoginOuter_C_2000 ---
message C2R_Login {          // ISessionRequest
  int32 RpcId = 1;
  string AccessToken = 2;
  int32 ZoneId = 3;
}
message R2C_Login {          // ISessionResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
  string Address = 4;
  int64 GateId = 5;
  string Token = 6;
}

// --- LoginOuter_C_2100 ---
message C2G_LoginGate {      // ISessionRequest
  int32 RpcId = 1;
  string Token = 2;
  int64 GateId = 3;
}
message G2C_LoginGate {      // ISessionResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
  int64 PlayerId = 4;
  int64 CharacterCount = 5;
}
message C2G_Ping {           // ISessionRequest
  int32 RpcId = 1;
}
message G2C_Ping {           // ISessionResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
  int64 Time = 4;
}

// --- LoginInner_S_22000 ---
message R2G_GateAssign {     // IRequest
  int32 RpcId = 1;
  int64 AccountId = 2;
}
message G2R_GateAssign {     // IResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
  int64 GateId = 4;
  string Token = 5;
}
message G2M_SessionDisconnect { // ILocationMessage
  int32 RpcId = 1;
}
```

### 组件定义

```go
// GateSessionKeyComponent — Token 存储（挂载于 Gate Scene）
type GateSessionKeyComponent struct {
    sessionKey map[string]int64  // hexToken → accountId
}
// 方法: Add(token, accountId), Get(token) int64, Remove(token)
// 自动清理: 20 秒后 Remove

// PlayerComponent — 玩家注册表（挂载于 Gate Scene）
type PlayerComponent struct {
    players map[int64]*Player  // accountId → Player
}

// Player — 玩家实体（ChildOf PlayerComponent）
type Player struct {
    ecs.Entity
    AccountId int64
    UnitId    int64
}

// PlayerSessionComponent — Player → Session 双向关联
type PlayerSessionComponent struct {
    Session *network.Session
}

// SessionPlayerComponent — Session → Player 双向关联（Destroy 时发 G2M_SessionDisconnect）
type SessionPlayerComponent struct {
    Player *Player
}
```

### Token 加解密算法

```go
// 加密: 明文 → UTF8 bytes → Base64Encrypt(key) → HexEncode
// 解密: HexDecode → Base64Decrypt(key) → UTF8 string → 解析
// Key = "whosyourdaddy"
// 格式: "{randomUInt64}|{accountId}:{startUnixSec}:{expireUnixSec}"
// 有效期: 7 天
```

### Gate Fiber 初始化组件列表

```
Scene.AddComponent: MailBoxComponent(UnOrderedMessage), TimerComponent,
  CoroutineLockComponent, ProcessInnerSender, MessageSender,
  LocationProxyComponent, MessageLocationSenderComponent,
  GateSessionKeyComponent, PlayerComponent, NetComponent(KCP/UDP)
```

### Realm Fiber 初始化组件列表

```
Scene.AddComponent: MailBoxComponent(UnOrderedMessage), TimerComponent,
  CoroutineLockComponent, ProcessInnerSender, MessageSender,
  LocationProxyComponent, DBManagerComponent, NetComponent(KCP/UDP)
```

---

## M07. module/central — 中心服务器

### Proto 消息

```protobuf
// CentralInner_S_50000
message R2Central_AccountLogin { // IRequest
  int32 RpcId = 1;
  string Username = 2;
  string Password = 3;
}
message Central2R_AccountLogin { // IResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
  string AccessToken = 4;
}
```

### 处理逻辑

1. 查询 MongoDB `account` 集合：`Username == req.Username AND PasswordHash == MD5(req.Password)`
2. 未找到返回 `ERR_UsernameOrPasswordIncorrectError (100009002)`
3. 生成 AccessToken（格式同上，有效期 7 天）
4. 返回 AccessToken

### Fiber 初始化

```
Scene.AddComponent: MailBoxComponent(UnOrderedMessage), TimerComponent,
  CoroutineLockComponent, ProcessInnerSender, MessageSender,
  LocationProxyComponent, MessageLocationSenderComponent, DBManagerComponent
```

---

## M08. module/gate — 网关服务

Gate 的核心职责由 login 模块的 Gate 端处理器覆盖（C2G_LoginGate、R2G_GateAssign、C2G_Ping）。

**GateSession MailBox 路由规则**：
- MailBoxType.GateSession (9001) 的消息 → 转发到 PlayerSessionComponent.Session.Send()
- 即：发给玩家 Actor 的消息通过 GateSession 邮箱中继到客户端 TCP 连接

---

## M09. module/unit — 游戏单位

### Unit 实体

```go
type Unit struct {
    ecs.Entity
    ConfigId int32         // 配置表 ID
    position math.Float3   // 同步属性，setter 触发 ChangePosition 事件
    rotation math.Quaternion // 同步属性，setter 触发 ChangeRotation 事件
}
// Forward() math.Float3 — 从 Rotation 计算的朝向
```

### UnitType 枚举

```go
const (
    UnitTypePlayer  = 5001  // PackageType.Unit(5)*1000+1
    UnitTypeMonster = 5002
    UnitTypeNPC     = 5003
)
```

### UnitComponent（挂载于 Scene）

```go
type UnitComponent struct {
    units map[int64]*Unit  // unitId → Unit
}
// 方法: Add(unit), Get(id) *Unit, Remove(id)
```

### 事件

```go
type ChangePosition struct { Unit *Unit; OldPos math.Float3 }
type ChangeRotation struct { Unit *Unit }
```

### UnitFactory — 创建玩家单位

```go
func CreatePlayer(scene *ecs.Scene, id int64) *Unit {
    unit := scene.UnitComponent.AddChildWithID(id, &Unit{ConfigId: 1001})
    unit.AddComponent(&MoveComponent{})
    unit.Position = Float3{-10, 0, -10}

    numeric := &NumericComponent{}
    unit.AddComponent(numeric)
    numeric.SetFloat(NumericTypeSpeed, 6.0)     // 6 m/s
    numeric.Set(NumericTypeAOI, 15000)           // 15000 世界单位

    unit.AddComponent(&AOIEntity{ViewDistance: 9000, Pos: unit.Position})
    return unit
}
```

---

## M10. module/numeric — 数值属性系统

### NumericComponent（挂载于 Unit，实现 TransferSystem）

```go
type NumericComponent struct {
    ecs.BaseComponent
    NumericDic map[int]int64  // NumericType → 值
}
```

### 数值类型常量（精确对齐）

```go
const NumericTypeMax = 10000  // >= Max 的类型需要重算

// 速度 (1000)
const (
    Speed         = 1000
    SpeedBase     = 10001  // 1000*10+1
    SpeedAdd      = 10002
    SpeedPct      = 10003
    SpeedFinalAdd = 10004
    SpeedFinalPct = 10005
)

// 生命值 (1001)
const (
    Hp     = 1001
    HpBase = 10011
)

// 最大生命值 (1002)
const (
    MaxHp         = 1002
    MaxHpBase     = 10021
    MaxHpAdd      = 10022
    MaxHpPct      = 10023
    MaxHpFinalAdd = 10024
    MaxHpFinalPct = 10025
)

// AOI 视距 (1003)
const (
    AOI         = 1003
    AOIBase     = 10031
    AOIAdd      = 10032
    AOIPct      = 10033
    AOIFinalAdd = 10034
    AOIFinalPct = 10035
)
```

### 计算公式

```
final = (((base + add) * (100 + pct) / 100) + finalAdd) * (100 + finalPct) / 100

子类型映射: 给定最终类型 T (如 1000)
  base     = T*10 + 1
  add      = T*10 + 2
  pct      = T*10 + 3
  finalAdd = T*10 + 4
  finalPct = T*10 + 5
```

### 值存储约定

- **浮点值存储**：`int64(value * 10000)`，读取时 `/10000.0`
- `GetAsFloat(t int) float64` → `NumericDic[t] / 10000.0`
- `GetAsInt(t int) int32` → `int32(NumericDic[t])`
- `Set(t int, v float64)` → `NumericDic[t] = int64(v * 10000)`

### 事件链

1. `Set()` 改变子属性值
2. 如果 finalType >= Max → 调用 `recalculate(finalType)`
3. 如果值变化且 type < Max → 发布 `NumericChange{Unit, Type, Old, New}` 事件
4. `NumericWatcher` 响应事件（如血量变化 UI 通知）

### INumericWatcher 接口

```go
type INumericWatcher interface {
    Run(unit *Unit, numType int, old, new_ int64)
}
// 按 NumericType 注册 watcher，变化时自动调用
```

---

## M11. module/aoi — AOI 视野管理

### AOIManagerComponent（挂载于 Map Scene）

```go
const CellSize = 10000  // 10*1000 世界单位

type AOIManagerComponent struct {
    cells   map[int64]*Cell     // cellId → Cell
    // cellId = (int(x*1000)/CellSize) << 32 | (int(z*1000)/CellSize)
}
```

### AOIEntity（挂载于 Unit）

```go
type AOIEntity struct {
    ViewDistance int            // 视距（世界单位），默认 9000
    Cell        *Cell          // 当前所在格子

    SeeUnits     map[int64]*AOIEntity  // 能看到的单位
    BeSeeUnits   map[int64]*AOIEntity  // 被哪些单位看到
    SeePlayers   map[int64]*AOIEntity  // 能看到的玩家（子集）
    BeSeePlayers map[int64]*AOIEntity  // 被哪些玩家看到（子集）
}
```

### Cell

```go
type Cell struct {
    X, Z              int
    AOIUnits          map[int64]*AOIEntity  // 物理在此格子的单位
    SubsEnterEntities map[int64]*AOIEntity  // 订阅进入事件的实体
    SubsLeaveEntities map[int64]*AOIEntity  // 订阅离开事件的实体
}
```

### 网格算法

```
进入半径 r = (ViewDistance - 1) / CellSize + 1
离开半径 leaveR = r + (如果是 Player 则 +1，否则 +0)  // 滞后：离开距离 > 进入距离，防止抖动

EnterCells = 以单位所在格子为中心，半径 r 内的所有格子
LeaveCells = 以单位所在格子为中心，半径 leaveR 内的所有格子
```

### 可见性规则

```
Player→Player: 双向（SeeUnits, SeePlayers, BeSeeUnits, BeSeePlayers 都更新）
Player→NPC:    单向（Player.SeeUnits += NPC, NPC.BeSeeUnits += Player）
NPC→Player:    单向（NPC.SeePlayers += Player, Player.BeSeePlayers += NPC）
NPC→NPC:       单向（SeeUnits, BeSeeUnits）
```

### 事件

```go
type UnitEnterSightRange struct { A, B *AOIEntity }  // A 看到 B 进入视野
type UnitLeaveSightRange struct { A, B *AOIEntity }  // A 看到 B 离开视野
```

### 与 ChangePosition 联动

Unit.Position 变化 → ChangePosition 事件 → 重算 Cell → AOIManager.Move() → 触发 Enter/Leave 事件

---

## M12. module/move — 移动寻路

### MoveComponent（挂载于 Unit）

```go
type MoveComponent struct {
    Speed           float32       // m/s
    StartTime       int64         // 起始时间
    BeginTime       int64         // 当前段开始时间
    StartPos        math.Float3   // 当前段起始位置
    NeedTime        int64         // 到达下一个目标点的时间(ms)
    Targets         []math.Float3 // 路径点列表
    N               int           // 当前目标点索引
    TurnTime        int           // 旋转插值时间(ms)，0=立即
    IsTurnHorizontal bool
    From            math.Quaternion  // 旋转起始
    To              math.Quaternion  // 旋转目标
}
```

### 方法

```go
func (mc *MoveComponent) MoveToAsync(targets []math.Float3, speed float32, turnTime int) error
func (mc *MoveComponent) ChangeSpeed(speed float32) bool
func (mc *MoveComponent) IsArrived() bool
func (mc *MoveComponent) Stop(ret bool)
func (mc *MoveComponent) FlashTo(target math.Float3) bool
```

### 移动插值算法

```
每帧 Update:
  elapsed = now - BeginTime
  if elapsed < NeedTime:
    t = elapsed / NeedTime
    position = Lerp(StartPos, Targets[N], t)
    if TurnTime > 0: rotation = Slerp(From, To, min(1, elapsed/TurnTime))
  else:
    移动到 Targets[N]
    N++
    if N >= len(Targets): MoveFinish()
    else: 计算下一段 NeedTime = distance / Speed * 1000
```

### 事件

```go
type MoveStart struct { Unit *Unit }
type MoveStop  struct { Unit *Unit }
```

### PathfindingComponent（挂载于 Unit）

```go
type PathfindingComponent struct {
    MapName string        // 地图名称
    // 使用 Recast/Detour 导航网格
    // extents: (15, 10, 15)
    // MaxPolys: 256
}
func (pc *PathfindingComponent) Find(start, target math.Float3) []math.Float3
// 坐标转换: 传入时 x 取反 (-x, y, z)，返回时 x 取反
```

---

## M13. module/map_ — 地图场景

### Fiber 初始化

```
Scene.AddComponent: MailBoxComponent(UnOrderedMessage), TimerComponent,
  CoroutineLockComponent, ProcessInnerSender, MessageSender,
  LocationProxyComponent, MessageLocationSenderComponent,
  RoomManagerComponent, DBManagerComponent, MapUnitManagerComponent,
  UnitComponent, AOIManagerComponent
```

### Proto 消息

```protobuf
// MapOuter_C_2500
message C2M_TransferMap {    // ILocationRequest
  int32 RpcId = 1;
}
message M2C_TransferMap {    // ILocationResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
}

// MapInner_S_22500
message M2M_UnitTransferRequest {  // IRequest
  int32 RpcId = 1;
  ActorId OldActorId = 2;
  bytes Unit = 3;                  // BSON 序列化的 Unit
  repeated bytes Entitys = 4;     // BSON 序列化的 TransferSystem 组件
}
message M2M_UnitTransferResponse { // IResponse
  int32 RpcId = 1; int32 Error = 2; string Message = 3;
}
```

### 地图转移流程

```
C2M_TransferMap:
  1. 确定目标地图（Map1 ↔ Map2）
  2. TransferAtFrameFinish(unit, targetActorId, targetMapName)

TransferAtFrameFinish:
  1. 等待帧结束 (WaitFrameFinish)
  2. Location.Lock(LocationType.Unit, unitId, oldActorId)
  3. 序列化 Unit 为 BSON
  4. 序列化所有 TransferSystem 组件为 BSON
  5. unit.Dispose()
  6. 发送 M2M_UnitTransferRequest 到新地图

M2M_UnitTransferRequest 处理:
  1. BSON 反序列化 Unit
  2. 添加到 UnitComponent
  3. 反序列化 TransferSystem 组件并添加到 Unit
  4. 添加 MoveComponent, PathfindingComponent
  5. 设置 Position = (-10, 0, -10)
  6. 添加 MailBoxComponent(OrderedMessage)
  7. 发送 M2C_StartSceneChange 通知客户端
  8. 发送 M2C_CreateMyUnit
  9. 添加 AOIEntity(distance=9000)
  10. Location.Unlock(unitId, newActorId)
```

### UnitDumperComponent — 自动存储

```
间隔: 5 分钟 (300000ms)
行为: 周期性保存所有 IDBEntityCollection 组件到 MongoDB
```

---

## M14. module/statesync — 状态同步

### Proto 消息

```protobuf
message C2M_EnterMap { int32 RpcId = 1; }  // ISessionRequest
message M2C_EnterMap { int32 RpcId = 1; int32 Error = 2; string Message = 3; }

message UnitInfo {
  int64 UnitId = 1;
  int32 ConfigId = 2;
  int32 Type = 3;
  float3 Position = 4;
  float3 Forward = 5;
  map<int32, int64> KV = 6;    // NumericType → value
  MoveInfo MoveInfo = 7;
}
message MoveInfo {
  repeated float3 Points = 1;
  quaternion Rotation = 2;
  int32 TurnSpeed = 3;
}

message M2C_CreateUnits { repeated UnitInfo Units = 1; }   // 视野内出现新单位
message M2C_CreateMyUnit { UnitInfo Unit = 1; }             // 自己的单位信息
message M2C_RemoveUnits { repeated int64 Units = 1; }       // 视野内移除单位
message M2C_StartSceneChange { int64 SceneInstanceId = 1; string SceneName = 2; }

message C2M_PathfindingResult {  // ILocationMessage
  int32 RpcId = 1;
  float3 Position = 2;          // 目标位置
}
message C2M_Stop { int32 RpcId = 1; }  // ILocationMessage

message M2C_PathfindingResult {  // IMessage
  int64 Id = 1;
  float3 Position = 2;
  repeated float3 Points = 3;
}
message M2C_Stop {               // IMessage
  int32 Error = 1;
  int64 Id = 2;
  float3 Position = 3;
  quaternion Rotation = 4;
}
```

### 移动处理流程

```
C2M_PathfindingResult Handler:
  speed = unit.NumericComponent.GetAsFloat(Speed)
  if speed < 0.01: SendStop(2); return
  PathfindingComponent.Find(currentPos, targetPos) → points
  if len(points) < 2: SendStop(3); return
  广播 M2C_PathfindingResult{Id, Position, Points} 给视野内玩家
  MoveComponent.MoveToAsync(points, speed)
  if 到达: SendStop(0)

C2M_Stop Handler:
  MoveComponent.Stop(true)
  广播 M2C_Stop{Id, Position, Rotation}
```

### AOI 事件 → 客户端通知

```
UnitEnterSightRange → 对观察者(Player): 发送 M2C_CreateUnits{UnitHelper.CreateUnitInfo(进入者)}
UnitLeaveSightRange → 对观察者(Player): 发送 M2C_RemoveUnits{离开者.ID}
```

### 广播函数

```go
// Broadcast 发送消息给 unit 的所有 BeSeePlayers
func Broadcast(unit *Unit, msg Message) {
    for _, player := range unit.AOIEntity.BeSeePlayers {
        MessageLocationSender.Send(player.Unit.ID, msg)
    }
}
```

### UnitHelper.CreateUnitInfo

```go
func CreateUnitInfo(unit *Unit) *UnitInfo {
    info := &UnitInfo{
        UnitId: unit.ID, ConfigId: unit.ConfigId,
        Type: unit.UnitType, Position: unit.Position, Forward: unit.Forward,
    }
    // 如果正在移动: 填充 MoveInfo.Points (当前位置 + 剩余目标点)
    // 填充所有 NumericComponent 的 KV
    return info
}
```

---

## M15. module/inventory — 背包物品系统

### Proto 消息

```protobuf
message ItemInfo {
  int64 Id = 1; int32 ConfigId = 2; int32 Count = 3; int32 SlotIndex = 4;
}

message C2M_GetBagInfo { int32 RpcId = 1; }
message M2C_GetBagInfo { int32 RpcId = 1; int32 Error = 2; string Message = 3;
  int32 MaxCapacity = 4; repeated ItemInfo Items = 5;
}

message C2M_BagOperation { int32 RpcId = 1;
  int32 OpType = 2;   // 1=Use, 2=Discard, 3=Swap, 4=Sort
  int64 ItemId = 3; int32 Count = 4; int32 TargetSlot = 5;
}

message C2M_GetWarehouseInfo { int32 RpcId = 1; }
message C2M_WarehouseOperation { int32 RpcId = 1;
  int32 OpType = 2;   // 1=Store, 2=Take, 3=Swap
  int64 ItemId = 3; int32 Count = 4; int32 TargetSlot = 5;
}

message M2C_ItemChange {
  int32 ChangeType = 1;  // 1=Add, 2=Update, 3=Delete
  int32 Container = 2;   // 1=Bag, 2=Warehouse
  ItemInfo Item = 3;
}
```

### Item 实体

```go
type Item struct {
    ecs.Entity
    ConfigId  int32
    Count     int32
    SlotIndex int32
    UniqueId  int64
}
func (i *Item) IsStackable() bool  // Material/Consumable 可堆叠
func (i *Item) GetMaxStackCount() int32  // Material=9999, Consumable=99, 其他=1
```

### ItemType / ItemQuality

```go
const (
    ItemTypeMaterial   = 1   // 可堆叠
    ItemTypeConsumable = 2   // 可堆叠
    ItemTypeWeapon     = 10  // 不可堆叠
    ItemTypeArmor      = 11
    ItemTypeAccessory  = 12
    ItemTypePet        = 20
)
const (
    ItemQualityWhite  = 0  // 普通
    ItemQualityGreen  = 1  // 优良
    ItemQualityBlue   = 2  // 精良
    ItemQualityPurple = 3  // 史诗
    ItemQualityOrange = 4  // 传说
    ItemQualityRed    = 5  // 神话
)
```

### BagComponent（挂载于 Unit）

```go
type BagComponent struct {
    MaxCapacity      int32
    Slots            map[int]int64     // slotIndex → itemId
    ItemSlotMap      map[int64]int     // itemId → slotIndex
    ConfigIdItemsMap map[int][]int64   // configId → []itemId
}
```

**核心方法**：
- `GetEmptySlotCount() int`
- `IsFull() bool`
- `GetFirstEmptySlot() int` — 返回 -1 表示满
- `TryAddItem(configId int32, count int32) (errCode int, items []*Item)`
- `RemoveItem(itemId int64) int`
- `RemoveItemByConfigId(configId int32, count int32) int`
- `SwapSlots(slotA, slotB int) int`
- `SortBag()` — 按 ConfigId 合并整理
- `InitMapsFromChildren()` — DB 加载后重建索引

### WarehouseComponent — 同 BagComponent 结构

额外方法：
- `StoreFromBag(bag, itemId, count) int` — 背包→仓库
- `TakeToBag(bag, itemId, count) int` — 仓库→背包

### 错误码

编码规则：`ERR_WithException(100000000) + PackageType.Inventory(30) * 1000 + 序号`。

```go
const (
    ERR_BagFull                     = 100030001
    ERR_BagItemNotFound             = 100030002
    ERR_BagItemCountNotEnough       = 100030003
    ERR_BagSlotInvalid              = 100030004
    ERR_BagOperationInvalid         = 100030005
    ERR_SessionPlayerError          = 100030006
    ERR_WarehouseFull               = 100030051
    ERR_WarehouseItemNotFound       = 100030052
    ERR_WarehouseItemCountNotEnough = 100030053
    ERR_WarehouseSlotInvalid        = 100030054
    ERR_WarehouseOperationInvalid   = 100030055
    ERR_ItemConfigNotFound          = 100030101
    ERR_ItemCannotStack             = 100030102
    ERR_ItemStackOverflow           = 100030103
)
```

当前 `module/inventory/errors.go` 使用的是 `30001` 等短值（缺 `ERR_WithException`
前缀），且缺失 `ERR_SessionPlayerError` 等 4 个码，与上表不一致。
修复任务见 `docs/modules/M15-inventory/03-tasks.md` 的 T15-FIX。

---

## M16. module/lockstep — 帧同步

### 常量

```go
const (
    MatchCount            = 1     // 匹配人数
    UpdateInterval        = 50    // 每帧 50ms
    FrameCountPerSecond   = 20    // 1000/50
    SaveLSWorldFrameCount = 1200  // 每 1200 帧(60秒)保存快照
)
```

### Proto 消息

```protobuf
message C2G_Match { int32 RpcId = 1; }
message G2C_Match { int32 RpcId = 1; int32 Error = 2; string Message = 3; }
message G2C_NotifyMatchSuccess { ActorId RoomActorId = 2; }

message C2Room_ChangeSceneFinish { int64 PlayerId = 1; }  // 客户端准备就绪
message Room2C_Start {
  int64 StartTime = 1;
  repeated LockStepUnitInfo UnitInfo = 2;
}

message LSInput { TSVector2 V = 1; int32 Button = 2; }
message FrameMessage { int32 Frame = 1; int64 PlayerId = 2; LSInput Input = 3; }
message OneFrameInputs { map<int64, LSInput> Inputs = 1; }

message Room2C_AdjustUpdateTime { int32 DiffTime = 1; }
message C2Room_CheckHash { int64 PlayerId = 1; int32 Frame = 2; int64 Hash = 3; }
message Room2C_CheckHashFail { int32 Frame = 1; bytes LSWorldBytes = 2; }

// 内部消息
message G2Match_Match { int32 RpcId = 1; int64 Id = 2; }
message Match2Map_GetRoom { int32 RpcId = 1; }
message Map2Match_GetRoom { int32 RpcId = 1; int32 Error = 2; string Message = 3;
  ActorId MapActorId = 4; ActorId RoomActorId = 5;
}
```

### 核心数据结构

```go
type Room struct {
    StartTime        int64
    PlayerIds        []int64
    FrameBuffer      *FrameBuffer
    AuthorityFrame   int  // 服务器权威帧
    PredictionFrame  int  // 客户端预测帧
    Replay           *Replay
}

type FrameBuffer struct {
    MaxFrame    int
    FrameInputs []OneFrameInputs
    Snapshots   [][]byte
    Hashs       []int64
}

type Replay struct {
    UnitInfos   []LockStepUnitInfo
    FrameInputs []OneFrameInputs
    Snapshots   [][]byte  // 每 SaveLSWorldFrameCount 帧保存一次
}
```

### 帧同步流程

```
1. 匹配: C2G_Match → G2Match_Match → Match2Map_GetRoom → 创建 Room Fiber
2. 通知: Match2G_NotifyMatchSuccess → G2C_NotifyMatchSuccess
3. 准备: C2Room_ChangeSceneFinish → 等待所有玩家就绪
4. 开始: Room2C_Start{StartTime, UnitInfos}
5. 每帧:
   - 客户端发送 FrameMessage{Frame, PlayerId, Input}
   - 服务器收集所有玩家输入 → OneFrameInputs
   - 广播 OneFrameInputs 给所有客户端
   - 每 SaveLSWorldFrameCount 帧保存 LSWorld 快照
6. Hash 校验: C2Room_CheckHash → 比对 → 不一致发 Room2C_CheckHashFail
```

---

## M17. module/http — HTTP API 服务

### 端点列表

| 方法 | 路径 | 处理器 | 说明 |
|------|------|--------|------|
| POST | `/login` | HttpPostLoginHandler | 账号登录，返回 AccessToken |
| POST | `/register` | HttpPostRegisterHandler | 账号注册 |
| GET | `/get_area_list` | HttpGetAreaListHandler | 获取大区列表 |

### 处理逻辑

**POST /login**：
- 入参: `{Username, Password}`
- 查询 `account` 集合，MD5 哈希密码比对
- 返回 `{AccessToken}` 或错误

**POST /register**：
- 入参: `{Username, Password}`
- 检查用户名唯一性
- 从 `increment` 集合自增 AccountId（起始 1000000）
- 创建 CAccount{Id, Username, PasswordHash=MD5(Password)}

**GET /get_area_list**：
- 从 StartAreaConfigCategory 读取大区列表返回

### HTTP 分发器

```go
type IHttpHandler interface {
    Handle(scene *ecs.Scene, req *http.Request, resp http.ResponseWriter) error
}
// 按 (SceneType, Path) 注册 handler
```

---

## M18. module/router — 消息路由

### 路由节点状态机

```
RouterSYN → 创建 RouterNode(status=Sync)
RouterACK → 设置 InnerConn(status=Msg)
MSG       → 转发消息（校验 outerConn/innerConn）
FIN       → 销毁 RouterNode
```

### RouterNode

```go
type RouterNode struct {
    OuterConn         uint32
    InnerConn         uint32
    ConnectId         uint32
    LastRecvOuterTime int64
    LastRecvInnerTime int64
    Status            RouterStatus  // Sync=0, Msg=1
    LimitCountPerSec  int           // 限流: max 1000/s
}
```

### 超时机制

- Sync 阶段: 10 秒超时
- Msg 阶段: SessionTimeoutTime + 10 秒
- 每秒最多检查 10 个节点

### HTTP 端点

| 路径 | 说明 |
|------|------|
| `/router/list` | 返回 ServerIP、Realms 列表、Routers 列表 |
| `/zone/list` | 返回可用分区列表 |

---

## M19. module/actorlocation — Actor 定位服务

### Proto 消息

```protobuf
message ObjectAddRequest { int32 RpcId=1; int32 Type=2; int64 Key=3; ActorId ActorId=4; }
message ObjectLockRequest { int32 RpcId=1; int32 Type=2; int64 Key=3; ActorId ActorId=4; int32 Time=5; }
message ObjectUnLockRequest { int32 RpcId=1; int32 Type=2; int64 Key=3; ActorId OldActorId=4; ActorId NewActorId=5; }
message ObjectRemoveRequest { int32 RpcId=1; int32 Type=2; int64 Key=3; }
message ObjectGetRequest { int32 RpcId=1; int32 Type=2; int64 Key=3; }
message ObjectGetResponse { int32 RpcId=1; int32 Error=2; string Message=3; ActorId ActorId=5; }
```

### LocationType 枚举

```go
const (
    LocationTypeUnit        = 9001  // 单位所在地图
    LocationTypePlayer      = 9002  // 玩家所在 Fiber
    LocationTypeGateSession = 9003  // 玩家 Gate 会话
    LocationTypeAccount     = 9004  // 账号锁定（防并发登录）
)
```

### LocationOneType

```go
type LocationOneType struct {
    locations map[int64]ActorID         // key → actorId
    lockInfos map[int64]*LockInfo       // key → 锁信息
}
type LockInfo struct {
    LockActorId   ActorID
    CoroutineLock *coroutinelock.Lock
}
```

### 操作语义

- **Add(key, actorId)**: 注册位置
- **Get(key) → actorId**: 查询位置
- **Lock(key, actorId, timeMs)**: 加锁（防并发迁移）
- **Unlock(key, oldActorId, newActorId)**: 解锁并更新位置
- **Remove(key)**: 删除位置记录

---

## M20. db/ — 数据库层

### MongoDB 集合

| 集合名 | Go 类型 | 字段 |
|-------|---------|------|
| `account` | CAccount | Id int64, Username string, PasswordHash string |
| `increment` | CIncrement | Key string, Value int64 |
| `player_profile` | CPlayerProfile | Id int64, ZoneId int32, AccountId int64, ShortId string, CreatedAt time.Time |
| `hero` | CHero | Id int64, Heroes map[int]HeroUnit |

### DBComponent 方法

```go
func (db *DBComponent) FindOne(ctx, id int64, collection string) (any, error)
func (db *DBComponent) Query(ctx, filter bson.M, collection string) ([]any, error)
func (db *DBComponent) Insert(ctx, entity any, collection string) error
func (db *DBComponent) Save(ctx, entity any, collection string) error    // Upsert
func (db *DBComponent) Remove(ctx, id int64, collection string) (int64, error)
func (db *DBComponent) Increment(ctx, field string, inc int64, defaultVal int64) (int64, error)
```

### 并发控制

- 使用 CoroutineLock，lockType = DB
- TaskCount = 32
- lockId = entityId % TaskCount

### DBManagerComponent

```go
// 按 zone 管理多个 DBComponent
func (m *DBManagerComponent) GetZoneDB(zone int) *DBComponent
// 从 StartZoneConfig 读取连接串和库名
```

---

## M21. module/crontab — 定时任务

### CrontabComponent

```go
type CrontabTask struct {
    Name           string
    CronExpression string      // "min hour day month weekday"
    InvokeType     int
    IsRunning      bool
    LastRunTime    *time.Time
}
type CrontabComponent struct {
    Tasks []CrontabTask
}
```

### Cron 表达式

```
格式: "minute hour day month weekday"
minute: 0-59, *, */n, n-m, n,m,...
hour: 0-23
day: 1-31
month: 1-12
weekday: 0-6 (Sunday=0), 7=Sunday

示例:
  "0 * * * *"     每小时整点
  "*/5 * * * *"   每5分钟
  "0 9 * * 1"     每周一上午9点
  "0 0 1 * *"     每月1号零点
```

### 执行逻辑

1. 每 1000ms 触发定时器
2. 检查是否分钟变更（防重复执行）
3. 仅在每分钟的第 0 秒执行
4. 遍历所有 Task，匹配 Cron 表达式
5. 设置 IsRunning=true 防并发
6. 调用注册的 handler
7. 完成后 IsRunning=false

---

## M22. engine/network/kcp — KCP 传输（待实现）

### 对齐参数

```
Inner: NoDelay=1, Interval=10, Resend=2, NC=1, WndSend=1024, WndRecv=1024, MTU=1400, MinRTO=30
Outer: NoDelay=1, Interval=10, Resend=2, NC=1, WndSend=256, WndRecv=256, MTU=470, MinRTO=30
ConnectTimeout: 20000ms
MaxMessageSize: 10000 bytes (超过则分包)
```

### KService 主循环

```
1. TimerOut(now)                   — 检查超时
2. CheckWaitAcceptChannel(now)     — 等待接受超时 20s
3. Recv()                          — 处理收到的包
4. UpdateChannel(now)              — 更新活跃通道
5. Transport.Update()              — 底层传输更新
```

---

## M23. engine/coroutinelock — 协程锁

```go
type Lock struct {
    locks map[string]chan struct{}
}
func (l *Lock) Acquire(ctx context.Context, key string) (release func(), err error)
```

### 使用场景

| LockType | 值 | 用途 |
|---------|---|------|
| Login | 9001 | 防止同一账号并发登录 |
| DB | - | 数据库操作互斥（entityId % 32） |
| Location | - | Actor 位置锁定（迁移时） |

---

## 全局错误码表

```go
// 框架错误
const (
    ERR_Success                  = 0
    ERR_Cancel                   = 错误取消
    ERR_RpcFail                  = RPC异常
    ERR_MessageTimeout           = 消息超时(40s)
    ERR_SessionSendOrRecvTimeout = 会话超时
    ERR_NotFoundActor            = Actor未找到
    ERR_KcpConnectTimeout        = 100205
    ERR_KcpAcceptTimeout         = 100206
    ERR_KcpReadWriteTimeout      = 100207
    ERR_PeerDisconnect           = 100208
    ERR_PacketParserError        = 110005
)

// 业务错误编码规则（对齐 C# ErrorCode.cs）
//   错误码 = ERR_WithException(100000000) + PackageType * 1000 + 序号
const ERR_WithException = 100000000

// 业务错误 — 登录（PackageType.Login = 9）
const (
    ERR_ConnectGateKeyError               = 100009001
    ERR_UsernameOrPasswordIncorrectError  = 100009002
    ERR_TokenInvalidError                 = 100009003
    ERR_TokenExpiredError                 = 100009004
)

// 业务错误 — 背包（PackageType.Inventory = 30）
// 注意：当前 module/inventory/errors.go 用的是 30001 等短值，缺 ERR_WithException
// 前缀，与下表不一致。修复见 docs/modules/M15-inventory/03-tasks.md 的 T15-FIX。
const (
    ERR_BagFull                     = 100030001
    ERR_BagItemNotFound             = 100030002
    ERR_BagItemCountNotEnough       = 100030003
    ERR_BagSlotInvalid              = 100030004
    ERR_BagOperationInvalid         = 100030005
    ERR_SessionPlayerError          = 100030006
    ERR_WarehouseFull               = 100030051
    ERR_WarehouseItemNotFound       = 100030052
    ERR_WarehouseItemCountNotEnough = 100030053
    ERR_WarehouseSlotInvalid        = 100030054
    ERR_WarehouseOperationInvalid   = 100030055
    ERR_ItemConfigNotFound          = 100030101
    ERR_ItemCannotStack             = 100030102
    ERR_ItemStackOverflow           = 100030103
)
```

---

## 核心概念映射总表（C# ET → Go）

| ET 概念 | Go 实现 | 位置 |
|---------|---------|------|
| Entity | `ecs.Entity` | `engine/ecs/entity.go` |
| Component | `ecs.Component` interface | `engine/ecs/component.go` |
| IAwake/IUpdate/IDestroy | Go interface | `engine/ecs/system.go` |
| Scene | `ecs.Scene` | `engine/ecs/scene.go` |
| World | `ecs.World` | `engine/ecs/world.go` |
| Fiber | `fiber.Fiber` (goroutine+channel) | `engine/fiber/fiber.go` |
| FiberManager | `fiber.Manager` | `engine/fiber/manager.go` |
| ActorId | `actor.ActorID` | `engine/actor/actor_id.go` |
| MailBoxComponent | `actor.MailBox` | `engine/actor/mailbox.go` |
| Session | `network.Session` | `engine/network/session.go` |
| ETTask | goroutine + context.Context | 原生 Go |
| CoroutineLock | `coroutinelock.Lock` | `engine/coroutinelock/lock.go` |
| ObjectPool\<T\> | `pool.TypedPool[T]` | `engine/pool/pool.go` |
| TimerComponent | `timer.Manager` | `engine/timer/timer.go` |
| EventSystem | `event.Bus` | `engine/event/bus.go` |
| IdGenerater | `id.Generator` | `engine/id/generator.go` |
| NumericComponent | `numeric.Component` | `module/numeric/component.go` |
| AOIManagerComponent | `aoi.Manager` | `module/aoi/manager.go` |
| BagComponent | `inventory.BagComponent` | `module/inventory/` |
| Unit | `unit.Unit` | `module/unit/` |
| MoveComponent | `move.MoveComponent` | `module/move/` |
| Room (LockStep) | `lockstep.Room` | `module/lockstep/` |
| DBComponent | `db.Client` | `db/client.go` |
| Config | `config.Config` | `config/config.go` |
