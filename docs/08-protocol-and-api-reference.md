# 协议与 API 参考

本文按 Go 代码的协议边界组织：

```text
proto/*.proto
  → proto/*pb/*.pb.go
  → module/*/proto_codec.go
  → module message struct
  → MailBox / Gate / Session / HTTP
```

业务结构和 wire 结构分离。修改协议时必须同步 `.proto`、生成代码、业务
message、编解码器、handler、测试和客户端兼容矩阵。

## 1. Packet

统一 Packet 头部为：

```text
Type       1 byte
MsgID      2 bytes，big endian
RpcID      4 bytes，big endian
PayloadLen 4 bytes，big endian
Payload    N bytes
```

`RpcID` 由请求方分配，响应必须原样携带。`PacketTypeResponse` 通过
`MsgID + RpcID` 与等待中的 RPC 关联。

跨进程 Actor `Message/Request` 的 Payload 前 20 字节是 big-endian Actor
envelope：

```text
ProcessID  4 bytes
FiberID    8 bytes
InstanceID 8 bytes
Payload    N bytes
```

跨进程 `Response` 的 Payload 是业务响应本身，不重复包 Actor envelope。

### 1.1 NetInner peer handshake

`engine/network/peer` 使用保留的内部 MsgID `65500` 完成 Session 认证。握手
JSON 字段为：

| 字段 | 含义 |
|---|---|
| `version` | peer 协议版本，当前为 `1` |
| `sender_process_id` | 发起握手的逻辑 ProcessID |
| `target_process_id` | 目标逻辑 ProcessID |
| `nonce` | 发起方随机 nonce |
| `mac` | HMAC-SHA256(`version|sender|target|nonce`, shared secret) |

较小 ProcessID 主动拨号，较大 ProcessID 接受连接。双方必须在
`StartProcessConfig.Peers` 中声明对端和相同 secret；认证失败、版本不匹配、
目标不匹配或 Session 关闭都会阻止 Actor 消息继续进入业务 Fiber。
handshake 的 `Request` 必须携带非零 `RpcID`；缺失或为零时接收端直接拒绝。
握手通过后才调用 `ProcessOuterSender.AddSession`。

### 1.2 Router UDP/TCP 控制帧

Router UDP/TCP 使用原 ET/C# 的 little-endian 控制帧，不能套用统一 Packet
头或 20 字节 Actor envelope：

| 帧 | wire 结构 | 方向 |
|---|---|---|
| `RouterSYN` | `1 + OuterConn(4) + InnerConn(4) + ConnectID(4) + InnerAddress` | 外部 → Router |
| `RouterACK` | `1 + InnerConn(4) + OuterConn(4)` | Router → 外部 |
| 普通 `SYN` | `1 + ConnA(4) + ConnB(4) + RealAddress` | 外部 → Router → 内部 |
| 普通 `ACK` | `1 + InnerConn(4) + OuterConn(4)` | 内部 → Router → 外部 |
| `MSG` | `1 + ConnA(4) + ConnB(4) + Payload` | 双向转发 |
| `FIN` | `1 + ConnA(4) + ConnB(4) + Error(4)` | 双向转发 |
| `RouterReconnectSYN` | 外部输入带 `ConnectID`；转发到内部时为 9 字节连接字段 | 外部 → Router → 内部 |
| `RouterReconnectACK` | `1 + InnerConn(4) + OuterConn(4)` | 内部 → Router → 外部 |

外部到内部时连接字段按 `OuterConn, InnerConn` 写入；内部到外部时按
`InnerConn, OuterConn` 写入。RouterSYN 只创建/更新 RouterNode 并返回
RouterACK，不再次发送内部 RouterSYN。Go `module/router/transport.go`
同时支持生产外/内双 UDP socket，以及测试用的共享 socket 方向识别。
`module/router/tcp_transport.go` 在外部 TCP 上使用 little-endian `uint16`
body length 前缀包裹同一 Router frame，并复用 RouterSYN 建立的已接受连接；
WebSocket 不属于当前已实现 transport。

## 2. protobuf 文件

| 文件 | 边界 |
|---|---|
| `proto/login_outer.proto` | 客户端 ↔ Realm/Gate |
| `proto/login_inner.proto` | Realm ↔ Gate、Gate ↔ Map |
| `proto/central_inner.proto` | Realm/Gate ↔ Central |
| `proto/launch_inner.proto` | Gate ↔ Game |
| `proto/map_inner.proto` | Gate ↔ Map、Map ↔ Map |
| `proto/map_outer.proto` | Map ↔ 客户端 |
| `proto/actorlocation_inner.proto` | 业务 ↔ Location |
| `proto/inventory_outer.proto` | Inventory ↔ 客户端 |
| `proto/lockstep_inner.proto` | Match/Map/Room |
| `proto/lockstep_outer.proto` | Lockstep ↔ 客户端 |

