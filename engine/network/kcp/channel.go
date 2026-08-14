package kcp

import (
	"net"
	"sync"
	"time"
)

const (
	handshakeRetryInterval = 300 * time.Millisecond
	maxPendingSendBytes    = 1024 * 1024
)

// ChannelStatus 表示通道状态。
type ChannelStatus int

const (
	// ChannelStatusNone 表示未初始化。
	ChannelStatusNone ChannelStatus = iota
	// ChannelStatusConnecting 表示连接建立中。
	ChannelStatusConnecting
	// ChannelStatusConnected 表示已连接。
	ChannelStatusConnected
	// ChannelStatusDisconnected 表示已断开。
	ChannelStatusDisconnected
)

// KChannel 封装一个 ET KService 连接。
//
// id 是本地 localConn，remoteConn 是对端 localConn。标准 KCP 的
// conversation 使用 remoteConn，正好与原 C# KChannel 保持一致。
type KChannel struct {
	mu            sync.RWMutex
	id            uint32
	remoteConn    uint32
	remoteAddr    *net.UDPAddr
	service       *KService
	kcp           *KCP
	status        ChannelStatus
	config        *Config
	createTime    int64
	lastRecv      int64
	lastHandshake int64

	pending     [][]byte
	pendingSize int

	onConnected    func(ch *KChannel)
	onDisconnected func(ch *KChannel)
	onRecv         func(ch *KChannel, data []byte)

	conn *channelConn
}

// NewKChannel 创建主动连接通道。
func NewKChannel(id uint32, remoteAddr *net.UDPAddr, config *Config, service *KService) *KChannel {
	return newConnectingKChannel(id, remoteAddr, config, service)
}

func newConnectingKChannel(id uint32, remoteAddr *net.UDPAddr, config *Config, service *KService) *KChannel {
	if config == nil {
		config = InnerConfig()
	}
	now := time.Now().UnixMilli()
	return &KChannel{
		id:         id,
		remoteAddr: cloneUDPAddr(remoteAddr),
		service:    service,
		config:     config,
		status:     ChannelStatusConnecting,
		createTime: now,
		lastRecv:   now,
	}
}

func newAcceptedKChannel(id, remoteConn uint32, remoteAddr *net.UDPAddr, config *Config, service *KService) *KChannel {
	channel := newConnectingKChannel(id, remoteAddr, config, service)
	channel.remoteConn = remoteConn
	// KCP conversation 必须与主动端收到 ACK 后使用的服务端 channel ID
	// 一致；remoteConn 仍保留客户端 localConn，用于外层连接校验和 FIN。
	channel.kcp = NewKCPWithConv(id, channel.config, channel.writeDataFrame)
	return channel
}

// ID 返回本地连接 ID。
func (ch *KChannel) ID() uint32 {
	if ch == nil {
		return 0
	}
	return ch.id
}

// RemoteAddr 返回远端地址。
func (ch *KChannel) RemoteAddr() *net.UDPAddr {
	if ch == nil {
		return nil
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return cloneUDPAddr(ch.remoteAddr)
}

// Status 返回当前状态。
func (ch *KChannel) Status() ChannelStatus {
	if ch == nil {
		return ChannelStatusNone
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.status
}

// SetOnConnected 设置连接成功回调。
func (ch *KChannel) SetOnConnected(fn func(ch *KChannel)) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.onConnected = fn
	ch.mu.Unlock()
}

// SetOnDisconnected 设置断开回调。
func (ch *KChannel) SetOnDisconnected(fn func(ch *KChannel)) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.onDisconnected = fn
	ch.mu.Unlock()
}

// SetOnRecv 设置收到完整消息后的回调。
func (ch *KChannel) SetOnRecv(fn func(ch *KChannel, data []byte)) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.onRecv = fn
	ch.mu.Unlock()
}

// Conn 返回当前通道的 net.Conn 视图。
func (ch *KChannel) Conn() net.Conn {
	if ch == nil {
		return nil
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	if ch.conn == nil {
		return nil
	}
	return ch.conn
}

// Send 发送一条可靠、有序消息。握手尚未完成时先进入有界等待队列。
func (ch *KChannel) Send(data []byte) error {
	if ch == nil {
		return ErrChannelClosed
	}
	if len(data) == 0 {
		return ErrMessageEmpty
	}
	if len(data) > MaxMessageSize {
		return ErrMessageTooLarge
	}
	if ch.service == nil {
		return ErrServiceNotListening
	}

	ch.mu.Lock()
	if ch.status == ChannelStatusDisconnected {
		ch.mu.Unlock()
		return ErrChannelClosed
	}
	if ch.status != ChannelStatusConnected || ch.kcp == nil {
		if ch.pendingSize+len(data) > maxPendingSendBytes {
			ch.mu.Unlock()
			return ErrSendQueueFull
		}
		ch.pending = append(ch.pending, append([]byte(nil), data...))
		ch.pendingSize += len(data)
		ch.mu.Unlock()
		return nil
	}
	engine := ch.kcp
	ch.mu.Unlock()
	return engine.Send(data)
}

// HandleRecv 把标准 KCP segment 输入并读取所有已经重组的消息。
func (ch *KChannel) HandleRecv(data []byte) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.lastRecv = time.Now().UnixMilli()
	engine := ch.kcp
	conn := ch.conn
	onRecv := ch.onRecv
	ch.mu.Unlock()

	if engine == nil {
		ch.close(true)
		return
	}
	if err := engine.Input(data); err != nil {
		ch.close(true)
		return
	}
	for {
		payload, ok := engine.Recv()
		if !ok {
			return
		}
		if conn != nil {
			if err := conn.pushInbound(payload); err != nil {
				ch.close(true)
				return
			}
		}
		if onRecv != nil {
			onRecv(ch, payload)
		}
	}
}

