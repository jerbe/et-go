package statesync

import (
	"fmt"
	"log/slog"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/event"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/unit"
)

// RegisterAOIHandlers 注册 AOI 事件处理。
func RegisterAOIHandlers(eventBus *event.Bus) {
	if eventBus == nil {
		return
	}
	eventBus.Subscribe(aoi.EventUnitEnterSightRange, func(args any) {
		event, ok := args.(*aoi.UnitEnterSightRange)
		if !ok || event == nil {
			slog.Error("statesync invalid AOI enter event", "type", eventType(args))
			return
		}
		onUnitEnterSight(event)
	})
	eventBus.Subscribe(aoi.EventUnitLeaveSightRange, func(args any) {
		event, ok := args.(*aoi.UnitLeaveSightRange)
		if !ok || event == nil {
			slog.Error("statesync invalid AOI leave event", "type", eventType(args))
			return
		}
		onUnitLeaveSight(event)
	})
}

func onUnitEnterSight(args *aoi.UnitEnterSightRange) {
	if args == nil || args.A == nil || args.B == nil || !args.A.IsPlayer() {
		return
	}
	scene := sceneFromEntity(args.A)
	if scene == nil {
		return
	}
	target := getUnitFromScene(scene, args.B.ID)
	if target == nil {
		return
	}
	sendToPlayer(scene, args.A.ID, &CreateUnits{
		Units: []*UnitInfo{CreateUnitInfo(target)},
	})
}

func onUnitLeaveSight(args *aoi.UnitLeaveSightRange) {
	if args == nil || args.A == nil || args.B == nil || !args.A.IsPlayer() {
		return
	}
	scene := sceneFromEntity(args.A)
	if scene == nil {
		return
	}
	sendToPlayer(scene, args.A.ID, &RemoveUnits{
		Units: []int64{args.B.ID},
	})
}

func getUnitFromScene(scene *ecs.Scene, id int64) *unit.Unit {
	if scene == nil || id <= 0 {
		return nil
	}
	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok {
		return nil
	}
	target, ok := unitComponent.Get(id)
	if !ok {
		return nil
	}
	return target
}

func eventType(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", value)
}

func sceneFromEntity(entity *aoi.AOIEntity) *ecs.Scene {
	if entity == nil || entity.GetEntity() == nil {
		return nil
	}
	return entity.GetEntity().Scene()
}
