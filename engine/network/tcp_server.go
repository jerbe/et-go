package network

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/internal/log"
)

// TCPServer TCP 网络服务器
type TCPServer struct {
	addr     string
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	logger   *log.Logger

	sessionIDGen atomic.Int64
	mu           sync.RWMutex
	sessions     map[int64]*Session

	onConnect    func(session *Session)
	onDisconnect func(session *Session)
	onMessage    func(session *Session, pkt *codec.Packet)
}

const (
	defaultAcceptTimeout     = 5 * time.Second
	defaultIdleCheckInterval = 2 * time.Second
	defaultIdleTimeout       = 40 * time.Second
)

// NewTCPServer 创建 TCP 服务器
func NewTCPServer(addr string, logger *log.Logger) *TCPServer {
	return &TCPServer{
		addr:     addr,
		logger:   logger,
		sessions: make(map[int64]*Session),
	}
}

// OnConnect 设置连接回调
func (s *TCPServer) OnConnect(fn func(session *Session)) { s.onConnect = fn }

// OnDisconnect 设置断开回调
func (s *TCPServer) OnDisconnect(fn func(session *Session)) { s.onDisconnect = fn }

// OnMessage 设置消息回调
func (s *TCPServer) OnMessage(fn func(session *Session, pkt *codec.Packet)) { s.onMessage = fn }

// Start 启动 TCP 监听
func (s *TCPServer) Start(parentCtx context.Context) error {
	if s == nil {
		return ErrTCPServerRequired
	}
	if parentCtx == nil {
		return ErrContextRequired
	}
	if err := parentCtx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(s.addr) == "" {
		return ErrAddressRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parentCtx)

	var err error
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		cancel()
		return err
	}
	s.ctx = ctx
	s.cancel = cancel
	s.listener = listener

	if s.logger != nil {
		s.logger.Info("TCP 服务器已启动", "addr", listener.Addr().String())
	}

	go s.acceptLoop(ctx, listener)
	return nil
}

func (s *TCPServer) acceptLoop(ctx context.Context, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				if s.logger != nil {
					s.logger.Error("TCP Accept 错误", "err", err)
				}
				continue
			}
		}
		s.handleConn(ctx, conn)
	}
}

func (s *TCPServer) handleConn(ctx context.Context, conn net.Conn) {
	id := s.sessionIDGen.Add(1)
	session := NewSession(ctx, id, conn, s.logger)
	entity := ecs.NewEntity()
	session.SetEntity(entity)

	acceptTimeoutComponent := NewSessionAcceptTimeoutComponent(session, defaultAcceptTimeout)
	idleCheckerComponent := NewSessionIdleCheckerComponent(session, defaultIdleCheckInterval, defaultIdleTimeout)
	entity.AddComponent(acceptTimeoutComponent)
	entity.AddComponent(idleCheckerComponent)
	session.setAcceptTimeoutComponent(acceptTimeoutComponent)
	session.setIdleCheckerComponent(idleCheckerComponent)

	session.SetOnClose(func() {
		if entity != nil {
			entity.Dispose()
		}

		s.mu.Lock()
		_, ok := s.sessions[id]
		if ok {
			delete(s.sessions, id)
		}
		onDisconnect := s.onDisconnect
		s.mu.Unlock()

		if ok && onDisconnect != nil {
			onDisconnect(session)
		}
	})

	s.mu.Lock()
	s.sessions[id] = session
	onConnect := s.onConnect
	onMessage := s.onMessage
	s.mu.Unlock()

	if onConnect != nil {
		onConnect(session)
	}

	session.StartWriteLoop()
	session.StartReadLoop(func(sess *Session, pkt *codec.Packet) {
		if onMessage != nil {
			onMessage(sess, pkt)
		}
	})
}

// GetSession 获取会话
func (s *TCPServer) GetSession(id int64) (*Session, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// RemoveSession 移除会话
func (s *TCPServer) RemoveSession(id int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	session, ok := s.sessions[id]
	if ok {
		delete(s.sessions, id)
	}
	onDisconnect := s.onDisconnect
	s.mu.Unlock()

	if ok && session != nil {
		session.Close()
		if onDisconnect != nil {
			onDisconnect(session)
		}
	}
}

// Stop 停止服务器
func (s *TCPServer) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	listener := s.listener
	s.cancel = nil
	s.ctx = nil
	s.listener = nil
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session != nil {
			sessions = append(sessions, session)
		}
	}
	s.sessions = make(map[int64]*Session)
	onDisconnect := s.onDisconnect
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) && s.logger != nil {
			s.logger.Warn("TCP 监听关闭失败", "addr", listener.Addr().String(), "err", err)
		}
	}

	for _, session := range sessions {
		session.Close()
		if onDisconnect != nil {
			onDisconnect(session)
		}
	}
}
