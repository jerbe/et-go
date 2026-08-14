package map_

import (
	"strings"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/move"
)

// MapUnitManagerComponent 管理地图元数据。
type MapUnitManagerComponent struct {
	ecs.BaseComponent
	MapName string
	MapID   int32
	ActorID actor.ActorID
	Targets map[string]actor.ActorID
	finder  move.Finder
	// Targets 必须由部署拓扑或地图配置显式注入；切图请求不会猜测 Map1/Map2。
}

// Type 返回组件名称。
func (c *MapUnitManagerComponent) Type() string { return "MapUnitManagerComponent" }

// Awake 初始化内部状态。
func (c *MapUnitManagerComponent) Awake() {
	if c == nil {
		return
	}
	if c.Targets == nil {
		c.Targets = make(map[string]actor.ActorID)
	}
}

// SetTarget 设置目标地图 Actor。
func (c *MapUnitManagerComponent) SetTarget(mapName string, actorID actor.ActorID) error {
	if c == nil || strings.TrimSpace(mapName) == "" || !actorID.IsValid() {
		return ErrMapTargetInvalid
	}
	c.Awake()
	c.Targets[strings.TrimSpace(mapName)] = actorID
	return nil
}

// ResolveTarget 返回目标地图 Actor。
func (c *MapUnitManagerComponent) ResolveTarget(mapName string) (actor.ActorID, bool) {
	if c == nil || c.Targets == nil {
		return actor.ActorID{}, false
	}
	actorID, ok := c.Targets[strings.TrimSpace(mapName)]
	return actorID, ok
}

// SetPathfindingFinder 注入当前地图真实导航实现。
func (c *MapUnitManagerComponent) SetPathfindingFinder(finder move.Finder) {
	if c == nil {
		return
	}
	c.finder = finder
}

// PathfindingFinder 返回当前地图导航实现。
func (c *MapUnitManagerComponent) PathfindingFinder() move.Finder {
	if c == nil {
		return nil
	}
	return c.finder
}
