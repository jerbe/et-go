package statesync

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/unit"
)

// HandleEnterMap 处理进入地图请求。
func HandleEnterMap(scene *ecs.Scene, u *unit.Unit, req *EnterMap) EnterMapResponse {
	if req == nil {
		return EnterMapResponse{Error: 1, Message: ErrUnitMissing.Error()}
	}
	if scene == nil || u == nil {
		return EnterMapResponse{RpcID: req.RpcID, Error: 1, Message: ErrUnitMissing.Error()}
	}

	aoiComponent, ok := u.GetComponent("AOIEntity")
	if !ok {
		return EnterMapResponse{RpcID: req.RpcID, Error: 1, Message: ErrUnitMissing.Error()}
	}
	aoiEntity, ok := aoiComponent.(*aoi.AOIEntity)
	if !ok || aoiEntity == nil {
		return EnterMapResponse{RpcID: req.RpcID, Error: 1, Message: ErrUnitMissing.Error()}
	}

	if err := sendToPlayer(scene, u.ID(), NewCreateMyUnit(u)); err != nil {
		return EnterMapResponse{RpcID: req.RpcID, Error: 1, Message: err.Error()}
	}
	visibleUnits := make([]*UnitInfo, 0, len(aoiEntity.SeeUnits))
	for _, seen := range aoiEntity.SeeUnits {
		if seen == nil {
			continue
		}
		if target := getUnitFromScene(scene, seen.ID); target != nil {
			visibleUnits = append(visibleUnits, CreateUnitInfo(target))
		}
	}
	if len(visibleUnits) > 0 {
		if err := sendToPlayer(scene, u.ID(), &CreateUnits{Units: visibleUnits}); err != nil {
			return EnterMapResponse{RpcID: req.RpcID, Error: 1, Message: err.Error()}
		}
	}
	return EnterMapResponse{RpcID: req.RpcID}
}
