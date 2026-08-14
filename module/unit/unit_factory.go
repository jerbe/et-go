package unit

import (
	"fmt"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/numeric"
)

// CreatePlayer 创建玩家单位。
func CreatePlayer(scene *ecs.Scene, id int64) (*Unit, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	if id <= 0 {
		return nil, ErrInvalidUnitID
	}
	component, err := requiredUnitComponent(scene)
	if err != nil {
		return nil, err
	}

	unit := NewUnit(1001, UnitTypePlayer)
	unit.SetID(id)

	parent := component.GetEntity()
	if parent == nil {
		parent = &scene.Entity
	}
	parent.AddChildWithID(id, &unit.Entity)
	if err := component.Add(unit); err != nil {
		unit.Dispose()
		return nil, err
	}

	moveComponent := move.NewMoveComponent()
	unit.AddComponent(moveComponent)
	moveComponent.Bind(unit)
	unit.AddComponent(move.NewPathfindingComponentForScene(scene, scene.Name()))
	unit.SetPosition(etmath.Vector3{X: -10, Y: 0, Z: -10})

	numericComponent := numeric.NewNumericComponent()
	unit.AddComponent(numericComponent)
	numericComponent.SetFloat(numeric.Speed, 6.0)
	numericComponent.Set(numeric.AOI, 15000)

	aoiEntity := aoi.NewAOIEntity(unit.ID(), int(unit.UnitType), 9000)
	aoiEntity.Pos = unit.Position()
	unit.AddComponent(aoiEntity)
	unit.AddComponent(actor.NewMailBox(actorIDForEntity(scene, &unit.Entity), actor.MailBoxTypeOrderedMessage))
	return unit, nil
}

// CreateMonster 创建怪物单位框架。
func CreateMonster(scene *ecs.Scene, id int64, configID int32) (*Unit, error) {
	return createBasicUnit(scene, id, configID, UnitTypeMonster)
}

// CreateNPC 创建 NPC 单位框架。
func CreateNPC(scene *ecs.Scene, id int64, configID int32) (*Unit, error) {
	return createBasicUnit(scene, id, configID, UnitTypeNPC)
}

func requiredUnitComponent(scene *ecs.Scene) (*UnitComponent, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil, ErrUnitComponentMissing
	}
	unitComponent, ok := component.(*UnitComponent)
	if !ok || unitComponent == nil {
		return nil, ErrUnitComponentMissing
	}
	return unitComponent, nil
}

func createBasicUnit(scene *ecs.Scene, id int64, configID int32, unitType UnitType) (*Unit, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	if id <= 0 {
		return nil, ErrInvalidUnitID
	}
	component, err := requiredUnitComponent(scene)
	if err != nil {
		return nil, err
	}

	unit := NewUnit(configID, unitType)
	unit.SetID(id)
	parent := component.GetEntity()
	if parent == nil {
		parent = &scene.Entity
	}
	parent.AddChildWithID(id, &unit.Entity)
	if err := component.Add(unit); err != nil {
		unit.Dispose()
		return nil, err
	}
	return unit, nil
}

func requiredLocationProxy(scene *ecs.Scene) (interface {
	Add(locationType int, key int64, actorID actor.ActorID) error
}, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return nil, ErrLocationProxyMissing
	}
	proxy, ok := component.(interface {
		Add(locationType int, key int64, actorID actor.ActorID) error
	})
	if !ok {
		return nil, ErrLocationProxyMissing
	}
	return proxy, nil
}

func requiredAOIManager(scene *ecs.Scene) (*aoi.AOIManagerComponent, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	component, ok := scene.GetComponent("AOIManagerComponent")
	if !ok || component == nil {
		return nil, ErrAOIManagerMissing
	}
	manager, ok := component.(*aoi.AOIManagerComponent)
	if !ok || manager == nil {
		return nil, ErrAOIManagerMissing
	}
	return manager, nil
}

// InitializeMapUnit 将已创建的 Unit 接入 Map 的 AOI 和 Location 服务。
// 这是 Map 服务边界的必需步骤；缺少依赖时直接返回错误，不生成降级实体。
func InitializeMapUnit(scene *ecs.Scene, unit *Unit) error {
	if scene == nil || unit == nil {
		return ErrSceneMissing
	}
	locationProxy, err := requiredLocationProxy(scene)
	if err != nil {
		return err
	}
	aoiManager, err := requiredAOIManager(scene)
	if err != nil {
		return err
	}
	aoiComponent, ok := unit.GetComponent("AOIEntity")
	if !ok || aoiComponent == nil {
		return ErrAOIManagerMissing
	}
	aoiEntity, ok := aoiComponent.(*aoi.AOIEntity)
	if !ok || aoiEntity == nil {
		return ErrAOIManagerMissing
	}
	aoiManager.Enter(aoiEntity, unit.Position().X, unit.Position().Z)
	if err := registerUnitLocation(locationProxy, unit, scene); err != nil {
		aoiManager.Leave(aoiEntity)
		return err
	}
	return nil
}

func registerUnitLocation(proxy interface {
	Add(locationType int, key int64, actorID actor.ActorID) error
}, unit *Unit, scene *ecs.Scene) error {
	if proxy == nil || unit == nil || scene == nil {
		return ErrLocationRegistration
	}
	if err := proxy.Add(int(actorlocation.LocationTypeUnit), unit.ID(), actorIDForEntity(scene, &unit.Entity)); err != nil {
		return fmt.Errorf("%w: %v", ErrLocationRegistration, err)
	}
	return nil
}

func actorIDForEntity(scene *ecs.Scene, entity *ecs.Entity) actor.ActorID {
	if entity != nil {
		if component, ok := entity.GetComponent("MailBox"); ok {
			if mailbox, ok := component.(*actor.MailBox); ok && mailbox.ActorID().IsValid() {
				return mailbox.ActorID()
			}
		}
	}
	if scene == nil {
		return actor.ActorID{}
	}
	if fiberRef, ok := scene.Fiber().(interface {
		ID() int64
		ProcessID() int
	}); ok {
		actorID := actor.ActorID{
			ProcessID: fiberRef.ProcessID(),
			FiberID:   fiberRef.ID(),
		}
		if entity != nil {
			actorID.InstanceID = entity.InstanceID()
		} else {
			actorID.InstanceID = scene.InstanceID()
		}
		return actorID
	}
	return actor.ActorID{}
}
