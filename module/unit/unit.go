package unit

import (
	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

// Unit 表示游戏世界中的单位实体。
type Unit struct {
	ecs.Entity
	ConfigId int32
	UnitType UnitType
	position etmath.Vector3
	rotation etmath.Quaternion
}

// NewUnit 创建单位实体。
func NewUnit(configID int32, unitType UnitType) *Unit {
	return &Unit{
		Entity:   *ecs.NewEntity(),
		ConfigId: configID,
		UnitType: unitType,
		rotation: etmath.QuaternionIdentity,
	}
}

// Position 返回当前位置。
func (u *Unit) Position() etmath.Vector3 {
	if u == nil {
		return etmath.Vector3Zero
	}
	return u.position
}

// SetPosition 设置当前位置，并发布变化事件。
func (u *Unit) SetPosition(pos etmath.Vector3) {
	if u == nil {
		return
	}
	old := u.position
	if old == pos {
		return
	}
	u.position = pos
	if scene := u.Scene(); scene != nil && scene.EventBus() != nil {
		scene.EventBus().Publish(EventChangePosition, &ChangePosition{
			Unit:   u,
			OldPos: old,
		})
	}
}

// Rotation 返回当前旋转。
func (u *Unit) Rotation() etmath.Quaternion {
	if u == nil {
		return etmath.QuaternionIdentity
	}
	return u.rotation
}

// SetRotation 设置当前旋转，并发布变化事件。
func (u *Unit) SetRotation(rot etmath.Quaternion) {
	if u == nil {
		return
	}
	if u.rotation == rot {
		return
	}
	u.rotation = rot
	if scene := u.Scene(); scene != nil && scene.EventBus() != nil {
		scene.EventBus().Publish(EventChangeRotation, &ChangeRotation{
			Unit: u,
		})
	}
}

// Forward 返回当前朝向向量。
func (u *Unit) Forward() etmath.Vector3 {
	if u == nil {
		return etmath.Vector3Forward
	}
	return etmath.QuaternionForward(u.rotation)
}
