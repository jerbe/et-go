package unit

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/numeric"
)

func TestCreatePlayer(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&UnitComponent{})

	unit, err := CreatePlayer(scene, 10086)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	if unit.ID() != 10086 {
		t.Fatalf("unit ID = %d, want 10086", unit.ID())
	}
	if unit.UnitType != UnitTypePlayer {
		t.Fatalf("unit type = %v, want %v", unit.UnitType, UnitTypePlayer)
	}
	if unit.Position().X != -10 || unit.Position().Z != -10 {
		t.Fatalf("unit position = %v, want (-10,0,-10)", unit.Position())
	}
	if _, ok := unit.GetComponent("MoveComponent"); !ok {
		t.Fatal("player should have MoveComponent")
	}
	if mailbox, ok := unit.GetComponent("MailBox"); !ok || mailbox == nil {
		t.Fatal("player should have MailBox")
	}

	component, ok := unit.GetComponent("NumericComponent")
	if !ok {
		t.Fatal("player should have NumericComponent")
	}
	numericComponent := component.(*numeric.NumericComponent)
	if got := numericComponent.GetAsFloat(numeric.Speed); got != 6.0 {
		t.Fatalf("Speed = %v, want 6.0", got)
	}
	if got := numericComponent.Get(numeric.AOI); got != 15000 {
		t.Fatalf("AOI = %d, want 15000", got)
	}

	aoiComponent, ok := unit.GetComponent("AOIEntity")
	if !ok {
		t.Fatal("player should have AOIEntity")
	}
	aoiEntity := aoiComponent.(*aoi.AOIEntity)
	if aoiEntity.ViewDistance != 9000 {
		t.Fatalf("ViewDistance = %d, want 9000", aoiEntity.ViewDistance)
	}
	if got, ok := scene.GetComponent("UnitComponent"); !ok || got.(*UnitComponent).Count() != 1 {
		t.Fatal("player should be registered in UnitComponent")
	}
}

func TestCreateMonsterAndNPC(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&UnitComponent{})

	monster, err := CreateMonster(scene, 20001, 3001)
	if err != nil {
		t.Fatalf("CreateMonster error = %v", err)
	}
	npc, err := CreateNPC(scene, 20002, 3002)
	if err != nil {
		t.Fatalf("CreateNPC error = %v", err)
	}

	if monster == nil || monster.UnitType != UnitTypeMonster || monster.ConfigId != 3001 {
		t.Fatalf("unexpected monster: %+v", monster)
	}
	if npc == nil || npc.UnitType != UnitTypeNPC || npc.ConfigId != 3002 {
		t.Fatalf("unexpected npc: %+v", npc)
	}
	if got, ok := scene.GetComponent("UnitComponent"); !ok || got.(*UnitComponent).Count() != 2 {
		t.Fatal("monster and npc should both register in UnitComponent")
	}
}
