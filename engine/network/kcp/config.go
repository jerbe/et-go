package kcp

// Config 定义 KCP 连接参数配置。
type Config struct {
	NoDelay  int
	Interval int
	Resend   int
	NC       int
	WndSend  int
	WndRecv  int
	MTU      int
	MinRTO   int
}

// InnerConfig 返回进程间通信的 KCP 配置。
func InnerConfig() *Config {
	return &Config{
		NoDelay:  1,
		Interval: 10,
		Resend:   2,
		NC:       1,
		WndSend:  1024,
		WndRecv:  1024,
		MTU:      1400,
		MinRTO:   30,
	}
}

// OuterConfig 返回客户端通信的 KCP 配置。
func OuterConfig() *Config {
	return &Config{
		NoDelay:  1,
		Interval: 10,
		Resend:   2,
		NC:       1,
		WndSend:  256,
		WndRecv:  256,
		MTU:      470,
		MinRTO:   30,
	}
}