## 3. 消息编号

### 3.1 登录与 Gate

| MsgID | 请求/消息 | 响应/消息 | 方向 |
|---:|---|---|---|
| `2001` | `C2R_Login` | — | Client → Realm |
| `2002` | — | `R2C_Login` | Realm → Client |
| `2103` | `C2G_Ping` | — | Client → Gate |
| `2104` | — | `G2C_Ping` | Gate → Client |
| `2105` | `C2G_LoginGate` | — | Client → Gate |
| `2106` | — | `G2C_LoginGate` | Gate → Client |
| `22001` | `R2G_GateAssign` | — | Realm → Gate |
| `22002` | — | `G2R_GateAssign` | Gate → Realm |
| `22003` | `G2M_SessionDisconnect` | — | Gate → Map |

客户端方向的响应使用独立 MsgID。Gate 通过 `RegisterLocationRequestWithResponse`
维护请求编号到客户端响应编号的映射；没有注册映射时才沿用请求编号。

### 3.2 Location

| MsgID | 名称 | 方向 |
|---:|---|---|
| `20001` | `ObjectAddRequest` | 业务 → Location |
| `20002` | `ObjectAddResponse` | Location → 业务 |
| `20003` | `ObjectLockRequest` | 业务 → Location |
| `20004` | `ObjectLockResponse` | Location → 业务 |
| `20005` | `ObjectUnlockRequest` | 业务 → Location |
| `20006` | `ObjectUnlockResponse` | Location → 业务 |
| `20007` | `ObjectRemoveRequest` | 业务 → Location |
| `20008` | `ObjectRemoveResponse` | Location → 业务 |
| `20009` | `ObjectGetRequest` | 业务 → Location |
| `20010` | `ObjectGetResponse` | Location → 业务 |

### 3.3 Central、Game Login 和 Map Inner

| MsgID | 名称 | 方向 |
|---:|---|---|
| `21501` | `G2Game_Login` | Gate → Central / Game-login handler |
| `21502` | `Game2G_Login` | Central / Game-login handler → Gate |
| `22501` | `G2M_EnterMap` | Gate → Map |
| `22502` | `M2G_EnterMap` | Map → Gate |
| `50001` | `R2Central_AccountLogin` | Realm → Central |
| `50002` | `Central2R_AccountLogin` | Central → Realm |
| `23003` | `M2M_UnitTransferRequest` | Map → Map |
| `23004` | `M2M_UnitTransferResponse` | Map → Map |

内部 RPC 的响应通常由 Actor/Fiber RPC 携带原请求 `MsgID/RpcID`；表中
响应编号用于协议源和业务消息类型，不表示 Gate 必然把内部响应编号直接
暴露给客户端。

这里的 `Game` 是历史协议命名，不是当前 Go 的 `SceneType`；当前实现由
`module/central` 的 Central Scene 承担对应 handler。

### 3.4 Map Outer 与 StateSync

`MapOuter_C_2500`：

| MsgID | 名称 | 方向 |
|---:|---|---|
| `2501` | `C2M_EnterMap` | Client → Map |
| `2502` | `M2C_EnterMap` | Map → Client |

`StateSyncOuter_C_3000`：

| MsgID | 名称 | 方向/用途 |
|---:|---|---|
| `3004` | `UnitInfo` | 单位数据结构编号 |
| `3005` | `M2C_CreateUnits` | Map → Client |
| `3006` | `M2C_CreateMyUnit` | Map → Client |
| `3007` | `M2C_StartSceneChange` | Map → Client |
| `3008` | `M2C_RemoveUnits` | Map → Client |
| `3009` | `C2M_PathfindingResult` | Client → Map |
| `3010` | `C2M_Stop` | Client → Map |
| `3011` | `M2C_PathfindingResult` | Map → Client |
| `3012` | `M2C_Stop` | Map → Client |
| `3021` | `C2M_TransferMap` | Client → Map |
| `3022` | `M2C_TransferMap` | Map → Client |

Go 中 `C2M_EnterMap` 的常量位于 `module/statesync`，编号是 `2501`；
不能把它和 StateSync 的 `UnitInfo=3004` 混写。

