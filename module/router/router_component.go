package router

import (
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// PacketTransport 定义 Router 发送 UDP/KCP 包的能力。
type PacketTransport interface {
	Send(to *net.UDPAddr, protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) error
}

// RouterComponent 管理 RouterNode。
type RouterComponent struct {
	ecs.BaseComponent

	mu             sync.RWMutex
	nodesByConnect map[uint32]*RouterNode
	nodesByOuter   map[uint32]*RouterNode
	nextConnectID  uint32
	checkCursor    int

	OuterUDP  PacketTransport
	OuterTCP  PacketTransport
	InnerUDP  PacketTransport
	OuterAddr *net.UDPAddr
	InnerIP   string
	Cache     []byte

	updateListener func()
	nowFunc        func() time.Time
	LastCheckTime  time.Time
	closed         bool
}

// Type returns name.
func (c *RouterComponent) Type() string { return "RouterComponent" }

// Awake initializes maps.
func (c *RouterComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.nodesByConnect == nil {
		c.nodesByConnect = make(map[uint32]*RouterNode)
	}
	if c.nodesByOuter == nil {
		c.nodesByOuter = make(map[uint32]*RouterNode)
	}
	if c.Cache == nil {
		c.Cache = make([]byte, 64*1024)
	}
}

// RegisterUpdateHandler optional.
func (c *RouterComponent) RegisterUpdateHandler(fn func()) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.updateListener = fn
}

// SetTransport 设置底层发送器。
func (c *RouterComponent) SetTransport(transport PacketTransport) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.OuterUDP = transport
	if c.InnerUDP == nil {
		c.InnerUDP = transport
	}
}

// SetOuterTransport 设置 Router 对外传输器。
func (c *RouterComponent) SetOuterTransport(transport PacketTransport) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.OuterUDP = transport
}

// SetInnerTransport 设置 Router 对内传输器。
func (c *RouterComponent) SetInnerTransport(transport PacketTransport) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.InnerUDP = transport
}

// SetOuterTCPTransport 设置 Router 对外 TCP 传输器。
func (c *RouterComponent) SetOuterTCPTransport(transport PacketTransport) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.OuterTCP = transport
}

// Transport 返回底层发送器。
func (c *RouterComponent) Transport() PacketTransport {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OuterUDP
}

// TCPTransport 返回 Router 对外 TCP 传输器。
func (c *RouterComponent) TCPTransport() PacketTransport {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.OuterTCP
}

// InnerTransport 返回 Router 对内传输器。
func (c *RouterComponent) InnerTransport() PacketTransport {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.InnerUDP
}

// SetInnerIP 设置内网地址。
func (c *RouterComponent) SetInnerIP(innerIP string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.InnerIP = innerIP
	}
}

// SetOuterAddr 设置外网监听地址。
func (c *RouterComponent) SetOuterAddr(addr *net.UDPAddr) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.OuterAddr = addr
	}
}

// Now 返回当前时间，便于测试注入。
func (c *RouterComponent) Now() time.Time {
	if c == nil {
		return time.Time{}
	}
	c.mu.RLock()
	nowFunc := c.nowFunc
	c.mu.RUnlock()
	if nowFunc != nil {
		return nowFunc()
	}
	return time.Now()
}

// SetNowFunc 注入测试时间源。
func (c *RouterComponent) SetNowFunc(fn func() time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.nowFunc = fn
	}
}

// AllocConnectID returns next value.
func (c *RouterComponent) AllocConnectID() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextConnectID++
	if c.nextConnectID == 0 {
		c.nextConnectID = 1
	}
	return c.nextConnectID
}

// AddNode adds node.
func (c *RouterComponent) AddNode(node *RouterNode) {
	if c == nil || node == nil {
		return
	}
	c.Awake()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.nodesByConnect[node.ConnectId] = node
	c.nodesByOuter[node.OuterConn()] = node
}

// GetNodeByOuter returns node.
func (c *RouterComponent) GetNodeByOuter(outer uint32) (*RouterNode, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	node, ok := c.nodesByOuter[outer]
	return node, ok
}

// GetNodeByConnect returns node.
func (c *RouterComponent) GetNodeByConnect(connect uint32) (*RouterNode, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	node, ok := c.nodesByConnect[connect]
	return node, ok
}

// RemoveNode removes.
func (c *RouterComponent) RemoveNode(node *RouterNode) {
	if c == nil || node == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodesByConnect, node.ConnectId)
	delete(c.nodesByOuter, node.OuterConn())
}

// Nodes returns snapshot.
func (c *RouterComponent) Nodes() []*RouterNode {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]*RouterNode, 0, len(c.nodesByConnect))
	for _, node := range c.nodesByConnect {
		res = append(res, node)
	}
	return res
}

// OnDestroy 释放底层传输资源。
func (c *RouterComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	outerTransport := c.OuterUDP
	tcpTransport := c.OuterTCP
	innerTransport := c.InnerUDP
	nodes := make([]*RouterNode, 0, len(c.nodesByConnect))
	for id, node := range c.nodesByConnect {
		nodes = append(nodes, node)
		delete(c.nodesByConnect, id)
	}
	c.nodesByOuter = nil
	c.OuterUDP = nil
	c.OuterTCP = nil
	c.InnerUDP = nil
	c.mu.Unlock()

	for _, node := range nodes {
		if node == nil {
			continue
		}
		node.OnDestroy()
		node.Dispose()
	}

	if transport, ok := outerTransport.(pollingTransport); ok && transport != nil {
		if err := transport.Close(); err != nil {
			slog.Error("router outer transport close failed", "err", err)
		}
	}
	if tcpTransport != nil && tcpTransport != outerTransport {
		if transport, ok := tcpTransport.(pollingTransport); ok && transport != nil {
			if err := transport.Close(); err != nil {
				slog.Error("router outer TCP transport close failed", "err", err)
			}
		}
	}
	if innerTransport != nil && innerTransport != outerTransport {
		if transport, ok := innerTransport.(pollingTransport); ok && transport != nil {
			if err := transport.Close(); err != nil {
				slog.Error("router inner transport close failed", "err", err)
			}
		}
	}
}
