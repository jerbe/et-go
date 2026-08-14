package kcp

import (
	"fmt"
	"sync"

	xtacikcp "github.com/xtaci/kcp-go"
)

// KCP 是标准 KCP 状态机的线程安全封装。
//
// 外层 KService 负责 ET 的握手/路由帧；这里仅负责标准 KCP 的可靠、有序、
// 重传、拥塞控制和分片。不能用普通队列替代，否则 UDP 丢包后会静默丢失
// Actor/Session 数据。
type KCP struct {
	mu        sync.Mutex
	config    *Config
	engine    *xtacikcp.KCP
	output    func([]byte) error
	outputErr error
}

// NewKCP 创建 conversation 为 0 的 KCP 实例，保留给单元测试和独立协议使用。
// 连接通道必须使用 NewKCPWithConv，因为 conversation 必须等握手完成后使用
// 对端的本地连接 ID。
func NewKCP(config *Config, output func([]byte) error) *KCP {
	return NewKCPWithConv(0, config, output)
}

// NewKCPWithConv 创建指定 conversation 的 KCP 实例。
func NewKCPWithConv(conv uint32, config *Config, output func([]byte) error) *KCP {
	if config == nil {
		config = InnerConfig()
	}
	c := &KCP{
		config: config,
		output: output,
	}
	c.engine = xtacikcp.NewKCP(conv, func(data []byte, size int) {
		if c.output == nil || size <= 0 {
			return
		}
		if size > len(data) {
			c.outputErr = fmt.Errorf("%w: output size=%d buffer=%d", ErrProtocolSend, size, len(data))
			return
		}
		if err := c.output(append([]byte(nil), data[:size]...)); err != nil {
			c.outputErr = err
		}
	})
	if c.engine.SetMtu(config.MTU) < 0 {
		c.outputErr = fmt.Errorf("%w: invalid mtu=%d", ErrProtocolSend, config.MTU)
	}
	if c.engine.NoDelay(config.NoDelay, config.Interval, config.Resend, config.NC) < 0 {
		c.outputErr = fmt.Errorf("%w: invalid no-delay configuration", ErrProtocolSend)
	}
	if c.engine.WndSize(config.WndSend, config.WndRecv) < 0 {
		c.outputErr = fmt.Errorf("%w: invalid window configuration", ErrProtocolSend)
	}
	if config.MinRTO != 0 && config.MinRTO != xtacikcp.IKCP_RTO_NDL {
		c.outputErr = fmt.Errorf("%w: unsupported min rto=%d", ErrProtocolSend, config.MinRTO)
	}
	return c
}

// Config 返回当前 KCP 配置。
func (k *KCP) Config() *Config {
	if k == nil {
		return nil
	}
	return k.config
}

// Send 将一条消息交给标准 KCP 状态机。
func (k *KCP) Send(data []byte) error {
	if k == nil {
		return ErrProtocolSend
	}
	if len(data) == 0 {
		return ErrMessageEmpty
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.engine == nil {
		return ErrProtocolSend
	}
	if err := k.consumeOutputError(); err != nil {
		return err
	}
	if result := k.engine.Send(data); result < 0 {
		return fmt.Errorf("%w: result=%d", ErrProtocolSend, result)
	}
	return k.consumeOutputError()
}

// Input 把一条标准 KCP wire segment 输入状态机。
func (k *KCP) Input(data []byte) error {
	if k == nil {
		return ErrProtocolInput
	}
	if len(data) == 0 {
		return ErrProtocolInput
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.engine == nil {
		return ErrProtocolInput
	}
	// xtaci/kcp-go 的 Input 允许延迟引用输入切片；外层 UDP 接收缓冲会
	// 复用，因此必须复制，不能把接收缓冲的生命周期泄漏给 KCP。
	input := append([]byte(nil), data...)
	if result := k.engine.Input(input, true, false); result < 0 {
		return fmt.Errorf("%w: result=%d", ErrProtocolInput, result)
	}
	return k.consumeOutputError()
}

// Recv 读取一条已经按序重组的消息。
func (k *KCP) Recv() ([]byte, bool) {
	if k == nil {
		return nil, false
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.engine == nil {
		return nil, false
	}
	size := k.engine.PeekSize()
	if size <= 0 {
		return nil, false
	}
	data := make([]byte, size)
	if read := k.engine.Recv(data); read != size {
		return nil, false
	}
	return data, true
}

// Update 刷新重传、ACK、窗口和拥塞控制状态。
func (k *KCP) Update(_ uint32) error {
	if k == nil {
		return ErrProtocolSend
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.engine == nil {
		return ErrProtocolSend
	}
	k.engine.Update()
	return k.consumeOutputError()
}

func (k *KCP) consumeOutputError() error {
	err := k.outputErr
	k.outputErr = nil
	return err
}
