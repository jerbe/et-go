# Go 运行时与并发模型

## 1. 运行时对象关系

```text
World
 └── FiberManager
      ├── Fiber 1
      │    └── Root Scene
      │         ├── service Components
      │         ├── Scene Entity children
      │         └── MailBox
      ├── Fiber 2
      └── ...
```

核心关系：

- `World` 管理全局 ECS 资源；
- `FiberManager` 管理 Fiber 生命周期；
- `Fiber` 是 goroutine、消息邮箱和帧循环的组合；
- `Scene` 是 Fiber 的根 Entity；
- `Entity` 持有 Component 和子 Entity；
- `MailBox` 是 Actor 消息进入 Scene/Entity 的入口；
- `UpdateSystem` 和 `LateUpdateSystem` 由 Fiber 在固定帧循环中执行。

## 2. Entity、Scene 和 Component

### 2.1 Entity

`engine/ecs.Entity` 维护：

- 逻辑 `ID`；
- 运行时唯一 `InstanceID`；
- parent/children；
- component map；
- scene 引用；
- disposed 状态。

`ID` 是业务或配置身份，`InstanceID` 是运行时实体身份。Actor 地址中的 `InstanceID` 使用后者。

### 2.2 Component 生命周期

添加组件：

```text
Entity.AddComponent
  → Component.SetEntity
  → 写入组件表
  → 如果实现 AwakeSystem，立即 Awake
```

销毁组件：

```text
Entity.Dispose / RemoveComponent
  → DestroySystem.OnDestroy
  → 清理组件引用
  → 从组件表移除
```

组件 `Awake` 不能假设所有同层组件都已安装。初始化顺序有依赖时，应该在 FiberInit 中先创建依赖，再创建使用方，或在业务方法中做显式校验。

### 2.3 Scene 生命周期

```text
Fiber.New
  → NewScene
  → Scene.SetFiber
  → FiberInit 安装组件
  → 动态 Fiber setup（如 Room 依赖注入）
  → Fiber.Run
  → Fiber.Stop
      → 停止网络/Timer/Update
      → Scene.Dispose
      → 关闭 mailbox 和 task queue
```

`Manager.CreateConfigured` 在 FiberInit 失败时停止并等待 Fiber，不把半初始化 Fiber 返回给调用方。
`Manager.CreateWithSetup` 在启动 loop 前完成动态依赖注入；setup 失败时同样
丢弃 Fiber，不允许调用方与清理过程并发访问 Scene。

`FiberManager.StopAll` 进入关闭状态后拒绝新的 Fiber 创建；重复关闭是幂等的。
Fiber 主循环的消息处理器、Update/LateUpdate、帧结束任务 panic 会被隔离并
记录，随后继续执行或进入统一 dispose；`Fiber.Call` 任务 panic 会转换为
`ErrFiberTaskPanic` 返回给调用方。空的帧结束任务，以及停止后的帧结束任务
注册，会返回明确错误。

Fiber 的关闭边界是单向的：未启动的 Fiber 调用 `Stop` 也会释放 Root Scene、
关闭完成信号，并且不能再次 `Run`；已经排队但尚未执行的 `Fiber.Call` 如果
context 已取消，不会进入业务 handler。这样可以避免“停止后重启半初始化
Scene”以及“调用方已超时但任务仍修改业务状态”。

`RegisterUpdate` 和 `RegisterLateUpdate` 按系统实例去重。组件因为重复
`Awake` 或动态 setup 再次注册时不会叠加同一个 tick；Crontab 会在拥有
Fiber 的 Scene 上自动注册 Update，并在销毁时注销。

## 3. Fiber 主循环

Fiber 同时处理：

```text
消息邮箱
任务队列（Call）
帧 Tick
帧结束任务
```

逻辑抽象：

```text
loop:
  select:
    ctx.Done       → dispose
    mailbox        → 调用 MessageHandler
    task           → 在 Fiber goroutine 执行
    ticker         → Update / LateUpdate / frame-finish
```

### 3.1 单向消息

```go
err := targetFiber.Send(fiber.Message{
    To:      targetInstanceID,
    MsgID:   msgID,
    Payload: payload,
})
```

`Send` 是非阻塞入队：

