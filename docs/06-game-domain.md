# 游戏领域能力

## 1. 领域状态的中心：Unit

`module/unit.Unit` 是 Map 内玩家、Monster、NPC 的实体载体。一个玩家 Unit 通常包含：

```text
Unit
 ├── MoveComponent
 ├── NumericComponent
 ├── AOIEntity
 ├── actor.MailBox
 ├── BagComponent（按业务注入）
 ├── WarehouseComponent（按业务注入）
 └── PathfindingComponent（必须配置 Finder 才能寻路）
```

`UnitComponent` 只负责 Unit registry：

```go
unitComponent.Get(id)
unitComponent.Add(u)
unitComponent.Remove(id)
```

创建和 Map 接入是两个步骤：

```text
CreatePlayer
  → 创建 Unit 基础组件
InitializeMapUnit
  → AOI.Enter
  → Location.Add(Unit)
```

这样可以避免工厂在缺少 Map 依赖时悄悄创建半可用玩家。

`UnitComponent.Add` 会拒绝 nil、无效 ID、重复 ID 和已关闭 registry；
Unit 工厂或 Map transfer 在注册失败时销毁刚创建的 Unit，不保留半接入
对象。

## 2. AOI

### 2.1 网格模型

`AOIManagerComponent` 使用网格 Cell：

```text
AOIEntity
  ├── Pos
  ├── ViewDistance
  ├── Cell
  ├── SeeUnits
  └── BeSeePlayers
```

AOI 管理器负责：

- Enter；
- Leave；
- Move；
- 计算进入/离开视野；
- 发布 `EventUnitEnterSightRange` 和 `EventUnitLeaveSightRange`。

### 2.2 状态同步

StateSync 订阅 AOI 事件：

```text
AOI enter
  → 找到观察者 Player
  → MessageLocationSender
  → CreateUnits

AOI leave
  → MessageLocationSender
  → RemoveUnits
```

广播发送失败会记录错误。不可见对象不会通过 AOI 正常路径广播。

AOI 管理器销毁时会取消位置订阅，并清理所有 Cell 中实体的 `Cell`、
`SeeUnits` 和 `BeSeePlayers` 引用；销毁后的管理器不会通过重复 `Awake`
重新订阅或创建内部状态。

`Enter` 具有幂等的重新接入语义：实体已在目标 Cell 时只更新位置；实体已
登记在其他 Cell 时先执行 Move，移除旧 Cell 关系后再进入新 Cell。不能仅把
同一个 Entity 追加到新 Cell，否则旧 Cell 会保留脏引用并重复广播。

## 3. Numeric

`NumericComponent` 保存整数和浮点数值，并提供：

- `Set/Get`；
- `SetFloat/GetAsFloat`；
- 数值变化；
- `NumericType`；
- 持久化和迁移。

玩家初始化时会写入：

- Speed；
- AOI view distance。

Numeric 是 Unit 组件，不负责网络广播；状态同步由上层事件/业务 handler 决定。

## 4. Move

### 4.1 MoveComponent

`MoveComponent` 保存：

- 当前路径 Targets；
- 当前路径索引 N；
- 起始位置和目标位置；
- 速度；
- 旋转插值；
- 完成/取消 channel。

移动开始时必须满足：

- 至少两个路径点；
- 速度大于 0；
- Unit accessor 存在。

Update 根据当前时间做位置和旋转插值。完成返回 nil，取消返回 `ErrMoveCanceled`。
组件销毁会主动取消当前移动并释放 `MoveToAsync` 的等待 channel，不能让
调用方永久阻塞。

### 4.2 PathfindingComponent

Pathfinding 接口：

```go
type Finder interface {
    FindPath(start, target, extents Float3, maxPolys int) ([]Float3, error)
}
```

`PathfindingComponent.Find`：

1. 转换坐标系；
2. 要求 Finder；
3. 调用 Finder；
4. 要求至少两个路径点；
5. 转回业务坐标系。

如果 Map 没有配置导航网格，缺少 Finder 时返回 `ErrFinderMissing`；
路径不足返回 `ErrPathfindingFailed`。不会回退成 `[start,target]`。

代码 TODO：

```text
TODO(map): 固定与原 DotRecast 兼容的 Go navmesh 实现、资源格式、坐标系和
`move.FinderFactory` 的生产注册。原 C# 已确认使用
`DtMeshSetReader.Read32Bit(br, 6)` 读取 Recast mesh，坐标转换为
`(-x, y, z)`，查询 extents 为 `(15, 10, 15)`、最大路径多边形数为 `256`，
并依次执行 nearest-poly、path、closest-point 和 straight-path 查询；Go
尚未有等价 reader/query 实现。配置了 `NavMeshFile` 但工厂缺失时，Map
启动直接失败；未配置导航网格时，Map 可启动但寻路请求必须失败。
```

