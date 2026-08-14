package unit

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/event"
	etmath "github.com/jerbe/et-go/engine/math"
)

const (
	// EventChangePosition 表示单位位置变化事件。
	EventChangePosition event.EventID = "unit.ChangePosition"
	// EventChangeRotation 表示单位旋转变化事件。
	EventChangeRotation event.EventID = "unit.ChangeRotation"
)

// ChangePosition 表示位置变化事件。
type ChangePosition struct {
	Unit   *Unit
	OldPos etmath.Vector3
}

// ChangedEntity 返回发生位置变化的实体。
func (e *ChangePosition) ChangedEntity() *ecs.Entity {
	if e == nil || e.Unit == nil {
		return nil
	}
	return &e.Unit.Entity
}

// ChangedPosition 返回变化后的新位置。
func (e *ChangePosition) ChangedPosition() etmath.Vector3 {
	if e == nil || e.Unit == nil {
		return etmath.Vector3Zero
	}
	return e.Unit.Position()
}

// ChangeRotation 表示旋转变化事件。
type ChangeRotation struct {
	Unit *Unit
}