Map transfer 的异步失败通知必须使用响应方向的 `3022 M2C_TransferMap`。
请求 `3021 C2M_TransferMap` 只表示客户端发起操作，不能被复用为服务端
失败通知的 MsgID。

### 3.5 Inventory

| MsgID | 名称 | 方向 |
|---:|---|---|
| `4001` | `ItemInfo` | 数据结构编号 |
| `4002` | `C2M_GetBagInfo` | Client → Map |
| `4003` | `M2C_GetBagInfo` | Map → Client |
| `4004` | `C2M_BagOperation` | Client → Map |
| `4005` | `M2C_BagOperation` | Map → Client |
| `4006` | `C2M_GetWarehouseInfo` | Client → Map |
| `4007` | `M2C_GetWarehouseInfo` | Map → Client |
| `4008` | `C2M_WarehouseOperation` | Client → Map |
| `4009` | `M2C_WarehouseOperation` | Map → Client |
| `4010` | `M2C_ItemChange` | Map → Client |

### 3.6 Lockstep

外部消息：

| MsgID | 名称 |
|---:|---|
| `3501` | `C2G_Match` |
| `3502` | `G2C_Match` |
| `3503` | `G2C_NotifyMatchSuccess` |
| `3504` | `C2Room_ChangeSceneFinish` |
| `3506` | `Room2C_Start` |
| `3507` | `FrameMessage` |
| `3508` | `OneFrameInputs` |
| `3509` | `Room2C_AdjustUpdateTime` |
| `3510` | `C2Room_CheckHash` |
| `3511` | `Room2C_CheckHashFail` |

内部消息：

| MsgID | 名称 |
|---:|---|
| `23501` | `G2Match_Match` |
| `23502` | `Match2G_Match` |
| `23503` | `Match2Map_GetRoom` |
| `23504` | `Map2Match_GetRoom` |
| `23505` | `G2Room_Reconnect` |
| `23506` | `Room2G_Reconnect` |
| `23507` | `RoomManager2Room_Init` |
| `23508` | `Room2RoomManager_Init` |
| `23509` | `Room2M_NotifyRoomDispose` |
| `23510` | `Match2G_NotifyMatchSuccess` |
| `23511` | `G2M_RoomExists` |
| `23512` | `M2G_RoomExists` |
| `23513` | `M2Room_PlayerOffline` |
| `23514` | `Match2Map_CancelRoom` |
| `23515` | `Map2Match_CancelRoom` |
| `23516` | `Match2G_CancelMatchSuccess` |

Go 的 `MsgRoom2CReconnect` 是当前 handler 使用的外部语义别名，底层
协议源没有独立的 `Room2C_Reconnect` 编号；需要客户端确认后才能新增
独立编号。

### 3.7 Lockstep 外部 Room 路由边界

当前 Gate 已注册 `C2G_Match` 的本地 handler，并可把明确声明为
Location request 的消息路由到认证 Session 绑定的 Unit MailBox。
`C2M_TransferMap` 属于这一类：目标 MailBox 的完整 ActorID 会继续传给
Map handler，Map 会再次校验完整地址，不接受仅凭 PlayerID 或 InstanceID
猜测的 Unit。

`Match2G_NotifyMatchSuccess` 和 `G2C_NotifyMatchSuccess` 的兼容数值字段仍
保留，但新增 `ActorId { ProcessId, FiberId, InstanceId }` 字段。Gate 收到
匹配成功通知后，将完整 `RoomActor`/`MapActor` 写入玩家的
`PlayerRoomComponent`，并向客户端发送外部通知。

`C2Room_ChangeSceneFinish`、`FrameMessage`、`C2Room_CheckHash` 由 Gate
读取认证 Session → Player → PlayerRoom 的绑定，使用完整 `RoomActorID`
调用 `MessageSender.Send`。Gate 会覆盖消息体中的 `PlayerId`，因此客户端
不能伪造其他玩家；没有 Room 绑定、ActorID 不完整、MessageSender 缺失或
发送失败都会报错，不改走 Unit Location。Room Fiber 只处理已路由的消息，
并通过 GateSession Location 广播 Start、调整时间和 Hash mismatch。

## 4. HTTP API

默认 HTTP Scene：

| Path | Method | 行为 |
|---|---|---|
| `/login` | POST | 查询账号并生成 AccessToken |
| `/register` | POST | 创建账号 |
| `/get_area_list` | GET | 返回 Area 列表 |

