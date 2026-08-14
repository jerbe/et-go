package router

import (
	"net"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// RouterNode 表示一个路由连接。
type RouterNode struct {
	ecs.Entity

	OuterConnID       uint32
	InnerConn         uint32
	ConnectId         uint32
	InnerAddress      string
	OuterAddr         *net.UDPAddr
	InnerAddr         *net.UDPAddr
	OuterTransport    PacketTransport
	LastRecvOuterTime time.Time
	LastRecvInnerTime time.Time
	LimitCountPerSec  int
	LastLimitReset    time.Time
	RouterSyncCount   int
	SyncCount         int
	Status            RouterStatus
}

// Type returns component name.
func (n *RouterNode) Type() string { return "RouterNode" }

// OuterConn 返回路由节点外部连接 ID。
func (n *RouterNode) OuterConn() uint32 {
	if n == nil {
		return 0
	}
	if id := n.ID(); id > 0 {
		return uint32(id)
	}
	return n.OuterConnID
}

// Awake initializes timestamps.
func (n *RouterNode) Awake() {
	if n.LastLimitReset.IsZero() {
		n.LastLimitReset = time.Now()
	}
	if n.OuterConnID == 0 && n.ID() > 0 {
		n.OuterConnID = uint32(n.ID())
	}
}

// OnDestroy resets fields.
func (n *RouterNode) OnDestroy() {
	n.OuterConnID = 0
	n.InnerConn = 0
	n.ConnectId = 0
	n.InnerAddress = ""
	n.OuterAddr = nil
	n.InnerAddr = nil
	n.OuterTransport = nil
	n.LastRecvOuterTime = time.Time{}
	n.LastRecvInnerTime = time.Time{}
	n.LimitCountPerSec = 0
	n.LastLimitReset = time.Time{}
	n.RouterSyncCount = 0
	n.SyncCount = 0
	n.Status = RouterStatusSync
}

// CheckOuterCount returns false when limit exceeded.
func (n *RouterNode) CheckOuterCount(now time.Time) bool {
	if now.Sub(n.LastLimitReset) >= time.Second {
		n.LimitCountPerSec = 0
		n.LastLimitReset = now
	}
	n.LimitCountPerSec++
	return n.LimitCountPerSec <= 1000
}
