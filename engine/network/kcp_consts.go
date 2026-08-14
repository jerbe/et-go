package network

// KCP 协议类型常量。
const (
	KcpSYN             = 1
	KcpACK             = 2
	KcpFIN             = 3
	KcpMSG             = 4
	KcpRouterReconnSYN = 5
	KcpRouterReconnACK = 6
	KcpRouterSYN       = 7
	KcpRouterACK       = 8
)

// KcpConfig 定义 KCP 参数配置。
type KcpConfig struct {
	NoDelay  int
	Interval int
	Resend   int
	NC       int
	WndSend  int
	WndRecv  int
	MTU      int
	MinRTO   int
}

// KcpInnerConfig 进程间通信默认 KCP 配置。
var KcpInnerConfig = KcpConfig{
	NoDelay:  1,
	Interval: 10,
	Resend:   2,
	NC:       1,
	WndSend:  1024,
	WndRecv:  1024,
	MTU:      1400,
	MinRTO:   30,
}

// KcpOuterConfig 客户端通信默认 KCP 配置。
var KcpOuterConfig = KcpConfig{
	NoDelay:  1,
	Interval: 10,
	Resend:   2,
	NC:       1,
	WndSend:  256,
	WndRecv:  256,
	MTU:      470,
	MinRTO:   30,
}

// KcpConnectTimeout KCP 连接超时（毫秒）。
const KcpConnectTimeout = 20000