RouterManager HTTP：

| Path | Method | 依赖 |
|---|---|---|
| `/login` | POST | MessageSender、Central Actor |
| `/router/list` | GET | 完整 Machine/Process/Scene 配置 |
| `/zone/list` | GET | Zone 配置 |
| `/zone/last` | GET | AccessToken、Logic Zone |

HTTP Scene、RouterManager、RouterNode 不是同一个运行对象：

| 运行对象 | 传输 | 主要职责 | 是否承载默认 `/login` |
|---|---|---|---|
| `HTTP` Scene | HTTP | 账号注册、登录、Area 列表 | 是 |
| `Router` Scene | HTTP | Router/Zone 管理接口和 Router 登录转发 | 否，使用独立 handler |
| `RouterNode` Scene | UDP/TCP | Router 中继和连接状态 | 否 |

HTTP 的 `/login`、`/register` 在请求对象、响应写入器、请求体或全局配置
缺失时返回明确 Go error；用户名或密码为空时返回 `400` 业务响应。空
`Areas` 只有在全局配置存在时才是合法的“当前没有 Area”结果。

统一响应字段：

```json
{
  "RequestId": "",
  "Error": 0,
  "Message": "",
  "AccessToken": ""
}
```

请求体字段使用 Go handler 当前兼容格式：

```json
{
  "Username": "user",
  "Password": "pass"
}
```

## 5. 错误码

### 5.1 登录

| 错误 | 含义 |
|---:|---|
| `100009001` | Gate Token 无效 |
| `100009002` | 用户名或密码错误 |
| `100009003` | AccessToken 无效 |
| `100009004` | AccessToken 过期 |

### 5.2 Location

| 错误 | 含义 |
|---:|---|
| `1001` | Location 当前被锁定 |
| `1002` | Location 记录不存在 |

未注册 key 的基础 `Get` 可以返回零 ActorID，表示当前没有 owner；
真正发送消息时，`MessageLocationSender` 会把无效 ActorID 转为
`ErrLocationNotFound`。

### 5.3 Inventory

原协议错误码：

| 错误 | 含义 |
|---:|---|
| `100030001` | 背包满 |
| `100030002` | 背包物品不存在 |
| `100030003` | 背包物品数量不足 |
| `100030004` | 背包格子无效 |
| `100030005` | 背包操作无效 |
| `100030006` | Session/Player 错误 |
| `100030051` | 仓库满 |
| `100030052` | 仓库物品不存在 |
| `100030053` | 仓库物品数量不足 |
| `100030054` | 仓库格子无效 |
| `100030055` | 仓库操作无效 |
| `100030101` | 物品配置不存在 |
| `100030102` | 物品不可堆叠 |
| `100030103` | 物品堆叠溢出 |

Go 严格化扩展错误码使用 `100030201` 起的保留段：

| 错误 | 含义 |
|---:|---|
| `100030201` | 请求无效 |
| `100030202` | Unit 不存在 |
| `100030203` | 容器组件缺失或类型错误 |
| `100030204` | 物品通知发送失败 |
| `100030205` | 背包数量无效 |
| `100030206` | 仓库数量无效 |

这些扩展码必须加入客户端版本矩阵，不能假设旧客户端已经理解。

### 5.4 Map/Move/Lockstep

Go error 主要在跨 Actor/服务边界传播：

- `move.ErrFinderMissing`、`move.ErrFinderFactoryMissing`；
- `move.ErrPathfindingFailed`；
- `map_.ErrTransferComponentUnsupported`；
- `map_.ErrTransferUnitAlreadyExists`；
- `map_.ErrTransferUnitInvalid`；
- `map_.ErrTransferRecoveryCoordinatorMissing`、
  `map_.ErrTransferRecoveryTokenMissing`、
  `map_.ErrTransferLedgerRecoveryCoordinatorMissing`；
- `login.ErrGateTokenGeneration`；
- `lockstep.ErrSnapshotProviderMissing`；
- `lockstep.ErrSnapshotMissing`；
- `lockstep.ErrSnapshotServerMissing`；
- `lockstep.ErrLSWorldMissing`、`lockstep.ErrLSWorldPlayerMissing`、
  `lockstep.ErrLockstepStateOutOfSync`；
- `actor.ErrMailboxFull`；
- `actor.ErrProcessSessionClosed`；
- `network.ErrSendChannelFull`。

Match 成功通知缺少 sender 或发送失败时，`G2C_Match` 返回业务错误；
服务端不会仅因为 Room 已创建就返回成功。

