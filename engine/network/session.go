package network

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/internal/log"
)

const defaultSessionRPCTimeout = 40 * time.Second

// Session 代表一个客户端连接会话。
// 对应 ET 框架的 Session 概念。
type Session struct {
	id     int64
	conn   net.Conn
	ctx    context.Context
	cancel context.CancelFunc
	logger *log.Logger

	sendCh chan *codec.Packet

	mu       sync.RWMutex
	closed   bool
	userData any // 关联的用户数据

	rpcID      atomic.Uint32
	callbacks  map[uint32]chan *codec.Packet
	callbackMu sync.Mutex

	lastRecvTime atomic.Int64
	lastSendTime atomic.Int64
	readStarted  atomic.Bool
	writeStarted atomic.Bool

	entity        *ecs.Entity
	acceptTimeout *SessionAcceptTimeoutComponent
	idleChecker   *SessionIdleCheckerComponent
	authenticated atomic.Bool

	onClose   func()
	closeOnce sync.Once
}

// NewSession 创建新会话
func NewSession(parentCtx context.Context, id int64, conn net.Conn, logger *log.Logger) *Session {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	session := &Session{
		id:        id,
		conn:      conn,
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger,
		sendCh:    make(chan *codec.Packet, 256),
		callbacks: make(map[uint32]chan *codec.Packet),
	}
	now := time.Now().UnixMilli()
	session.lastRecvTime.Store(now)
	session.lastSendTime.Store(now)
	return session
}

// ID 返回会话 ID
func (s *Session) ID() int64 { return s.id }

// Context 返回 Session 生命周期上下文。
// Session 关闭时该上下文会被取消，业务 RPC 可以据此停止等待。
func (s *Session) Context() context.Context {
	if s == nil || s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

// RemoteAddr 返回远端地址
func (s *Session) RemoteAddr() net.Addr {
	if s.conn == nil {
		return nil
	}
	return s.conn.RemoteAddr()
}

// SetUserData 设置关联的用户数据
func (s *Session) SetUserData(data any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userData = data
}

// UserData 获取关联的用户数据
func (s *Session) UserData() any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userData
}

// Send 发送数据包（异步写入发送队列）。
// 队列满或会话已关闭时返回错误，调用方必须决定重试、关闭连接或上报。
func (s *Session) Send(pkt *codec.Packet) error {
	return s.enqueue(pkt)
}

// StartReadLoop 启动读取循环
func (s *Session) StartReadLoop(handler func(session *Session, pkt *codec.Packet)) {
	if s == nil || s.conn == nil {
		if s != nil {
			s.Close()
		}
		return
	}
	if s.IsClosed() || !s.readStarted.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logWarn("Session 读取处理器 panic", "id", s.id, "panic", recovered)
			}
			s.Close()
		}()
		for {
			pkt, err := codec.Decode(s.conn)
			if err != nil {
				if s.ctx.Err() == nil {
					if s.logger != nil {
						s.logger.Debug("Session 读取错误", "id", s.id, "err", err)
					} else {
						slog.Debug("Session 读取错误", "id", s.id, "err", err)
					}
				}
				return
			}
			s.lastRecvTime.Store(time.Now().UnixMilli())
			if pkt.Type == codec.PacketTypeResponse {
				if s.resolveRpc(pkt) {
					continue
				}
			}
			if handler == nil {
				continue
			}
			handler(s, pkt)
		}
	}()
}

// StartWriteLoop 启动写入循环
func (s *Session) StartWriteLoop() {
	if s == nil || s.conn == nil {
		if s != nil {
			s.Close()
		}
		return
	}
	if s.IsClosed() || !s.writeStarted.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.Close()
		for {
			select {
			case <-s.ctx.Done():
				return
			case pkt := <-s.sendCh:
				if pkt == nil {
					continue
				}
				data, err := codec.Encode(pkt)
				if err != nil {
					if s.logger != nil {
						s.logger.Error("Session 编码错误", "id", s.id, "err", err)
					} else {
						slog.Error("Session 编码错误", "id", s.id, "err", err)
					}
					return
				}
				if _, err := s.conn.Write(data); err != nil {
					if s.logger != nil {
						s.logger.Debug("Session 写入错误", "id", s.id, "err", err)
					} else {
						slog.Debug("Session 写入错误", "id", s.id, "err", err)
					}
					return
				}
				s.lastSendTime.Store(time.Now().UnixMilli())
			}
		}
	}()
}

// NextRpcID 返回下一个 RPC ID。
func (s *Session) NextRpcID() uint32 {
	for {
		id := s.rpcID.Add(1)
		if id != 0 {
			return id
		}
	}
}

