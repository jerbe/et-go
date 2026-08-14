package gate

import (
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
)

// AcceptTimeoutMs 表示认证超时时间。
const AcceptTimeoutMs = 5000

// IdleCheckIntervalMs 表示空闲检查间隔。
const IdleCheckIntervalMs = 2000

// IdleTimeoutMs 表示空闲超时时间。
const IdleTimeoutMs = 40000

// SessionAcceptTimeoutComponent 复用 network 层认证超时组件。
type SessionAcceptTimeoutComponent = network.SessionAcceptTimeoutComponent

// SessionIdleCheckerComponent 复用 network 层空闲检查组件。
type SessionIdleCheckerComponent = network.SessionIdleCheckerComponent

// NewSessionAcceptTimeoutComponent 创建 Gate 认证超时组件。
func NewSessionAcceptTimeoutComponent(session *network.Session, timeout time.Duration) *SessionAcceptTimeoutComponent {
	return network.NewSessionAcceptTimeoutComponent(session, timeout)
}

// NewSessionIdleCheckerComponent 创建 Gate 空闲检查组件。
func NewSessionIdleCheckerComponent(session *network.Session, checkInterval, idleTimeout time.Duration) *SessionIdleCheckerComponent {
	return network.NewSessionIdleCheckerComponent(session, checkInterval, idleTimeout)
}

// GateSessionComponent 绑定 Session Entity 与网络连接。
type GateSessionComponent struct {
	ecs.BaseComponent
	Session *network.Session
}

// Type 返回组件名称。
func (c *GateSessionComponent) Type() string { return "GateSessionComponent" }
