package map_

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/unit"
)

// HandleG2MEnterMap 初始化玩家在地图中的单位与位置。
func HandleG2MEnterMap(scene *ecs.Scene, req *G2MEnterMap) *M2GEnterMap {
	if req == nil {
		return &M2GEnterMap{
			Error:   1,
			Message: ErrTransferUnitMissing.Error(),
		}
	}
	if scene == nil || req.PlayerID == 0 {
		return &M2GEnterMap{
			RpcID:   req.RpcID,
			Error:   1,
			Message: ErrTransferUnitMissing.Error(),
		}
	}

	if component, ok := scene.GetComponent("MapUnitManagerComponent"); ok {
		if manager, ok := component.(*MapUnitManagerComponent); ok && manager.MapName == "" {
			manager.MapName = scene.Name()
		}
	}

	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return &M2GEnterMap{RpcID: req.RpcID, Error: 1, Message: unit.ErrUnitComponentMissing.Error()}
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok || unitComponent == nil {
		return &M2GEnterMap{RpcID: req.RpcID, Error: 1, Message: unit.ErrUnitComponentMissing.Error()}
	}
	if existing, ok := unitComponent.Get(req.PlayerID); ok && existing != nil && !existing.IsDisposed() {
		return &M2GEnterMap{RpcID: req.RpcID}
	}

	u, err := unit.CreatePlayer(scene, req.PlayerID)
	if err != nil {
		return &M2GEnterMap{
			RpcID:   req.RpcID,
			Error:   1,
			Message: err.Error(),
		}
	}
	u.AddComponent(&inventory.BagComponent{})
	u.AddComponent(&inventory.WarehouseComponent{})
	if err := unit.InitializeMapUnit(scene, u); err != nil {
		u.Dispose()
		unitComponent.Remove(req.PlayerID)
		return &M2GEnterMap{
			RpcID:   req.RpcID,
			Error:   1,
			Message: err.Error(),
		}
	}
	return &M2GEnterMap{RpcID: req.RpcID}
}
