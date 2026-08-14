# C# / Go 兼容性说明

## 1. 文档定位

ET-Go 不是把 C# ET package 原样翻译为 Go package。Go 实现按照：

```text
cmd
engine
module
db
config
proto
```

组织代码；C# 资料只作为行为和协议参考。

## 2. 可以对照的内容

### Actor 地址

两边都需要区分：

- 逻辑 Process；
- Fiber/执行上下文；
- Entity runtime instance。

Go 使用：

```go
type ActorID struct {
    ProcessID  int
    FiberID    int64
    InstanceID int64
}
```

### 消息

客户端和跨边界消息的兼容点包括：

- MsgID；
- RpcID；
- protobuf field number；
- Error/Message；
- PacketType；
- Map transfer 和 Lockstep snapshot 的 payload 语义。

### 服务职责

| 历史职责 | Go 运行对象 |
|---|---|
| Location | `Location` Scene + `module/actorlocation` |
| Central | `Central` Scene + `module/central` |
| Realm | `Realm` Scene + `module/login` |
| Gate | `Gate` Scene + `module/gate`/`module/login` |
| Map | `Map` Scene + `module/map_` |
| Match/Room | `Match`/`Room` Scene + `module/lockstep` |
| Router | `Router`/`RouterNode` Scene + `module/router` |

这是一张职责映射，不是 Go 的 package 依赖图。

## 3. 已知行为差异

### Fiber RPC

Go 同进程 RPC 经过目标 Fiber 的 task queue：

```text
caller
  → targetFiber.Call
  → target Fiber goroutine
  → DispatchFiberMessage
```

不会直接在新 goroutine 访问目标 Scene。

### Location

Go 的 Location handler 使用 `TryGet`，避免在 Location Fiber 内阻塞等待自身锁。代理端在锁竞争时用 context 重试。

### Map transfer

Go 源端只有在目标成功完成 Location ownership 切换后才销毁本地 Unit。客户端
请求必须由 Gate 路由到真实 Unit MailBox 的完整 ActorID，Map 不按第一个
Unit 或仅按 PlayerID 猜测目标；未知迁移组件会返回错误，内部 transfer
request 的 RpcID/旧 owner ActorID 也会严格校验。

source journal 的 Begin 还要求有效 source ActorID，Unit snapshot 拒绝非正
ID/ConfigID 和未知 UnitType；目标 durable ledger 先持久化 completed 再返回
成功。Go 已提供 `TransferRecoveryCoordinator` 与
`TransferLedgerRecoveryCoordinator` 的严格状态收敛编排，但原 C# 没有可直接
对照的跨进程 recovery token、Location ownership 证明或崩溃恢复协议，因此
真实协调器仍需部署级定义。

### Gate Token 与数据库递增

Realm 生成的 Gate Token 使用 Go `crypto/rand`，但保留 C# 的
`D5.accountID.D4` UTF-8 Hex wire format。随机源失败是登录错误，不注册空
Token。

`DBComponent.Increment` 先用 `$setOnInsert` 确保默认文档，再使用独立原子
`$inc` 返回结果；这保持原 C# 的两步语义，并避免 MongoDB 把同一路径的
`$setOnInsert` 与 `$inc` 识别为更新冲突。

### Pathfinding

Go 缺少真实 Finder 时返回 `ErrFinderMissing`，不生成两点直线路径。

### Lockstep RoomStateSnapshot

Go 当前提供的是 `RoomStateSnapshot v2`。默认 provider 序列化
`Room/FrameBuffer/RoomPlayer/LSWorld`，其中 LSWorld 保存按 PlayerID
确定性排序的单位、Position、Rotation、上一帧 Input、World Frame 和
ID watermark；恢复前校验 checksum、连续帧、Room/World 帧一致性以及
`LSUnit.ID == PlayerID`，校验完成后再提交。缺少真实 provider 时仍返回
`ErrSnapshotProviderMissing`，不返回固定占位字节。

原 C# `LSWorld`/`LSUnit` 的职责和 `TSVector2 * 6 * 50 / 1000` 移动规则已在
Go 内部模型中落地；但 Go JSON snapshot 不是原 C# MemoryPack/TrueSync
fixed-point wire。TrueSync FP 的完整编码、客户端恢复、replay/hash-mismatch
协议和版本迁移仍需协议级验收，不能把 `RoomStateSnapshot v2` 宣称为跨语言
客户端战斗快照。

客户端 Room 消息现已按原 C# `Session → PlayerRoomComponent → RoomActorId`
语义接入 Gate。Go 协议新增完整三段式 `ActorId` 字段，同时保留旧数值字段
兼容解码；Match 成功通知建立玩家 Room 绑定，Gate 覆盖客户端 `PlayerId`
后直接向 Room Actor 发送 ChangeSceneFinish、FrameMessage 和 CheckHash。
缺少完整绑定时直接失败，不把帧消息错误转发到 Unit Location。
Room snapshot v2 保留连续帧和空帧，恢复先校验后提交；AOI 重新 Enter 其他 Cell
时清理旧 Cell，ECS 同类型组件替换时清理旧 Entity 引用。

## 4. 不能直接迁移的内容

以下内容需要单独设计：

1. C# async/ETTask 与 Go Fiber/Task queue；
2. C# MemoryPack 与 Go protobuf；
3. Hotfix DLL 装载与 Go 静态链接；
4. C# config/Luban 与 Go JSON；
5. 原 C# Token/密码数据迁移；
6. Snapshot/Replay 二进制格式；
7. 跨进程 Router/NetInner 的真实部署连接发现和兼容验收；
8. C# 运行时中的隐式服务注册。

## 5. 迁移规则

历史代码只提供“应该发生什么”的证据。实现时必须另外确认：

```text
Go 类型是否存在
  → FiberInit 是否注册
  → 启动配置是否创建
  → 依赖是否显式注入
  → 协议是否 wire 兼容
  → 自动化和真实运行是否验证
```

如果只能确认历史意图，不能确认 Go 运行语义，必须写 TODO，而不是添加 fallback。