- Fiber 已关闭：`ErrFiberClosed`；
- mailbox 已满：`ErrMailboxFull`；
- 成功：消息进入目标队列。

队列满不是成功。上层必须选择重试、拒绝请求、断开连接或记录告警。

### 3.2 Fiber Call

同进程 RPC 使用 `Fiber.Call`：

```text
调用方
  → targetFiber.Call
  → task 入目标 Fiber task queue
  → 目标 Fiber 执行 DispatchFiberMessage
  → 返回 payload/error
```

这与旧式“起 goroutine 后直接访问目标 Scene”不同。业务 handler 和目标 Fiber 的 Update/普通消息共享同一个串行边界。

Call 的终止条件：

- 目标 Fiber 执行成功；
- context 取消；
- Fiber 停止；
- RPC timeout。

`Call` 会把调用方 context 带入任务边界：入队前已取消的任务直接返回，目标
Fiber 取出任务时再次检查取消状态。跨组件/跨进程 handler 应继续把 Session
或请求 context 向下传递，不能用新的无限期 `context.Background()` 覆盖调用方
的取消语义。

### 3.3 帧结束任务

Map 切图使用 `AddFrameFinishTask`，而不是在当前 Fiber 内阻塞等待：

```text
客户端切图请求
  → 校验目标
  → 添加 frame-finish task
  → 立即返回 accepted
  → 当前帧结束执行迁移
  → 失败通过玩家消息和日志报告
```

`TransferAtFrameFinish` 仍提供直接等待 API，主要给测试和明确的调用方使用。
`WaitFrameFinish` 在 Fiber 尚未启动、正在停止或已经关闭时会关闭等待
channel；调用方不会因为等待一个永远不会执行的帧任务而永久阻塞。

## 4. 并发所有权

### 4.1 Fiber 内部状态

默认约束：

```text
一个 Scene 的业务状态只能由所属 Fiber 修改。
```

例如：

- Map 的 Unit registry 由 Map Fiber 修改；
- Room 的帧状态由 Room Fiber 修改；
- Location 的类型 map 由 Location Fiber/锁保护；
- Router Node map 由 Router Fiber 修改。

外部 goroutine 需要修改这些状态时，使用：

- `Fiber.Send`；
- `Fiber.Call`；
- `AddFrameFinishTask`；
- 组件自身明确声明的 mutex。

### 4.2 允许共享的状态

以下组件内部使用锁：

- `LocationOneType`；
- `LocationManagerComponent`；
- `MessageLocationSenderComponent`；
- `RoomManagerComponent` 的元数据访问；
- `RpcManager`；
- `FrameBuffer`；
- `Replay`；
- `Session` 的发送/关闭边界；
- Router node registry。

加锁只保护数据结构，不代表可以绕过业务 ownership。例如给 `Unit` 加 mutex 不能证明跨 Fiber 直接改 Unit 是安全的。

### 4.3 goroutine 的使用边界

适合起 goroutine 的场景：

- Session ReadLoop/WriteLoop；
- KCP/TCP accept loop；
- 独立数据库保存；
- 不阻塞 Fiber 的异步任务；
- 明确等待 channel 的通知。

不适合的场景：

- 直接在 goroutine 中修改目标 Scene；
- 用 goroutine 绕过 Fiber 的顺序保证；
- 启动异步任务后丢弃 error；
- 让源 Entity 在远端 RPC 成功前先销毁。

Map 迁移已经改成：

```text
锁定 Location
  → 序列化
  → 目标准备 Entity/AOI/通知
  → 目标完成 Location ownership 切换
  → 返回成功
  → 源地图销毁旧 Entity
```

失败时目标回滚，源保留单位。

组件替换也遵循完整生命周期：同一类型组件被替换时，旧组件先执行销毁并
清除旧的 Entity 引用，再安装新组件；不能让已移除组件继续持有活动 Entity
指针。

## 5. Event、Timer 和锁

### 5.1 EventBus

AOI 通过 Scene EventBus 发布：

- Entity 进入视野；
- Entity 离开视野；
- 位置变化。

订阅返回取消函数，组件销毁时应调用取消函数，避免 Scene 已销毁但回调仍持有 Entity。

### 5.2 TimerComponent

