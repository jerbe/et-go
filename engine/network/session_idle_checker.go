package network

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

const (
	defaultSessionIdleCheckInterval = 2 * time.Second
	defaultSessionIdleTimeout       = 40 * time.Second
)

// SessionIdleCheckerComponent 空闲检查组件。
// 定期检查会话是否长期无活动，若超时则关闭 Session。
type SessionIdleCheckerComponent struct {
	ecs.BaseComponent

	session       *Session
	checkInterval time.Duration
	idleTimeout   time.Duration

	ticker *time.Ticker
	done   chan struct{}
	start  sync.Once
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup

	lastActivity atomic.Int64
}

// NewSessionIdleCheckerComponent 创建空闲检查组件。
func NewSessionIdleCheckerComponent(session *Session, checkInterval, idleTimeout time.Duration) *SessionIdleCheckerComponent {
	if checkInterval <= 0 {
		checkInterval = defaultSessionIdleCheckInterval
	}
	if idleTimeout <= 0 {
		idleTimeout = defaultSessionIdleTimeout
	}
	component := &SessionIdleCheckerComponent{
		session:       session,
		checkInterval: checkInterval,
		idleTimeout:   idleTimeout,
	}
	component.Touch()
	return component
}

// Type 返回组件类型名称。
func (c *SessionIdleCheckerComponent) Type() string { return "SessionIdleCheckerComponent" }

// Awake 启动空闲检查循环。
func (c *SessionIdleCheckerComponent) Awake() {
	if c == nil {
		return
	}
	c.start.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.closed {
			return
		}
		if c.checkInterval <= 0 {
			c.checkInterval = defaultSessionIdleCheckInterval
		}
		if c.idleTimeout <= 0 {
			c.idleTimeout = defaultSessionIdleTimeout
		}
		c.done = make(chan struct{})
		c.Touch()
		c.ticker = time.NewTicker(c.checkInterval)
		c.wg.Add(1)
		go c.checkLoop()
	})
}

// Touch 标记会话活跃时间。
func (c *SessionIdleCheckerComponent) Touch() {
	if c == nil {
		return
	}
	c.lastActivity.Store(time.Now().UnixMilli())
}

func (c *SessionIdleCheckerComponent) checkLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.done:
			return
		case <-c.ticker.C:
			if c.session == nil || c.session.IsClosed() {
				return
			}

			last := c.currentActivityMillis()
			if last <= 0 {
				continue
			}

			now := time.Now().UnixMilli()
			if now-last > c.idleTimeout.Milliseconds() {
				c.session.Close()
				return
			}
		}
	}
}

func (c *SessionIdleCheckerComponent) currentActivityMillis() int64 {
	if c == nil {
		return 0
	}
	last := c.lastActivity.Load()

	if provider, ok := any(c.session).(interface{ LastRecvTime() int64 }); ok {
		if recv := provider.LastRecvTime(); recv > last {
			last = recv
		}
	}
	if provider, ok := any(c.session).(interface{ LastSendTime() int64 }); ok {
		if sent := provider.LastSendTime(); sent > last {
			last = sent
		}
	}
	return last
}

// OnDestroy 停止空闲检查循环。
func (c *SessionIdleCheckerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	ticker := c.ticker
	done := c.done
	c.mu.Unlock()
	if ticker != nil {
		ticker.Stop()
	}
	if done != nil {
		close(done)
	}
	c.wg.Wait()
}
