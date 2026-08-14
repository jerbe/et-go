package unit

import (
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/numeric"
	"github.com/jerbe/et-go/proto"
)

// CreateUnitInfo 将 Unit 转换为同步结构。
func CreateUnitInfo(unit *Unit) *proto.UnitInfo {
	if unit == nil {
		return nil
	}

	info := &proto.UnitInfo{
		UnitId:   unit.ID(),
		ConfigId: unit.ConfigId,
		Type:     int32(unit.UnitType),
		Position: unit.Position(),
		Forward:  unit.Forward(),
		KV:       make(map[int32]int64),
	}

	if component, ok := unit.GetComponent("NumericComponent"); ok {
		if numericComponent, ok := component.(*numeric.NumericComponent); ok {
			for numType, value := range numericComponent.GetAllFinal() {
				info.KV[int32(numType)] = value
			}
		}
	}

	if component, ok := unit.GetComponent("MoveComponent"); ok {
		if moveComponent, ok := component.(*move.MoveComponent); ok && !moveComponent.IsArrived() {
			points := moveComponent.RemainingTargets()
			moveInfo := &proto.MoveInfo{
				Points:    make([]etmath.Vector3, 0, len(points)+1),
				Rotation:  unit.Rotation(),
				TurnSpeed: int32(moveComponent.TurnTime),
			}
			moveInfo.Points = append(moveInfo.Points, unit.Position())
			moveInfo.Points = append(moveInfo.Points, points...)
			info.MoveInfo = moveInfo
		}
	}

	return info
}
