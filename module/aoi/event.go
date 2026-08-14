package aoi

import "github.com/jerbe/et-go/engine/event"

const (
	// EventUnitEnterSightRange 表示进入视野事件。
	EventUnitEnterSightRange event.EventID = "aoi.UnitEnterSightRange"
	// EventUnitLeaveSightRange 表示离开视野事件。
	EventUnitLeaveSightRange event.EventID = "aoi.UnitLeaveSightRange"
)

// UnitEnterSightRange 表示 A 看到 B 进入视野。
type UnitEnterSightRange struct {
	A *AOIEntity
	B *AOIEntity
}

// UnitLeaveSightRange 表示 A 看到 B 离开视野。
type UnitLeaveSightRange struct {
	A *AOIEntity
	B *AOIEntity
}