## 5. StateSync

Unit 有序 MailBox 处理：

- `C2M_EnterMap`；
- `C2M_PathfindingResult`；
- `C2M_Stop`；
- `G2M_SessionDisconnect`。

寻路流程：

```text
PathfindingResult
  → 校验 Numeric.Speed
  → 校验 PathfindingComponent/Finder
  → Find
  → MoveComponent.StartMove
  → BroadcastIncludeSelf(PathfindingResult)
  → 完成/失败发送 Stop
```

依赖类型错误、速度无效、寻路失败、移动启动失败都会发送明确 Stop 错误码。
`Broadcast` 和 `BroadcastIncludeSelf` 返回发送错误；EnterMap 的初始
CreateMyUnit/可见单位通知失败会使响应返回错误，寻路过程中的广播失败会
停止当前移动并记录错误，不再把通知失败当作成功。

## 6. Map 生命周期

### 6.1 EnterMap

```text
Gate 登录
  → Central 得到 PlayerID
  → Location.Get(Unit, PlayerID)
  → 没有 owner
  → Call Home Map/G2M_EnterMap
  → CreatePlayer
  → InitializeMapUnit
```

初始化失败会清理 Unit registry 和 Entity。

### 6.2 Transfer

Map transfer 使用两阶段提交，关键保证是：

```text
RPC 失败 ≠ 源 Unit 已销毁
```

源端锁 Unit Location。目标端只有在 Entity、AOI、通知和 Location ownership 全部成功后返回成功。未知迁移组件是错误，不会跳过。

客户端 `C2M_TransferMap` 必须先由 Gate 根据认证 Session 的 UnitID 路由到
当前 Unit MailBox。Map handler 使用完整 `ActorID`（ProcessID、FiberID、
InstanceID）解析 Unit；不匹配时直接返回 `ErrTransferUnitMissing`，不再
从 Map 的第一个 Unit 或 PlayerID 推断目标。地图间 RPC 的错误响应保留
原始 RpcID，旧 owner ActorID 无效时拒绝执行。

可迁移组件：

- Numeric；
- Bag；
- Warehouse；
- 通过 `RegisterTransferComponent` 注册的其他组件。

Entity 的核心运行时组件（Move、AOI、MailBox）由目标 Map 重新创建，不从 payload 直接复用旧 Scene 引用。

## 7. Inventory

### 7.1 Bag

Bag 维护：

- MaxCapacity；
- Slots；
- items；
- ItemSlotMap；
- ConfigIdItemsMap；
- BSON snapshot。

支持：

- 添加；
- 删除；
- 按配置扣减；
- SwapSlots；
- SortBag；
- 迁移和持久化。

物品配置必须先通过 `RegisterItemConfigType` 注册；未知配置返回
`ERR_ItemConfigNotFound`，未知类型不会被当作容量为 1 的合法物品。

BSON/Transfer snapshot 会严格校验容量、数量、正数配置和唯一 ID、合法且
不重复的 SlotIndex、已注册物品配置以及最大堆叠数；恢复时以 Items
重建 Slots 和运行时索引，保留持久化 SlotIndex，不信任过期的派生 map。

### 7.2 Warehouse

Warehouse 嵌入 BagComponent，但业务错误码独立：

- 仓库满；
- 仓库物品不存在；
- 仓库操作无效；
- 数量无效。

仓库操作必须同时具备 BagComponent 和 WarehouseComponent，类型错误不会 panic。
Bag 与 Warehouse 的跨容器操作先保存双方 snapshot，第二阶段失败时回滚
目标容器，避免只完成一半的转移。

### 7.3 通知

Item 变更通过 GateSession Location sender 推送。通知发送错误返回 `ERR_InventoryNotifyFailed` 并记录 Message，不再使用 `_ = sender.Send`。

数据已经变更后通知失败无法自动回滚，因此生产需要补充可靠消息/重试策略；当前这部分属于运营可靠性 TODO。

## 8. Lockstep

### 8.1 Match/Room

MatchComponent 维护匹配中的玩家 ID。RoomManager 负责：

- 房间元数据；
- 动态 Room Fiber；
- Map ActorID；
- Room ActorID；
- 玩家列表。

Match 完成后必须向每个玩家的 GateSession 发送
`Match2G_NotifyMatchSuccess`；Gate sender 缺失或发送失败会使 handler 返回
错误，不静默继续。

动态 Room 创建必须注入 Fiber Manager，并且 Room Fiber 必须有 RoomServerComponent。
`RoomServerComponent` 的 `LateUpdate` 已注册到 Room Fiber；全员离线时会通知
Map 并回收所属 Fiber，不能只删除房间元数据。

### 8.2 帧推进

`LSServerUpdater` 以 `UpdateIntervalMillis` 推进：

