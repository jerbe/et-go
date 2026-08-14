package network

import "errors"

var (
	// ErrSessionClosed 表示会话已关闭。
	ErrSessionClosed = errors.New("network: session is closed")
	// ErrSendChannelFull 表示发送队列已满。
	ErrSendChannelFull = errors.New("network: send channel is full")
	// ErrRpcTimeout 表示 RPC 请求超时。
	ErrRpcTimeout = errors.New("network: rpc timeout")
	// ErrKcpConnectTimeout 表示 KCP 连接超时。
	ErrKcpConnectTimeout = errors.New("network: kcp connect timeout")
	// ErrKcpAcceptTimeout 表示 KCP 接受连接超时。
	ErrKcpAcceptTimeout = errors.New("network: kcp accept timeout")
	// ErrKcpReadWriteTimeout 表示 KCP 读写超时。
	ErrKcpReadWriteTimeout = errors.New("network: kcp read/write timeout")
	// ErrAddressRequired 表示网络服务必须显式提供监听地址。
	ErrAddressRequired = errors.New("network: listen address required")
	// ErrProtocolRequired 表示网络协议必须显式配置。
	ErrProtocolRequired = errors.New("network: protocol required")
	// ErrContextRequired 表示网络服务缺少生命周期上下文。
	ErrContextRequired = errors.New("network: context required")
	// ErrTCPServerRequired 表示 TCP Server 接收者为空。
	ErrTCPServerRequired = errors.New("network: tcp server required")
	// ErrNetComponentClosed 表示 NetComponent 已经销毁。
	ErrNetComponentClosed = errors.New("network: net component closed")
	// ErrPeerConfigInvalid 表示跨进程 peer 配置非法。
	ErrPeerConfigInvalid = errors.New("network: invalid peer config")
	// ErrPeerComponentClosed 表示跨进程 peer 组件已经销毁。
	ErrPeerComponentClosed = errors.New("network: peer component is closed")
	// ErrPeerHandshakeInvalid 表示跨进程握手无效。
	ErrPeerHandshakeInvalid = errors.New("network: invalid peer handshake")
	// ErrPeerHandshakeTimeout 表示跨进程握手超时。
	ErrPeerHandshakeTimeout = errors.New("network: peer handshake timeout")
)
