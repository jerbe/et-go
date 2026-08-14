package login

import (
	"sync"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

// GateEndpoint 描述一个可分配的 Gate 节点。
type GateEndpoint struct {
	GateId  int64
	Address string
	ActorID actor.ActorID
}

// GateRegistryComponent 保存 Realm 可用 Gate 列表。
type GateRegistryComponent struct {
	ecs.BaseComponent
	mu    sync.RWMutex
	zones map[int32][]GateEndpoint
}

// Type 返回组件名称。
func (c *GateRegistryComponent) Type() string { return "GateRegistryComponent" }

// Awake 初始化内部状态。
func (c *GateRegistryComponent) Awake() {
	if c == nil {
		return
	}
	if c.zones == nil {
		c.zones = make(map[int32][]GateEndpoint)
	}
}

// SetGates 设置分区的 Gate 列表。
func (c *GateRegistryComponent) SetGates(zone int32, endpoints []GateEndpoint) {
	if c == nil {
		return
	}
	c.Awake()
	c.mu.Lock()
	c.zones[zone] = append([]GateEndpoint(nil), endpoints...)
	c.mu.Unlock()
}

// GetGates 返回分区的 Gate 列表。
func (c *GateRegistryComponent) GetGates(zone int32) []GateEndpoint {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]GateEndpoint(nil), c.zones[zone]...)
}
