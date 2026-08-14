# 消息与网络

## 1. 消息的四层边界

```text
业务结构
  → protobuf wire 结构
  → codec.Packet
  → TCP/KCP/Router/Session
```

业务 handler 不应该自行拼 Packet header；网络层不应该理解 Unit、登录或 Inventory 字段。

典型处理链：

```text
客户端/进程包
  → codec.Decode
  → MsgID/RpcID 路由
  → unmarshal
  → handler
  → marshal
  → codec.Encode
  → Session.Send
```

## 2. Actor 地址

```go
type ActorID struct {
    ProcessID  int
    FiberID    int64
    InstanceID int64
}
```

| 字段 | 含义 |
|---|---|
| `ProcessID` | 启动拓扑中的逻辑 Process ID，不是 OS PID |
| `FiberID` | 目标执行上下文 |
| `InstanceID` | 目标 Entity 的运行时实例 ID |

服务根 Scene、Player、Unit、Session 都可能是 Actor。`StartSceneConfig.ID` 是配置身份，通常不是 Actor envelope 中的 `InstanceID`。

实例化时应按以下规则区分 ID：

| 名称 | 来源 | 用途 | 是否进入 Actor envelope |
|---|---|---|---|
| 配置 Scene ID | `StartSceneConfig.ID` | 配置查找、拓扑身份 | 否，不能替代 `InstanceID` |
| Fiber ID | Fiber 运行时 `Fiber.ID()` | 选择执行上下文 | 是，写入 `ActorID.FiberID` |
| Entity InstanceID | Entity 创建时分配 | 定位运行时 Entity | 是，写入 `ActorID.InstanceID` |
| Root Scene ActorID | `ProcessID + FiberID + Scene.InstanceID()` | 定位 Scene 服务 | 是 |
| Unit ActorID | `ProcessID + Map FiberID + Unit.InstanceID()` | 定位地图单位 | 是 |

例如：

```text
配置 Scene ID       = 9003
逻辑 Process ID     = 1
Realm Fiber ID      = 12
Realm Root Instance = 1001

Realm Root ActorID  = (ProcessID=1, FiberID=12, InstanceID=1001)
```

`SceneRegistry` 保存的是运行时 Scene ActorID，不是配置 Scene ID。`GateID` 是登录协议中的业务/配置标识，也不能直接当作 `ActorID.InstanceID`。

## 3. MailBox 分发

MailBox 挂在 Entity 上：

```go
mailbox.RegisterHandler(msgID, func(
    target actor.ActorID,
    msgID uint16,
    payload []byte,
) ([]byte, error) {
    // decode → validate → execute → response
})
```

`DispatchFiberMessage` 的处理顺序：

```text
Fiber.Message.To
  → Scene.GetEntity(instanceID)
  → Entity.GetComponent("MailBox")
  → MailBox.Dispatch(msgID, payload)
```

邮箱类型：

| 类型 | 用途 |
|---|---|
| `MailBoxTypeUnOrderedMessage` | Scene 服务和普通 Actor |
| `MailBoxTypeOrderedMessage` | Unit 顺序业务消息 |
| `MailBoxTypeGateSession` | Gate Session 到客户端 |

未知消息应返回 `actor.ErrHandlerNotFound` 或服务层错误，不再静默成功。

## 4. MessageSender

`MessageSender` 依据 `ActorID.ProcessID` 选择路径：

```text
同进程
  → ProcessInnerSender
  → FiberManager.Get(FiberID)
  → Fiber.Send / Fiber.Call

跨进程
  → ProcessOuterSender
  → ProcessID 对应 PacketSession
  → Actor envelope
  → codec.Packet
```

### 4.1 同进程发送

单向消息是非阻塞入队：

```go
err := inner.Send(actorID, msgID, payload)
```

错误包括：

- Actor 无效；
- Fiber 不存在；
- Fiber 关闭；
- Fiber mailbox 满。

同进程 Call 必须经 `Fiber.Call`，不直接在新 goroutine 调用目标 Scene。