Timer 使用一次性 Timer 或 repeating ticker。底层 ticker 可以在独立
goroutine 触发，但挂载到 Scene 的 TimerComponent 会通过所属 Fiber 的
`Call` 执行 callback，保持 Scene 状态串行；独立 ECS 单测没有 Fiber 时
才直接执行 callback。`WaitAsync` 的等待 channel 在正常触发、移除和
组件销毁时都会关闭；TimerComponent 关闭后不允许重新添加定时器。
Session accept-timeout 和 idle checker 在组件销毁后也不会重新启动 ticker。
销毁时必须停止 ticker，并等待已启动的异步 Save 按组件约定退出。

### 5.3 CoroutineLock

CoroutineLock 用于：

- DB 写入分桶；
- Location key；
- 登录账号；
- Map/Room 业务串行化。

锁超时是错误，不应该返回“未加锁但继续执行”的成功状态。

## 6. ActorID 和 Registry

```go
type ActorID struct {
    ProcessID  int
    FiberID    int64
    InstanceID int64
}
```

`IsValid` 要求三段都大于 0。

`actor.UpdateSceneRegistry(scene)` 记录：

- SceneType；
- Zone；
- 配置 SceneID；
- Name；
- Scene 根 ActorID。

Registry 只解决“当前进程可见的运行时 Scene 引用”。它不是跨进程服务发现，不能替代 ProcessOuterSender 的连接和重连机制。

依赖具体 Unit 的消息必须使用完整 ActorID，而不是只比较 InstanceID。
`C2M_TransferMap` 的目标 Unit 就是这一约束的代表：同一 InstanceID 但
ProcessID/FiberID 不匹配时仍必须拒绝。

## 7. 关闭流程

推荐顺序：

```text
收到 SIGTERM
  → cancel root context
  → FiberManager.StopAll
      → 停止 Fiber loop
      → 停止 Timer/Update
      → 关闭 Network/HTTP/KCP
      → 释放 Scene/Entity/Component
      → 关闭 mailbox/task queue
  → World.Shutdown
```

网络和 DB 的 Close 错误会记录日志。迁移和持久化的异步错误必须可观察。

动态 Room 的关闭由 `RoomServerComponent.LateUpdate` 发起：它在所属 Fiber
内完成离线判定，随后异步调用 `FiberManager.Remove`，避免当前 Fiber 等待
自身退出；没有 Manager 的独立测试 Fiber 只发出非阻塞 `RequestStop`。

## 8. 跨进程运行时边界

跨进程网络编排不在基础 `engine/network` 包内，而由
`engine/network/peer` 子包负责，避免基础网络层依赖 Actor 上层：

```text
FiberInit(NetInner)
  → ProcessPeerComponent.Start
  → KCP Listen
  → Update 驱动收发和重连
  → Session handshake
  → ProcessOuterSender.AddSession
  → Actor envelope → 目标 Fiber queue
```

连接方向固定为较小 `ProcessID` 主动拨号，保证一个 Process 对之间只有一个
稳定的主动连接。Session 关闭时先从 `ProcessOuterSender` 移除，pending RPC
收到 `actor.ErrProcessSessionClosed`；重连由主动拨号方按固定间隔触发。

这部分已经有同机双 peer、HMAC handshake、Actor RPC 和 Session 替换测试。
真实双 OS 进程、跨机器网络、TLS 和故障演练仍属于部署验收，不是基础运行时
实现缺失。

### TODO(lockstep)

Room 的 `SnapshotProvider` 已注入 `RoomStateSnapshot v2`；版本、checksum、
连续帧、Go `Room/FrameBuffer/RoomPlayer/LSWorld` 恢复和 Room/World 帧一致性
已实现。剩余 TODO 是原 C# MemoryPack/TrueSync fixed-point wire、客户端
replay/hash-mismatch 恢复协议和跨版本迁移，不能把 Go JSON snapshot 宣称为
客户端可直接消费的跨语言战斗快照。

### TODO(map)

Map 的 Pathfinding 需要真实 Finder。配置了 `navMeshFile` 时，部署层必须在
Map Fiber 初始化前注册 `move.FinderFactory`，否则启动失败；没有配置
`navMeshFile` 时 Map 可以启动，但寻路请求返回 `ErrFinderMissing`，不会
生成直线路径。
