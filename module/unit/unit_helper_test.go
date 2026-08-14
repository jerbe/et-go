package unit

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/numeric"
)

func TestCreateUnitInfoStaticAndMoving(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&UnitComponent{})

	staticUnit, err := CreatePlayer(scene, 1)
	if err != nil {
		t.Fatalf("CreatePlayer static error = %v", err)
	}
	staticInfo := CreateUnitInfo(staticUnit)
	if staticInfo == nil {
		t.Fatal("CreateUnitInfo should return info")
	}
	if staticInfo.UnitId != 1 || staticInfo.ConfigId != 1001 {
		t.Fatalf("unexpected static unit info: %+v", staticInfo)
	}
	if staticInfo.MoveInfo != nil {
		t.Fatal("static unit should not contain MoveInfo")
	}
	if staticInfo.KV[int32(numeric.Speed)] != 60000 {
		t.Fatalf("Speed KV = %d, want 60000", staticInfo.KV[int32(numeric.Speed)])
	}

	movingUnit, err := CreatePlayer(scene, 2)
	if err != nil {
		t.Fatalf("CreatePlayer moving error = %v", err)
	}
	moveComponent, _ := movingUnit.GetComponent("MoveComponent")
	moveState := moveComponent.(*move.MoveComponent)
	moveState.StartPos = movingUnit.Position()
	moveState.Targets = []etmath.Vector3{
		movingUnit.Position(),
		{X: 5, Y: 0, Z: 5},
		{X: 10, Y: 0, Z: 10},
	}
	moveState.N = 1
	moveState.TurnTime = 150

	movingInfo := CreateUnitInfo(movingUnit)
	if movingInfo == nil || movingInfo.MoveInfo == nil {
		t.Fatal("moving unit should include MoveInfo")
	}
	if len(movingInfo.MoveInfo.Points) != 3 {
		t.Fatalf("MoveInfo points len = %d, want 3", len(movingInfo.MoveInfo.Points))
	}
	if movingInfo.MoveInfo.Points[0] != movingUnit.Position() {
		t.Fatalf("first move point = %v, want current position %v", movingInfo.MoveInfo.Points[0], movingUnit.Position())
	}
	if movingInfo.MoveInfo.TurnSpeed != 150 {
		t.Fatalf("TurnSpeed = %d, want 150", movingInfo.MoveInfo.TurnSpeed)
	}
}
