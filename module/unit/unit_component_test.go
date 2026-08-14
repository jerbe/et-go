package unit

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestUnitComponentLifecycle(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	component := &UnitComponent{}
	scene.AddComponent(component)

	first := NewUnit(1001, UnitTypePlayer)
	first.SetID(1)
	second := NewUnit(1002, UnitTypeMonster)
	second.SetID(2)

	scene.AddChildWithID(1, &first.Entity)
	scene.AddChildWithID(2, &second.Entity)
	component.Add(first)
	component.Add(second)

	if got := component.Count(); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
	if got, ok := component.Get(1); !ok || got != first {
		t.Fatal("Get should return first unit")
	}
	if len(component.GetAll()) != 2 {
		t.Fatalf("GetAll len = %d, want 2", len(component.GetAll()))
	}

	component.Remove(1)
	if _, ok := component.Get(1); ok {
		t.Fatal("unit 1 should be removed")
	}

	component.OnDestroy()
	if component.Count() != 0 {
		t.Fatal("OnDestroy should clear all units")
	}
	if !second.IsDisposed() {
		t.Fatal("remaining units should be disposed on destroy")
	}
}

func TestUnitComponentRejectsInvalidAndDuplicateUnits(t *testing.T) {
	component := &UnitComponent{}
	if err := component.Add(nil); !errors.Is(err, ErrInvalidUnitID) {
		t.Fatalf("Add nil error = %v, want %v", err, ErrInvalidUnitID)
	}

	first := NewUnit(1001, UnitTypePlayer)
	first.SetID(10)
	if err := component.Add(first); err != nil {
		t.Fatalf("Add first error = %v", err)
	}

	duplicate := NewUnit(1002, UnitTypeMonster)
	duplicate.SetID(10)
	if err := component.Add(duplicate); !errors.Is(err, ErrUnitAlreadyExists) {
		t.Fatalf("Add duplicate error = %v, want %v", err, ErrUnitAlreadyExists)
	}

	component.OnDestroy()
	if err := component.Add(first); !errors.Is(err, ErrUnitComponentClosed) {
		t.Fatalf("Add after destroy error = %v, want %v", err, ErrUnitComponentClosed)
	}
}
