package aoi_test

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/unit"
)

func TestAOIChangePositionIntegration(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&aoi.AOIManagerComponent{})
	scene.AddComponent(&unit.UnitComponent{})

	entity, err := unit.CreatePlayer(scene, 1001)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	component, ok := entity.GetComponent("AOIEntity")
	if !ok {
		t.Fatal("player should have AOIEntity")
	}
	aoiEntity := component.(*aoi.AOIEntity)
	if aoiEntity.Cell != nil {
		t.Fatal("aoi entity should not be entered before movement")
	}

	entity.SetPosition(entity.Position())
	if aoiEntity.Cell != nil {
		t.Fatal("same position should not trigger enter")
	}

	entity.SetPosition(entity.Position().Add(etmath.Vector3{X: 1, Y: 0, Z: 0}))
	if aoiEntity.Cell == nil {
		t.Fatal("position change should trigger AOI enter")
	}
	if aoiEntity.Pos != entity.Position() {
		t.Fatalf("aoi position = %v, want %v", aoiEntity.Pos, entity.Position())
	}
}
