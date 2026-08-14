package kcp

import "errors"

var (
	// ErrConnectTimeout 表示连接超时。
	ErrConnectTimeout = errors.New("kcp: connect timeout")
	// ErrAcceptTimeout 表示接受连接超时。
	ErrAcceptTimeout = errors.New("kcp: accept timeout")
	// ErrReadWriteTimeout 表示读写超时。
	ErrReadWriteTimeout = errors.New("kcp: read/write timeout")
	// ErrMessageTooLarge 表示消息超过上限。
	ErrMessageTooLarge = errors.New("kcp: message too large")
	// ErrChannelNotFound 表示通道不存在。
	ErrChannelNotFound = errors.New("kcp: channel not found")
	// ErrChannelClosed 表示通道已关闭。
	ErrChannelClosed = errors.New("kcp: channel closed")
	// ErrAddressRequired 表示网络服务必须显式提供监听地址。
	ErrAddressRequired = errors.New("kcp: listen address required")
	// ErrServiceNotListening 表示主动连接前没有显式启动本地监听。
	ErrServiceNotListening = errors.New("kcp: service is not listening")
	// ErrReceiveBufferFull 表示通道入站缓冲区已满。
	ErrReceiveBufferFull = errors.New("kcp: receive buffer full")
	// ErrMessageEmpty 表示不能发送空消息。
	ErrMessageEmpty = errors.New("kcp: empty message")
	// ErrProtocolInput 表示底层 KCP 拒绝输入帧。
	ErrProtocolInput = errors.New("kcp: protocol input rejected")
	// ErrProtocolSend 表示底层 KCP 拒绝发送消息。
	ErrProtocolSend = errors.New("kcp: protocol send rejected")
	// ErrChannelNotConnected 表示通道尚未完成握手。
	ErrChannelNotConnected = errors.New("kcp: channel not connected")
	// ErrSendQueueFull 表示握手期间的待发送队列已达到上限。
	ErrSendQueueFull = errors.New("kcp: send queue full")
)
