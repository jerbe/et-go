package login

import (
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

const defaultGateTokenExpire = 20 * time.Second

// GateSessionKeyComponent 管理 Gate Token 与账号的映射。
type GateSessionKeyComponent struct {
	ecs.BaseComponent

	mu         sync.RWMutex
	sessionKey map[string]int64
	timers     map[string]*time.Timer
	expiry     time.Duration
	afterFunc  func(time.Duration, func()) *time.Timer
	closed     bool
}

// NewGateSessionKeyComponent 创建 GateSessionKeyComponent。
func NewGateSessionKeyComponent(expiry time.Duration) *GateSessionKeyComponent {
	return &GateSessionKeyComponent{
		expiry: expiry,
	}
}

// Type 返回组件名称。
func (c *GateSessionKeyComponent) Type() string { return "GateSessionKeyComponent" }

// Awake 初始化内部状态。
func (c *GateSessionKeyComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.expiry <= 0 {
		c.expiry = defaultGateTokenExpire
	}
	if c.afterFunc == nil {
		c.afterFunc = time.AfterFunc
	}
	if c.sessionKey == nil {
		c.sessionKey = make(map[string]int64)
	}
	if c.timers == nil {
		c.timers = make(map[string]*time.Timer)
	}
}

// OnDestroy 停止所有过期定时器。
func (c *GateSessionKeyComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	defer c.mu.Unlock()
	for key, timer := range c.timers {
		if timer != nil {
			timer.Stop()
		}
		delete(c.timers, key)
	}
	c.sessionKey = nil
}

// SetAfterFunc 设置过期定时器工厂，便于测试。
func (c *GateSessionKeyComponent) SetAfterFunc(fn func(time.Duration, func()) *time.Timer) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if fn == nil {
		fn = time.AfterFunc
	}
	c.afterFunc = fn
	c.mu.Unlock()
}

// Add 注册 Gate Token。
func (c *GateSessionKeyComponent) Add(token string, accountId int64) error {
	if c == nil || token == "" || accountId <= 0 {
		return ErrInvalidLoginRequest
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrGateSessionKeyClosed
	}
	if timer, ok := c.timers[token]; ok && timer != nil {
		timer.Stop()
	}
	c.sessionKey[token] = accountId
	timer := c.afterFunc(c.expiry, func() {
		c.Remove(token)
	})
	if timer == nil {
		delete(c.sessionKey, token)
		delete(c.timers, token)
		c.mu.Unlock()
		return ErrGateSessionKeyTimerMissing
	}
	c.timers[token] = timer
	c.mu.Unlock()
	return nil
}

// Get 查询 Gate Token。
func (c *GateSessionKeyComponent) Get(token string) (int64, bool) {
	if c == nil {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	accountId, ok := c.sessionKey[token]
	return accountId, ok
}

// Remove 移除 Gate Token。
func (c *GateSessionKeyComponent) Remove(token string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if timer, ok := c.timers[token]; ok && timer != nil {
		timer.Stop()
		delete(c.timers, token)
	}
	delete(c.sessionKey, token)
}
