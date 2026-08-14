package kcp

const (
	// ProtoSYN 表示连接请求。
	ProtoSYN byte = 1
	// ProtoACK 表示连接确认。
	ProtoACK byte = 2
	// ProtoFIN 表示断开连接。
	ProtoFIN byte = 3
	// ProtoMSG 表示普通数据消息。
	ProtoMSG byte = 4
	// ProtoRouterReconnSYN 表示路由重连请求。
	ProtoRouterReconnSYN byte = 5
	// ProtoRouterReconnACK 表示路由重连确认。
	ProtoRouterReconnACK byte = 6
	// ProtoRouterSYN 表示路由连接请求。
	ProtoRouterSYN byte = 7
	// ProtoRouterACK 表示路由连接确认。
	ProtoRouterACK byte = 8
)

const (
	// ConnectTimeout 表示连接超时毫秒数。
	ConnectTimeout int64 = 20000
	// AcceptTimeout 表示 Accept 等待超时毫秒数。
	AcceptTimeout int64 = 20000
	// MaxMessageSize 表示最大消息大小。
	MaxMessageSize = 10000
)