// Update 刷新 KCP 的重传、ACK 和窗口状态。
func (ch *KChannel) Update(now uint32) error {
	if ch == nil {
		return ErrChannelClosed
	}
	ch.mu.RLock()
	engine := ch.kcp
	status := ch.status
	ch.mu.RUnlock()
	if status != ChannelStatusConnected || engine == nil {
		return nil
	}
	if err := engine.Update(now); err != nil {
		ch.close(true)
		return err
	}
	return nil
}

// Close 主动关闭通道。
func (ch *KChannel) Close() {
	if ch != nil {
		ch.close(false)
	}
}

func (ch *KChannel) close(remote bool) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	if ch.status == ChannelStatusDisconnected {
		ch.mu.Unlock()
		return
	}
	ch.status = ChannelStatusDisconnected
	service := ch.service
	conn := ch.conn
	onDisconnected := ch.onDisconnected
	remoteAddr := cloneUDPAddr(ch.remoteAddr)
	remoteConn := ch.remoteConn
	ch.pending = nil
	ch.pendingSize = 0
	ch.mu.Unlock()

	if !remote && service != nil && remoteConn != 0 {
		if err := service.sendControlPair(remoteAddr, ProtoFIN, ch.id, remoteConn, make([]byte, 4)); err != nil {
			service.logWarn("KCP FIN 发送失败", "channel_id", ch.id, "err", err)
		}
	}
	if conn != nil {
		conn.close()
	}
	if onDisconnected != nil {
		onDisconnected(ch)
	}
	if service != nil {
		service.removeChannel(ch.id)
	}
}

// IsTimeout 检查握手是否超时。
func (ch *KChannel) IsTimeout(now int64) bool {
	if ch == nil {
		return false
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.status == ChannelStatusConnecting && now-ch.createTime > ConnectTimeout
}

func (ch *KChannel) markConnected(remoteConnections ...uint32) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	if ch.status == ChannelStatusDisconnected {
		ch.mu.Unlock()
		return
	}
	remoteConn := ch.remoteConn
	if len(remoteConnections) > 0 {
		remoteConn = remoteConnections[0]
	}
	if remoteConn == 0 {
		ch.mu.Unlock()
		return
	}
	if ch.remoteConn != 0 && ch.remoteConn != remoteConn {
		ch.mu.Unlock()
		return
	}
	ch.remoteConn = remoteConn
	if ch.kcp == nil {
		ch.kcp = NewKCPWithConv(remoteConn, ch.config, ch.writeDataFrame)
	}
	ch.status = ChannelStatusConnected
	onConnected := ch.onConnected
	pending := ch.pending
	ch.pending = nil
	ch.pendingSize = 0
	engine := ch.kcp
	ch.mu.Unlock()

	if onConnected != nil {
		onConnected(ch)
	}
	for _, data := range pending {
		if err := engine.Send(data); err != nil {
			ch.close(true)
			return
		}
	}
}

func (ch *KChannel) writeDataFrame(data []byte) error {
	if ch == nil {
		return ErrChannelClosed
	}
	ch.mu.RLock()
	service := ch.service
	remoteAddr := cloneUDPAddr(ch.remoteAddr)
	status := ch.status
	localConn := ch.id
	ch.mu.RUnlock()
	if status != ChannelStatusConnected {
		return ErrChannelNotConnected
	}
	if service == nil {
		return ErrChannelClosed
	}
	return service.sendMessage(remoteAddr, localConn, data)
}

func (ch *KChannel) attachConn(conn *channelConn) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.conn = conn
	ch.mu.Unlock()
}

func (ch *KChannel) setRemoteAddr(addr *net.UDPAddr) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.remoteAddr = cloneUDPAddr(addr)
	ch.mu.Unlock()
}

func (ch *KChannel) remoteConnection() uint32 {
	if ch == nil {
		return 0
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.remoteConn
}

func (ch *KChannel) handshakeState() (localConn, remoteConn uint32, remoteAddr *net.UDPAddr, accepted bool) {
	if ch == nil {
		return 0, 0, nil, false
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.id, ch.remoteConn, cloneUDPAddr(ch.remoteAddr), ch.kcp != nil && ch.status == ChannelStatusConnecting
}

func (ch *KChannel) needsHandshakeRetry(now int64) bool {
	if ch == nil {
		return false
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return ch.status == ChannelStatusConnecting && now-ch.lastHandshake >= handshakeRetryInterval.Milliseconds()
}

func (ch *KChannel) markHandshakeSent(now int64) {
	if ch == nil {
		return
	}
	ch.mu.Lock()
	ch.lastHandshake = now
	ch.mu.Unlock()
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	clone := *addr
	clone.IP = append(net.IP(nil), addr.IP...)
	return &clone
}