这些错误不能被转换为“成功但没有数据”。

## 6. Token 与安全

当前 AccessToken 的生产格式是带 key-id 的签名 Token：

```text
v2.<key-id>.<payload>.<signature>
  → payload = Base64URL(JSON(account_id, issued_at, expires_at, nonce))
  → signature = HMAC-SHA256(v2.<key-id>.<payload>, key-ring[key-id])
```

Gate Token 是短期 registry key。Realm 生成并交给客户端，Gate 登录成功
前读取并删除。

Gate Token 的 wire format 与原 C# 实现保持一致：随机生成
`D5.accountID.D4`，再做 UTF-8 Hex 编码。Go 使用 `crypto/rand` 生成两段随机
数；随机源失败会返回 `login.ErrGateTokenGeneration`，不会把空字符串注册为
有效 Token。

生成使用当前 key；验证可以使用 key ring 中仍保留的旧 key，从而支持轮换。
配置了 `allowLegacyTokens=true` 和 `legacyTokenKey` 时，验证器才接受旧 XOR
Token；只有显式设置 `accessTokenFormat="legacy"` 时才生成旧 SimpleToken，
这只用于迁移窗口，不是默认生成格式。
未安装安全配置时，生成和验证直接返回 `login.ErrTokenConfigRequired`，不会
隐式使用历史固定 key。

代码仍保留 `TODO(security)`：

- MongoDB 共享 Token 撤销的真实连接、故障恢复和多进程验收；
- TLS 密钥托管、证书文件原子替换约束和生产验收；
- 跨 Process 登录限流、审计保留策略和真实审计集合验收。

密码字段已经增加 `password_algorithm`：新账号使用 Argon2id，旧 MD5 账号
首次成功登录时升级。HTTP 已支持显式 TLS 强制、MongoDB 原子 bucket 登录
限流，AccessToken 已支持撤销存储接口；剩余安全 TODO 是真实 DB 故障策略、
密钥托管和审计协议。HTTP 新 TLS 握手的证书文件热加载以及 MongoDB 撤销/
限流 wiring 已经实现，但不替代真实 MongoDB、部署层原子文件替换和密钥托管
验收。

## 7. Map target、navmesh、snapshot

Map 配置支持：

```json
{
  "mapTargets": ["Map2"],
  "navMeshFile": "maps/map1/navmesh.bin"
}
```

`mapTargets` 当前最多一个，因为 `C2M_TransferMap` 不携带目标名；
目标必须是同一 Process 中已经创建的 Map。跨进程 peer transport 已完成，
但 Map target 的远端注册、选择和失效恢复协议尚未定义，因此配置校验仍会
直接拒绝跨进程 target。

如果配置了 `navMeshFile`，Map Fiber 启动时必须已经注册
`move.RegisterFinderFactory`，否则启动返回 `ErrFinderFactoryMissing`；
工厂加载资源失败也会阻止启动。若没有配置 `navMeshFile`，Map 可以启动，
但寻路请求返回 `ErrFinderMissing`。Go 核心不会生成直线寻路。

默认 Room Fiber 已注入 Go `RoomStateSnapshot v2` provider，
快照包含版本、Room 状态、玩家状态、连续帧输入、`LSWorld` 单位状态和
SHA-256。Room 启动时先生成第 `0` 帧服务端恢复锚点，成功后才发送
`Room2C_Start`；外部客户端帧仍从 `1` 开始。没有输入的帧也保留为空帧，
Replay 不压缩帧号。Restore 先校验
全部玩家、帧数据、世界帧和单位身份，再原子提交
`Room/FrameBuffer/Replay/LSWorld` 状态；关闭的 Room Server 不会发生部分恢复。
当前格式是 Go 内部 JSON 状态，不是原客户端 MemoryPack/TrueSync wire；
TrueSync fixed-point 编码、客户端恢复和 hash-mismatch replay 协议仍未完成。

## 8. 协议兼容规则

1. 不复用已有字段编号；
2. 不改变字段类型；
3. 增加字段时保持旧客户端可解析；
4. 修改 Error/Message 语义时更新客户端矩阵；
5. 任何新消息都要有 encode/decode 测试；
6. 跨进程 envelope 和 Packet RpcID 不能丢失；
7. KCP 控制帧和标准 KCP segment 必须保持原 ET 字节序；
8. Snapshot、Map transfer 等状态恢复协议必须有真实数据模型，不允许占位 payload。