```text
当前时间 - StartTime
  → 计算 target frame
  → 补齐输入
  → 写入 FrameBuffer
  → Replay.AddFrameInput
  → 广播 OneFrameInputs
```

缺少玩家输入时，使用上一帧输入；如果上一帧也没有，使用空输入。这是锁步协议定义的输入补齐策略，不是依赖缺失兜底。

### 8.3 快照

快照接口：

```go
type SnapshotProvider func(room *LockstepRoom) ([]byte, error)
```

默认 Room Fiber 已注入 `MarshalRoomSnapshot`：

```text
version + Room state + player state + frame inputs
       + LSWorld(SceneType/Frame/IDGenerator/units) + SHA-256
```

Room 启动时先在第 `0` 个权威帧调用一次 SnapshotProvider；只有快照成功后才
广播 `Room2C_Start`。之后 `LSServerUpdater` 每 `SaveLSWorldFrameCount` 帧
（当前为 1200 帧）调用 SnapshotProvider，并将非空快照同时写入
`FrameBuffer` 和 `Replay`。Reconnect
读取最近快照及其后的 Replay 输入；Hash mismatch handler 会读取不晚于目标
帧的最近快照并构造 `Room2C_CheckHashFail`。这表示服务端已经具备检测、
选择恢复起点和构造响应的路径，不表示客户端状态恢复、Replay 重放和完整
hash mismatch recovery 已经完成或验收。快照生成失败只记录
`LastSnapshotError`，不会写入占位字节。

`VerifyRoomSnapshot` 会校验版本和 checksum，`RestoreRoomSnapshot` 会在校验
通过后恢复 Go Room/FrameBuffer/RoomPlayer/LSWorld 的状态。LSWorld 包含按
PlayerID 排序的确定性单位、Position、Rotation、上一帧 Input、World Frame
和 ID watermark；输入使用原 TrueSync Q31.32 raw integer 解码，移动公式保持
`TSVector2 * 6 * 50 / 1000`。Room authority frame、Updater frame 和
LSWorld frame 不一致时不会推进。没有 Provider 时：

- 不生成固定 `[]byte("snapshot")`；
- `LastSnapshotError()` 返回 `ErrSnapshotProviderMissing`；
- Reconnect/Hash mismatch 在协议 handler 层返回 `ErrSnapshotMissing`。

服务端恢复锚点允许为第 `0` 帧，但外部客户端输入和状态推进仍从第 `1` 帧
开始。后续快照中的帧必须从 1 开始连续递增；即使某一帧没有玩家输入，也会
保留空帧，不能在写入 Replay 时压缩掉该索引，否则重放帧号会错位。CheckHash
通过 `FrameBuffer.CheckAndSetHash` 原子记录首个哈希，适配无序 Room mailbox
下的并发请求。恢复玩家列表和
玩家状态采用先校验、后一次性提交的方式；目标 Room Server 已关闭或恢复
数据非法时，不会先修改部分 Room 状态再返回错误。

当前序列化和恢复实现覆盖 Go `LockstepRoom/FrameBuffer/RoomPlayer/LSWorld`
状态，且恢复在世界和玩家校验完成后再提交。`TODO(lockstep)` 不再是 Go
LSWorld 模型缺失，而是原 C# MemoryPack/TrueSync fixed-point wire、客户端
恢复/replay/hash-mismatch 协议和跨版本迁移规则尚未证明；因此不能把 Go JSON
snapshot 宣称为客户端可直接消费的原始战斗世界快照。

此外，客户端 Room 消息已经完成 Gate → Room 的严格路由绑定。匹配成功
通知写入玩家的 `PlayerRoomComponent`，其中保存完整 `MapActorID` 和
`RoomActorID`；Gate 只使用该绑定发送 `FrameMessage`、ChangeSceneFinish、
CheckHash，并覆盖客户端提交的 `PlayerID`。缺少绑定或 ActorID 不完整时
直接失败，不把消息转发到 Unit。

## 9. Crontab

CrontabComponent 支持五段 cron：

```text
minute hour day month weekday
```

任务执行前检查：

- task 名称；
- 表达式；
- handler 是否注册；
- 是否已有同名任务；
- 是否正在运行。

任务 panic 会被捕获并写日志。当前没有默认业务任务；组件存在不代表业务已经启用。

## 10. 领域层共性约束

1. Unit 状态只由 Map Fiber 所有者修改。
2. AOI 只在 Unit 已进入 Map 后使用。
3. Location 是 actor ownership，不是简单缓存。
4. Inventory 操作必须先检查容器和数量。
5. Pathfinding 缺少 Finder 时失败，不能直线兜底。
6. Lockstep snapshot 缺少 serializer 时失败，不能伪造。
7. 异步广播/保存必须处理错误。