// Call 发送 RPC 请求并等待响应。
func (s *Session) Call(ctx context.Context, pkt *codec.Packet) (*codec.Packet, error) {
	if s == nil {
		return nil, ErrSessionClosed
	}
	if pkt == nil {
		return nil, codec.ErrInvalidPacket
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.IsClosed() {
		return nil, ErrSessionClosed
	}

	req := *pkt
	req.Type = codec.PacketTypeRequest
	req.RpcID = s.NextRpcID()

	ch := make(chan *codec.Packet, 1)
	s.callbackMu.Lock()
	s.callbacks[req.RpcID] = ch
	s.callbackMu.Unlock()

	if err := s.enqueue(&req); err != nil {
		s.removeCallback(req.RpcID)
		return nil, err
	}

	timer := time.NewTimer(defaultSessionRPCTimeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		if resp == nil {
			return nil, ErrSessionClosed
		}
		return resp, nil
	case <-ctx.Done():
		s.removeCallback(req.RpcID)
		return nil, ctx.Err()
	case <-timer.C:
		s.removeCallback(req.RpcID)
		return nil, ErrRpcTimeout
	}
}

func (s *Session) resolveRpc(pkt *codec.Packet) bool {
	if pkt == nil || pkt.RpcID == 0 {
		return false
	}
	s.callbackMu.Lock()
	ch, ok := s.callbacks[pkt.RpcID]
	if ok {
		delete(s.callbacks, pkt.RpcID)
	}
	s.callbackMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- pkt:
	default:
	}
	return true
}

// Close 关闭会话
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		onClose := s.onClose
		s.mu.Unlock()

		s.cancel()
		if s.conn != nil {
			if err := s.conn.Close(); err != nil {
				s.logWarn("Session 连接关闭失败", "id", s.id, "err", err)
			}
		}

		s.callbackMu.Lock()
		callbacks := make([]chan *codec.Packet, 0, len(s.callbacks))
		for id, ch := range s.callbacks {
			delete(s.callbacks, id)
			callbacks = append(callbacks, ch)
		}
		s.callbackMu.Unlock()
		for _, ch := range callbacks {
			select {
			case ch <- nil:
			default:
			}
		}

		if onClose != nil {
			onClose()
		}
	})
}

// IsClosed 检查是否已关闭
func (s *Session) IsClosed() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// LastRecvTime 返回最近一次收包时间（毫秒时间戳）。
func (s *Session) LastRecvTime() int64 {
	return s.lastRecvTime.Load()
}

// LastSendTime 返回最近一次发包时间（毫秒时间戳）。
func (s *Session) LastSendTime() int64 {
	return s.lastSendTime.Load()
}

// TouchRecv 更新最近一次收包时间。
func (s *Session) TouchRecv() {
	now := time.Now().UnixMilli()
	s.lastRecvTime.Store(now)
}

// TouchSend 更新最近一次发包时间。
func (s *Session) TouchSend() {
	now := time.Now().UnixMilli()
	s.lastSendTime.Store(now)
}

// SetEntity 设置 Session 关联实体。
func (s *Session) SetEntity(entity *ecs.Entity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entity = entity
}

// Entity 返回 Session 关联实体。
func (s *Session) Entity() *ecs.Entity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entity
}

// SetOnClose 设置关闭回调。
func (s *Session) SetOnClose(fn func()) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if fn != nil {
			fn()
		}
		return
	}
	s.onClose = fn
	s.mu.Unlock()
}

// MarkAuthed 标记会话已认证并移除认证超时组件。
func (s *Session) MarkAuthed() {
	s.authenticated.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entity != nil && s.acceptTimeout != nil {
		s.entity.RemoveComponent(s.acceptTimeout.Type())
		s.acceptTimeout = nil
	}
}

// IsAuthed 返回会话是否已认证。
func (s *Session) IsAuthed() bool {
	return s.authenticated.Load()
}

func (s *Session) setAcceptTimeoutComponent(component *SessionAcceptTimeoutComponent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acceptTimeout = component
}

func (s *Session) setIdleCheckerComponent(component *SessionIdleCheckerComponent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idleChecker = component
}

func (s *Session) enqueue(pkt *codec.Packet) error {
	if s == nil || pkt == nil {
		return codec.ErrInvalidPacket
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrSessionClosed
	}
	select {
	case s.sendCh <- pkt:
		return nil
	default:
		return ErrSendChannelFull
	}
}

func (s *Session) removeCallback(rpcID uint32) {
	s.callbackMu.Lock()
	delete(s.callbacks, rpcID)
	s.callbackMu.Unlock()
}

func (s *Session) logWarn(message string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(message, args...)
		return
	}
	slog.Warn(message, args...)
}