### 4.2 跨进程 envelope

`ProcessOuterSender` 将 ActorID 写入 Payload 前 20 字节：

```text
ProcessID  4 bytes big endian
FiberID    8 bytes big endian
InstanceID 8 bytes big endian
业务 Payload N bytes
```

完整 Packet：

```text
Packet header
  ├── Type       1 byte
  ├── MsgID      2 bytes
  ├── RpcID      4 bytes
  ├── PayloadLen 4 bytes
  └── Payload
        ├── Actor envelope（Message/Request）
        └── 业务数据
```

Response 不再包含 Actor envelope，直接由 RpcManager 依据 RpcID 解析。

### 4.3 跨进程接收 API

`ProcessOuterSender.HandlePacket` 支持：

- `PacketTypeMessage`：解包 ActorID 后向目标 Fiber 入队；
- `PacketTypeRequest`：通过目标 Fiber Call 执行并生成 Response；
- `PacketTypeResponse`：交给 RpcManager。

`HandleSessionPacket` 将接收 API 与 Session 连接起来。

`ProcessOuterSender.RemoveSession(processID)` 会使该 Process 上尚未返回的
RPC 立即以 `actor.ErrProcessSessionClosed` 结束。这个生命周期已经由
`engine/network/peer.ProcessPeerComponent` 接入：

```text
StartProcessConfig.Peers
  → NetInner KCP Listen
  → 较小 ProcessID 主动 Connect
  → peer handshake
  → ProcessOuterSender.AddSession
  → Session ReadLoop
  → HandleSessionPacket
  → Fiber.Send / Fiber.Call
```

NetInner Fiber 启动后会把 `ProcessOuterSender` 注册到 Actor 进程级 registry。
业务 Fiber 创建 `MessageSender` 时即使没有显式传入 outer sender，也会按当前
Process 动态解析该 registry；因此 Realm、Gate、Map、Match 等业务模块不会
各自维护不一致的跨进程 Session 表。

握手载荷是 JSON envelope，包含协议版本、发送方 ProcessID、目标 ProcessID、
随机 nonce 和 HMAC-SHA256。双方都必须在本地配置中声明对端和相同 secret；
缺失、版本不匹配、目标不匹配或 MAC 错误都会关闭 Session。Session 关闭时
调用 `RemoveSession`，并按固定间隔由主动拨号方重建连接。

peer 的 handshake `Request` 必须携带非零 `RpcID`；缺失或为零的相关请求会
在接收端拒绝，避免握手响应无法与等待中的 RPC 关联。

当当前 Process 有 `Peers` 但没有显式 `NetInner` Scene 时，`cmd/server`
自动创建 NetInner Fiber；显式 NetInner 则直接复用其 FiberInit。peer 组件
位于 `engine/network/peer`，避免基础 `engine/network` 反向依赖
`engine/actor` 形成 Go import cycle。

## 5. RPC

`RpcManager` 维护：

```text
rpcID → response channel + timeout timer
```

`ProcessOuterSender.Call` 使用 `RpcManager` 配置的 timeout，而不是忽略调用方
配置后继续使用固定时长。无论 Response 是业务响应、context cancel 还是
timeout，pending RPC 都必须从 sender 的 pending 表删除；否则长时间运行的
进程会积累已经不可达的回调。

成功路径：

```text
Call
  → RegisterWithTimeout
  → Send Request
  → 远端处理
  → Response.RpcID
  → Resolve
  → 返回 Payload
```

失败路径：

- Send 失败：移除 callback 并立即返回；
- context 取消：移除 callback；
- timeout：定时器返回 `ErrTimeout`；
- Fiber 停止：`Fiber.Call` 返回错误。

RPC 不应无限等待 DB、Location 或远端 Map。业务调用应给出明确 context；
Session 提供的生命周期 context 会传入 Gate、Realm 和 Net accepted-session
路径，连接关闭时可以取消尚未完成的业务调用。

## 6. Packet 编码

`engine/network/codec.Packet` 固定头为 11 bytes：

