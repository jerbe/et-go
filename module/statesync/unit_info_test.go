package statesync

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/unit"
)

func TestCreateUnitInfoWrapper(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	player, err := unit.CreatePlayer(scene, 123)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	info := CreateUnitInfo(player)
	if info == nil || info.UnitId != 123 {
		t.Fatalf("CreateUnitInfo returned %+v", info)
	}
}

func TestMessagingStructs(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	player, err := unit.CreatePlayer(scene, 200)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	change := NewStartSceneChange(10, "map-1")
	if change.SceneInstanceId != 10 || change.SceneName != "map-1" {
		t.Fatalf("unexpected change %v", change)
	}
	unitMsg := NewCreateMyUnit(player)
	if unitMsg == nil || unitMsg.Unit == nil || unitMsg.Unit.UnitId != 200 {
		t.Fatalf("NewCreateMyUnit failed: %+v", unitMsg)
	}
}
