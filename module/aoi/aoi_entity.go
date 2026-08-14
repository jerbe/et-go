package aoi

import (
	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

const playerUnitType = 5001

// AOIEntity AOI 实体组件。
type AOIEntity struct {
	ecs.BaseComponent
	ID           int64
	UnitType     int
	ViewDistance int
	Pos          etmath.Vector3
	Cell         *Cell
	SeeUnits     map[int64]*AOIEntity
	BeSeeUnits   map[int64]*AOIEntity
	SeePlayers   map[int64]*AOIEntity
	BeSeePlayers map[int64]*AOIEntity
}

// NewAOIEntity 创建 AOIEntity。
func NewAOIEntity(id int64, unitType int, viewDistance int) *AOIEntity {
	entity := &AOIEntity{
		ID:           id,
		UnitType:     unitType,
		ViewDistance: viewDistance,
	}
	entity.Awake()
	return entity
}

// Type 返回组件类型名称。
func (e *AOIEntity) Type() string { return "AOIEntity" }

// Awake 初始化内部集合。
func (e *AOIEntity) Awake() {
	if e.ViewDistance <= 0 {
		e.ViewDistance = 9000
	}
	if e.SeeUnits == nil {
		e.SeeUnits = make(map[int64]*AOIEntity)
	}
	if e.BeSeeUnits == nil {
		e.BeSeeUnits = make(map[int64]*AOIEntity)
	}
	if e.SeePlayers == nil {
		e.SeePlayers = make(map[int64]*AOIEntity)
	}
	if e.BeSeePlayers == nil {
		e.BeSeePlayers = make(map[int64]*AOIEntity)
	}
}

// IsPlayer 返回是否为玩家单位。
func (e *AOIEntity) IsPlayer() bool {
	if e == nil {
		return false
	}
	return e.UnitType == playerUnitType
}

func (e *AOIEntity) resetVisibility() {
	e.SeeUnits = make(map[int64]*AOIEntity)
	e.BeSeeUnits = make(map[int64]*AOIEntity)
	e.SeePlayers = make(map[int64]*AOIEntity)
	e.BeSeePlayers = make(map[int64]*AOIEntity)
}