```text
┌────────────┬────────┬────────┬──────────────┬──────────────┐
│ PacketType │ MsgID  │ RpcID  │ PayloadLen   │ Payload      │
│ 1 byte     │ 2 byte │ 4 byte │ 4 byte       │ N bytes      │
└────────────┴────────┴────────┴──────────────┴──────────────┘
```

- 数值使用 big endian；
- Decode 使用 `io.ReadFull`，适合 TCP 字节流；
- Payload 受最大长度限制；
- `Message`、`Request`、`Response` 的语义由 Type 决定。

## 7. Session

Session 封装 `net.Conn`：

```text
NewSession
  → Session Entity
  → AcceptTimeoutComponent
  → IdleCheckerComponent
  → StartReadLoop
  → StartWriteLoop
  → MarkAuthed 移除认证超时
  → Close
```

写入队列容量为 256。`Session.Send` 返回：

- `ErrSessionClosed`；
- `ErrSendChannelFull`；
- codec packet 错误。

调用方必须处理返回值。队列满不再只写日志后假装成功。发送入队和关闭
共享同一把状态锁，关闭边界上的并发发送不会在 Session 已关闭后继续写入。

## 8. TCP

`TCPServer` 提供：

- 显式 `Start(parentCtx)`；
- accept loop；
- Session map；
- `OnConnect`、`OnMessage`、`OnDisconnect`；
- `Stop`。

空监听地址返回 `network.ErrAddressRequired`。TCP Server 不会自动绑定临时本地地址。
`Start` 会拒绝已经取消的父 context；`Stop` 会关闭 listener、等待 accept
loop 退出并清理运行状态，停止后可以用新的 context 重新启动。

## 9. KCP

### 9.1 Service/Channel

```text
ET UDP datagram
  ├── SYN/ACK/Router: Proto(1) + LocalConn(4) + RemoteConn(4)
  ├── FIN:             Proto(1) + LocalConn(4) + RemoteConn(4) + Error(4)
  └── MSG:             Proto(1) + LocalConn(4) + Standard KCP Segment
```

整数按原 ET C# `BitConverter` 的 little-endian 编码。MSG 内部的标准
KCP segment 使用 `RemoteConn` 作为 conversation，因此接收端可以用 KCP
segment 的 conv 找到自己的 local channel，再校验发送方 localConn。

协议包括：

- SYN/ACK；
- FIN；
- MSG；
- Router reconnect SYN/ACK。

`engine/network/kcp/KCP` 使用标准 KCP ARQ 状态机，负责：

- 有序交付；
- 分片和重组；
- ACK；
- 超时重传；
- 快速重传；
- 滑动窗口和拥塞控制。

普通 `[][]byte` 队列不是 KCP 实现，不能替代上述状态机。

`KService.Update`：

1. 清理连接超时；
2. 重发连接 SYN/ACK；
3. 读取 UDP；
4. 分发控制帧和校验 local/remote connection；
5. 输入 KCP segment；
6. 更新 Channel 的 ACK/重传状态。

### 9.2 KCP 的错误边界

- `Listen("")` 返回 `ErrAddressRequired`；
- `Connect` 前必须显式 `Listen`，不再自动绑定 `127.0.0.1:0`；
- 空消息、协议输入错误、KCP 发送错误会返回明确错误；
- Channel 发送错误会关闭 Channel；
- 入站缓冲区满会返回 `ErrReceiveBufferFull` 并关闭 Channel；
- KCP 输出错误由 Service/Channel 处理，不再丢弃。

测试仍可显式使用 `127.0.0.1:0`，因为这是测试调用方主动请求操作系统分配端口。

## 10. Router

Router 不是统一 13 字节大端帧，而是原 ET Router 的方向相关小端帧。Go
实现已按目标 `RouterComponentSystem` 的读取方式固定为 little-endian：

