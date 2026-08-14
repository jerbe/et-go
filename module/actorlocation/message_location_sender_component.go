package actorlocation

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// MessageLocationSenderComponent 管理多种类型的定位消息发送器。
type MessageLocationSenderComponent struct {
	ecs.BaseComponent
	mu      sync.RWMutex
	proxy   *LocationProxyComponent
	sender  MessageSenderClient
	senders map[int]*MessageLocationSender
	closed  bool
}

// Type 返回组件类型名称。
func (c *MessageLocationSenderComponent) Type() string { return "MessageLocationSenderComponent" }

// Awake 初始化内部状态。
func (c *MessageLocationSenderComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.senders == nil {
		c.senders = make(map[int]*MessageLocationSender)
	}
}

// SetDependencies 设置依赖项。
func (c *MessageLocationSenderComponent) SetDependencies(proxy *LocationProxyComponent, sender MessageSenderClient) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.proxy = proxy
	c.sender = sender
	c.senders = make(map[int]*MessageLocationSender)
}

// Get 获取指定类型的发送器。
func (c *MessageLocationSenderComponent) Get(locationType int) *MessageLocationSender {
	if c == nil {
		return nil
	}
	c.Awake()
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil
	}
	if sender, ok := c.senders[locationType]; ok {
		c.mu.RUnlock()
		return sender
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if sender, ok := c.senders[locationType]; ok {
		return sender
	}
	sender := NewMessageLocationSender(LocationType(locationType), c.proxy, c.sender)
	c.senders[locationType] = sender
	return sender
}

// OnDestroy 释放按类型缓存的发送器。
func (c *MessageLocationSenderComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.senders = nil
	c.proxy = nil
	c.sender = nil
	c.mu.Unlock()
}
