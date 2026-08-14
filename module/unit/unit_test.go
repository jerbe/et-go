package unit

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

func TestUnitSetPositionAndRotationEvents(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	component := &UnitComponent{}
	scene.AddComponent(component)

	unit := NewUnit(1001, UnitTypePlayer)
	scene.AddChildWithID(1001, &unit.Entity)
	component.Add(unit)

	positionEvents := make(chan *ChangePosition, 2)
	rotationEvents := make(chan *ChangeRotation, 2)
	cancelPosition := scene.EventBus().Subscribe(EventChangePosition, func(args any) {
		positionEvents <- args.(*ChangePosition)
	})
	defer cancelPosition()
	cancelRotation := scene.EventBus().Subscribe(EventChangeRotation, func(args any) {
		rotationEvents <- args.(*ChangeRotation)
	})
	defer cancelRotation()

	oldPos := unit.Position()
	unit.SetPosition(etmath.Vector3{X: 1, Y: 2, Z: 3})

	select {
	case evt := <-positionEvents:
		if evt.Unit != unit {
			t.Fatal("position event unit mismatch")
		}
		if evt.OldPos != oldPos {
			t.Fatalf("OldPos = %v, want %v", evt.OldPos, oldPos)
		}
		if unit.Position() != (etmath.Vector3{X: 1, Y: 2, Z: 3}) {
			t.Fatal("position should update before event delivery")
		}
	default:
		t.Fatal("position event should be published")
	}

	unit.SetPosition(etmath.Vector3{X: 1, Y: 2, Z: 3})
	select {
	case <-positionEvents:
		t.Fatal("same position should not publish event")
	default:
	}

	unit.SetRotation(etmath.LookRotation(etmath.Vector3{X: 1, Y: 0, Z: 0}))
	select {
	case evt := <-rotationEvents:
		if evt.Unit != unit {
			t.Fatal("rotation event unit mismatch")
		}
	default:
		t.Fatal("rotation event should be published")
	}
}