```text
普通 SYN / MSG：
  Protocol(1) + FirstConn(4) + SecondConn(4) + Payload(N)

RouterSYN：
  RouterSYN(1) + OuterConn(4) + InnerConn(4) + ConnectID(4) + InnerAddress(N)

RouterACK / RouterReconnectACK / ACK：
  Protocol(1) + InnerConn(4) + OuterConn(4)

FIN：
  FIN(1) + FirstConn(4) + SecondConn(4) + Error(4)
```

字段方向必须按来源解释：

- 外部 → Router、Router → 内部：`OuterConn` 在前，`InnerConn` 在后；
- 内部 → Router、Router → 外部：`InnerConn` 在前，`OuterConn` 在后；
- `RouterSYN` 只负责外部注册，Router 返回 9 字节 `RouterACK`，不把
  `RouterSYN` 再转发到内部；
- 普通 `SYN`、`MSG`、`FIN` 才由 Router 在外部 UDP/TCP 和内部 UDP 之间转发；
- `RouterReconnectSYN` 转发到内部时只有 9 字节连接字段，内部回
  `RouterReconnectACK` 后，Router 再向外部转发。

RouterNode 状态：

```text
RouterSYN
  → RouterACK
  → ordinary SYN/ACK
  → Msg
  → ReconnectSYN/ACK
  → FIN forwarding/timeout
```

Router 转发失败会返回 `false` 或销毁节点；发送到空地址不再返回 nil 成功。
如果外部 Router transport 缺失，RouterSYN 不会注册节点；内部 transport
缺失时 ordinary SYN、MSG、FIN 和 reconnect 转发返回失败，不会先修改
节点状态再静默成功。

RouterManager 是 HTTP Scene，RouterNode 是 UDP/TCP 中继 Scene，两者是不同运行
职责。RouterNode 生产启动会分别创建外部 UDP、外部 TCP 和绑定机器 `innerIP`
的内部 UDP；测试为了验证兼容路径可以共享 UDP socket，但不能把共享 socket
当作生产拓扑。

外部 TCP 使用目标 TService 风格的 little-endian `uint16` body length 前缀，
每个已接受连接按远端地址绑定 RouterNode，响应沿原 TCP 连接返回。没有已
接受连接时，TCP transport 返回明确错误，不自动伪造连接。WebSocket 属于
WebGL 客户端传输；原 C# 已确认 Router 在 WebGL 构建选择
`WebSocketTransport`/`WebSocketChannel`，但 Go 仓库没有对应 listener、传输
选择配置、连接生命周期和客户端版本矩阵，因此仍保留 TODO，不能把 TCP/UDP
宣称为 WebSocket 兼容。

## 11. HTTP

HTTP Component 的监听是显式的：

```text
Awake
  → 初始化 Dispatcher
Start
  → 校验地址
  → net.Listen
  → server.Serve
```

`HttpComponent.Start` 返回监听错误，异步 Serve 异常可由 `ServeError` 观察。

当前默认 HTTP handler：

- `/login`；
- `/register`；
- `/get_area_list`。

HTTP handler 对 nil request/response、缺少全局配置以及空用户名/密码返回
明确错误或 `400` 响应；缺少依赖不会被解释成业务成功。

RouterManager 额外注册：

- `/router/list`；
- `/zone/list`；
- `/zone/last`。

## 12. 网络状态

| 能力 | 状态 |
|---|---|
| Packet 编解码 | 已测试 |
| Session 读写/RPC | 已测试 |
| TCP Server | 有回环测试 |
| KCP Service/Channel | 有回环测试 |
| Router UDP/TCP frame | 有 UDP/TCP 回环和 wire 测试 |
| 同进程 Actor RPC | 已通过 Fiber queue |
| 跨进程 envelope 接收 API | 已有单元测试 |
| Process peer 配置/握手/Session/重连 | 已有同机 KCP 集成测试 |
| 双进程真实 RPC | 同机双组件自动化通过，真实 OS 进程未验证 |
| TLS、登录限流、审计 | HTTP TLS 强制、证书文件热轮换、MongoDB 原子 bucket 限流和 HTTP 审计写入已有代码/测试；真实 DB、密钥托管和审计保留仍是 TODO；HTTP CORS allowlist 已完成 |
