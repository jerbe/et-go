package aoi

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestCellIDAndRadius(t *testing.T) {
	if got := cellID(0, 0); got != 0 {
		t.Fatalf("cellID(0,0) = %d, want 0", got)
	}
	wantID := int64(1)<<32 | 2
	if got := cellID(15.0, 20.0); got != wantID {
		t.Fatalf("cellID(15,20) = %d, want %d", got, wantID)
	}
	x, z := cellXZ(wantID)
	if x != 1 || z != 2 {
		t.Fatalf("cellXZ = (%d,%d), want (1,2)", x, z)
	}
	if enterRadius(9000) != 1 || leaveRadius(9000, true) != 2 || leaveRadius(9000, false) != 1 {
		t.Fatal("radius calculation mismatch")
	}
	if enterRadius(15000) != 2 || enterRadius(10000) != 1 || enterRadius(10001) != 2 {
		t.Fatal("enter radius examples mismatch")
	}
}

func TestAOIEnterLeaveAndMove(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &AOIManagerComponent{}
	scene.AddComponent(manager)

	enterEvents := make(chan *UnitEnterSightRange, 8)
	leaveEvents := make(chan *UnitLeaveSightRange, 8)
	cancelEnter := scene.EventBus().Subscribe(EventUnitEnterSightRange, func(args any) {
		enterEvents <- args.(*UnitEnterSightRange)
	})
	defer cancelEnter()
	cancelLeave := scene.EventBus().Subscribe(EventUnitLeaveSightRange, func(args any) {
		leaveEvents <- args.(*UnitLeaveSightRange)
	})
	defer cancelLeave()

	playerA := NewAOIEntity(1, 5001, 9000)
	playerB := NewAOIEntity(2, 5001, 9000)

	manager.Enter(playerA, 1, 1)
	manager.Enter(playerB, 1, 1)

	if _, ok := playerA.SeeUnits[playerB.ID]; !ok {
		t.Fatal("player A should see player B")
	}
	if _, ok := playerA.SeePlayers[playerB.ID]; !ok {
		t.Fatal("player A should see player B as player")
	}
	if _, ok := playerB.BeSeePlayers[playerA.ID]; !ok {
		t.Fatal("player B should be seen by player A")
	}

	manager.Move(playerB, 40, 1)
	if _, ok := playerA.SeeUnits[playerB.ID]; ok {
		t.Fatal("player A should stop seeing player B after leaving leave radius")
	}

	select {
	case <-enterEvents:
	default:
		t.Fatal("enter event should be published")
	}
	select {
	case <-leaveEvents:
	default:
		t.Fatal("leave event should be published")
	}

	manager.Leave(playerA)
	if len(playerA.SeeUnits) != 0 || len(playerA.BeSeeUnits) != 0 {
		t.Fatal("leave should clear visibility sets")
	}
}

func TestAOIEnterMovesAlreadyRegisteredEntityWithoutLeavingStaleCell(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &AOIManagerComponent{}
	scene.AddComponent(manager)

	entity := NewAOIEntity(1, 5001, 9000)
	manager.Enter(entity, 1, 1)
	oldCell := entity.Cell
	if oldCell == nil {
		t.Fatal("entity should be registered in the first cell")
	}

	manager.Enter(entity, 20000, 1)

	if entity.Cell == nil || entity.Cell == oldCell {
		t.Fatal("re-entering an entity in another cell should move it")
	}
	if _, exists := oldCell.AOIUnits[entity.ID]; exists {
		t.Fatal("old cell should not retain the moved entity")
	}
}

func TestAOIManagerDoesNotReopenAfterDestroy(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &AOIManagerComponent{}
	scene.AddComponent(manager)
	scene.RemoveComponent(manager.Type())

	entity := NewAOIEntity(1, 5001, 9000)
	manager.Enter(entity, 1, 1)
	manager.Move(entity, 2, 2)
	manager.Leave(entity)

	if entity.Cell != nil {
		t.Fatal("destroyed AOI manager should not reattach entities")
	}
}

func TestAOIManagerDestroyClearsEntityState(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &AOIManagerComponent{}
	scene.AddComponent(manager)

	first := NewAOIEntity(10, 5001, 9000)
	second := NewAOIEntity(11, 5001, 9000)
	manager.Enter(first, 1, 1)
	manager.Enter(second, 1, 1)
	if first.Cell == nil || len(first.SeeUnits) == 0 {
		t.Fatal("entities should be registered before destroy")
	}

	manager.OnDestroy()
	if first.Cell != nil || second.Cell != nil {
		t.Fatal("destroy should clear entity cell references")
	}
	if len(first.SeeUnits) != 0 || len(second.SeeUnits) != 0 {
		t.Fatal("destroy should clear entity visibility")
	}
}
