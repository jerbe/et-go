# M18-router：RouterManager HTTP 与 RouterNode UDP/TCP 中继

> `M18` 是原 ET 的兼容审计编号；Go 代码按两个运行对象组织：
> `module/router` 的 RouterManager HTTP Scene 与 RouterNode UDP/TCP Scene。

## 目标边界

目标项目证据：

- `cn.etetet.router/Scripts/Hotfix/Server/FiberInit_RouterManager.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/FiberInit_Router.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/RouterComponentSystem.cs`
- `cn.etetet.router/Scripts/Model/Server/RouterNode.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/Http/HttpPostLoginHandler.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/Http/HttpGetRouterHandler.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/Http/HttpGetZoneListHandler.cs`
- `cn.etetet.router/Scripts/Hotfix/Server/Http/HttpGetLastZoneHandler.cs`

## 任务状态

| 任务 | Go 实现 | 验证 |
|---|---|---|
| RouterManager 注册 `/login`、`/router/list`、`/zone/list`、`/zone/last` | `fiber_init_router_mgr.go` | `http_handlers_test.go`、Fiber integration |
| RouterManager 只依赖 MessageSender，不安装 DB Manager | `initRouterManagerFiber` | `TestRouterManagerFiberInitAlignment` |
| ServerIP、Realm 内网地址、Router 外网地址解析 | `resolveServerIP/resolveSceneAddress` | `TestRouterListHandler` |
| Logic Zone 列表和最后一个 Logic Zone | `configuredLogicZoneList/configuredLastLogicZone` | Zone/LastZone tests |
| `/zone/last` AccessToken 校验 | `httpLastZoneHandler` | invalid/expired token tests |
| RouterNode Sync/Msg 状态和 10 秒/50 秒超时 | `RouterNode`、`RouterComponentSystem.checkTimeout` | state/timeout tests |
| RouterSYN 注册、RouterACK 返回 | `HandleRouterSYNWithConnect` | UDP integration/wire tests |
| 外部 ordinary SYN 转内部、内部 ACK 转外部 | `HandleOuterSYN/HandleRouterACK` | `TestRouterForwardFullFlow` |
| MSG/FIN 双向校验和转发 | `HandleOuterMsg/HandleInnerMsg/HandleOuterFIN/HandleInnerFIN` | UDP integration/state tests |
| RouterReconnectSYN/ACK | `HandleRouterReconnSYNWithState/HandleRouterReconnACKWithState` | reconnect tests |
| 小端、9/13 字节目标帧 | `transport.go` | `TestRouterFrameUsesTargetLittleEndianLayout` |
| 生产外部 UDP/TCP、内部 UDP 分离 | `fiber_init_router.go`、`RouterComponent`、`tcp_transport.go` | TCP framing/connection reuse test、Fiber startup path |

## Router wire 结论

Go 当前不再使用旧的统一 13 字节大端帧。目标协议如下：

```text
RouterSYN:          1 + outer(4) + inner(4) + connectID(4) + address
RouterACK/ACK:      1 + inner(4) + outer(4)
ordinary SYN/MSG:   1 + sourceConn(4) + destinationConn(4) + payload
FIN:               1 + sourceConn(4) + destinationConn(4) + error(4)
ReconnectSYN:       外部注册帧带 connectID；转发到内部时为 9 字节
ReconnectACK:       1 + inner(4) + outer(4)
```

所有 uint32 使用 little-endian。外部到内部写 `outer, inner`，内部到
外部写 `inner, outer`。RouterSYN 只创建/更新节点并返回 RouterACK；
ordinary SYN/ACK 才启动内部 KCP 连接。

## 状态和失败边界

- RouterSYN 缺少外部传输器时失败且不注册；ordinary forwarding/reconnect
  缺少内部传输器时失败，不先修改节点；
- RouterSYN connect ID 与已有节点不一致：返回 `ErrRouterConnectIDMismatch`；
- ACK、MSG、FIN 校验 OuterConn/InnerConn；RouterSYN/Reconnect 校验
  ConnectID；
- 外部包速率超过 1000/s：拒绝当前包；
- Sync 超过 10 秒、Msg 双向 50 秒无流量：销毁节点；
- FIN 在 Router 层先转发，显式清理或超时才销毁节点；
- 地址、端口、Machine/Process/Scene 配置缺失：启动或 handler 返回错误；
- 不按随机 map 遍历顺序选择路由节点，不把缺失传输器当成功。

## 兼容性范围

当前 Go M18 对齐并验证目标 UDP/KCP Router 数据面，并实现了非 WebGL
外部 TCP transport。TCP 使用目标 TService 风格的 little-endian `uint16`
body length 前缀，接收 RouterSYN 后复用已接受的 TCP 连接发送 RouterACK、
ordinary ACK、MSG、FIN 和 reconnect ACK；没有已接受连接时不会偷偷主动拨号。

目标 C# 的 WebGL 构建使用 WebSocket。Go 仓库目前没有 WebSocket 客户端、
传输选择配置和连接生命周期协议，因此 `fiber_init_router.go` 保留
`TODO(router-transport)`，不能把 TCP 或 UDP 宣称为 WebSocket 兼容。

## 验证命令

```bash
go test -count=1 ./module/router ./module/http
go test -race -count=1 ./module/login ./module/central ./module/router ./module/map_ ./module/actorlocation ./cmd/server
```

其中 `TestTCPTransportUsesTargetOuterLengthPrefixAndRouterWire` 验证 TCP
外层长度前缀、RouterSYN 注册、RouterACK 回写和已接受连接复用。
UDP/TCP 回环和 wire 测试通过不等于真实客户端、跨机器地址、防火墙、
WebSocket 变体和压力验收通过；这些证据仍记录在
`docs/IMPLEMENTATION_STATUS.md`。
