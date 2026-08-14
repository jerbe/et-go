package router

// KcpProtocol 定义 router 所用 KCP 协议类型。
type KcpProtocol byte

const (
	KcpSYN             KcpProtocol = 1
	KcpACK             KcpProtocol = 2
	KcpFIN             KcpProtocol = 3
	KcpMSG             KcpProtocol = 4
	KcpRouterReconnSYN KcpProtocol = 5
	KcpRouterReconnACK KcpProtocol = 6
	KcpRouterSYN       KcpProtocol = 7
	KcpRouterACK       KcpProtocol = 8
)

// IsValidKcpProtocol returns true for known values.
func IsValidKcpProtocol(p KcpProtocol) bool {
	switch p {
	case KcpSYN, KcpACK, KcpFIN, KcpMSG, KcpRouterReconnSYN, KcpRouterReconnACK, KcpRouterSYN, KcpRouterACK:
		return true
	default:
		return false
	}
}
