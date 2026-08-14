package ecs

import "github.com/jerbe/et-go/engine/event"

// SceneType 定义场景/服务器类型
type SceneType int

const (
	SceneTypeNone       SceneType = 0
	SceneTypeMain       SceneType = 1001
	SceneTypeLaunch     SceneType = 1002
	SceneTypeNetInner   SceneType = 1003
	SceneTypeNetClient  SceneType = 1004
	SceneTypeLocation   SceneType = 3001
	SceneTypeRouter     SceneType = 9001
	SceneTypeRouterNode SceneType = 9002
	SceneTypeRealm      SceneType = 9003
	SceneTypeGate       SceneType = 9004
	SceneTypeLockStep   SceneType = 11001
	SceneTypeMatch      SceneType = 11002
	SceneTypeRoom       SceneType = 11003
	SceneTypeHTTP       SceneType = 16001
	SceneTypeMap        SceneType = 18001
	SceneTypeCentral    SceneType = 20001
)

// String 返回场景类型名称
func (st SceneType) String() string {
	switch st {
	case SceneTypeMain:
		return "Main"
	case SceneTypeLaunch:
		return "Launch"
	case SceneTypeNetInner:
		return "NetInner"
	case SceneTypeNetClient:
		return "NetClient"
	case SceneTypeLocation:
		return "Location"
	case SceneTypeRouter:
		return "Router"
	case SceneTypeRouterNode:
		return "RouterNode"
	case SceneTypeRealm:
		return "Realm"
	case SceneTypeGate:
		return "Gate"
	case SceneTypeLockStep:
		return "LockStep"
	case SceneTypeMatch:
		return "Match"
	case SceneTypeRoom:
		return "Room"
	case SceneTypeHTTP:
		return "HTTP"
	case SceneTypeMap:
		return "Map"
	case SceneTypeCentral:
		return "Central"
	default:
		return "None"
	}
}

// Scene 是 Fiber 的根实体，代表一个服务器场景。
// 每个 Fiber 拥有一个 Scene，Scene 持有该 Fiber 下所有实体。
type Scene struct {
	Entity
	sceneType SceneType
	zone      int // 分区 ID
	name      string
	fiber     any
	eventBus  *event.Bus
	entities  map[int64]*Entity
}

// NewScene 创建新场景
func NewScene(sceneType SceneType, zone int, name string) *Scene {
	if name == "" {
		name = sceneType.String()
	}
	s := &Scene{
		Entity:    *NewEntity(),
		sceneType: sceneType,
		zone:      zone,
		name:      name,
		eventBus:  event.NewBus(),
		entities:  make(map[int64]*Entity),
	}
	s.RegisterEntity(&s.Entity)
	return s
}

// SceneType 返回场景类型
func (s *Scene) SceneType() SceneType { return s.sceneType }

// Zone 返回分区 ID
func (s *Scene) Zone() int { return s.zone }

// Name 返回场景名称
func (s *Scene) Name() string { return s.name }

// SetName 设置场景名称。
func (s *Scene) SetName(name string) {
	if s == nil || name == "" {
		return
	}
	s.name = name
}

// SetFiber 设置场景所属 Fiber 引用。
func (s *Scene) SetFiber(f any) { s.fiber = f }

// Fiber 返回场景所属 Fiber 引用。
func (s *Scene) Fiber() any { return s.fiber }

// EventBus 返回场景事件总线。
func (s *Scene) EventBus() *event.Bus { return s.eventBus }

// RegisterEntity 注册实体到场景索引。
func (s *Scene) RegisterEntity(e *Entity) {
	if s == nil || e == nil {
		return
	}
	s.registerEntityTree(e)
}

// UnregisterEntity 从场景索引移除实体。
func (s *Scene) UnregisterEntity(instanceID int64) {
	if s == nil {
		return
	}
	entity, ok := s.entities[instanceID]
	if !ok {
		return
	}
	s.unregisterEntityTree(entity)
}

// GetEntity 按实例 ID 获取实体。
func (s *Scene) GetEntity(instanceID int64) (*Entity, bool) {
	if s == nil {
		return nil, false
	}
	entity, ok := s.entities[instanceID]
	return entity, ok
}

func (s *Scene) registerEntityTree(e *Entity) {
	if e == nil {
		return
	}
	if e.scene != nil && e.scene != s {
		e.scene.unregisterEntityTree(e)
	}
	e.scene = s
	e.status = e.status.Set(StatusIsRegister)
	if s.entities == nil {
		s.entities = make(map[int64]*Entity)
	}
	s.entities[e.instanceID] = e
	for _, child := range e.children {
		s.registerEntityTree(child)
	}
}

func (s *Scene) unregisterEntityTree(e *Entity) {
	if e == nil {
		return
	}
	delete(s.entities, e.instanceID)
	e.scene = nil
	e.status = e.status.Clear(StatusIsRegister)
	for _, child := range e.children {
		s.unregisterEntityTree(child)
	}
}

// Dispose 销毁场景并清理事件总线。
func (s *Scene) Dispose() {
	if s == nil {
		return
	}
	if s.eventBus != nil {
		if clearAll, ok := any(s.eventBus).(interface{ ClearAll() }); ok {
			clearAll.ClearAll()
		}
		s.eventBus = nil
	}
	s.Entity.Dispose()
}
