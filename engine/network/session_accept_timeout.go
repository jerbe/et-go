package network

import (
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

const defaultSessionAcceptTimeout = 5 * time.Second

// SessionAcceptTimeoutComponent 认证超时组件。
// 如果会话在超时时间内未完成认证（未移除本组件），将自动关闭 Session。
type SessionAcceptTimeoutComponent struct {
	ecs.BaseComponent

	session *Session
	timeout time.Duration
	timer   *time.Timer
	start   sync.Once
	mu      sync.Mutex
	closed  bool
}

// NewSessionAcceptTimeoutComponent 创建认证超时组件。
func NewSessionAcceptTimeoutComponent(session *Session, timeout time.Duration) *SessionAcceptTimeoutComponent {
	if timeout <= 0 {
		timeout = defaultSessionAcceptTimeout
	}
	return &SessionAcceptTimeoutComponent{
		session: session,
		timeout: timeout,
	}
}

// Type 返回组件类型名称。
func (c *SessionAcceptTimeoutComponent) Type() string { return "SessionAcceptTimeoutComponent" }

// Awake 启动认证超时定时器。
func (c *SessionAcceptTimeoutComponent) Awake() {
	if c == nil {
		return
	}
	c.start.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return
		}
		if c.timeout <= 0 {
			c.timeout = defaultSessionAcceptTimeout
		}
		c.timer = time.AfterFunc(c.timeout, func() {
			c.mu.Lock()
			closed := c.closed
			session := c.session
			c.mu.Unlock()
			if !closed && session != nil && !session.IsClosed() {
				session.Close()
			}
		})
	})
}

// OnDestroy 停止定时器。
func (c *SessionAcceptTimeoutComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	timer := c.timer
	c.timer = nil
	c.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}
