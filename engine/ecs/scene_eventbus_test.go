package ecs

import (
	"testing"

	"github.com/jerbe/et-go/engine/event"
)

func TestSceneEventBusIsolation(t *testing.T) {
	sceneA := NewScene(SceneTypeMap, 1, "map-a")
	sceneB := NewScene(SceneTypeMap, 1, "map-b")

	if sceneA.EventBus() == nil || sceneB.EventBus() == nil {
		t.Fatal("scene event bus should be initialized")
	}
	if sceneA.EventBus() == sceneB.EventBus() {
		t.Fatal("scene event bus should be isolated per scene")
	}

	var calledA int
	const eventID = event.EventID("scene.event.isolation")

	sceneA.EventBus().Subscribe(eventID, func(args any) {
		calledA++
	})

	sceneB.EventBus().Publish(eventID, "from-b")
	if calledA != 0 {
		t.Fatalf("sceneA handler should not be called by sceneB publish, got %d", calledA)
	}

	sceneA.EventBus().Publish(eventID, "from-a")
	if calledA != 1 {
		t.Fatalf("sceneA handler should be called once, got %d", calledA)
	}
}

func TestSceneDisposeCleansEventBus(t *testing.T) {
	scene := NewScene(SceneTypeMap, 1, "map")
	bus := scene.EventBus()
	if bus == nil {
		t.Fatal("scene event bus should be initialized")
	}

	fired := 0
	const eventID = event.EventID("scene.dispose.cleanup")
	bus.Subscribe(eventID, func(args any) {
		fired++
	})

	scene.Dispose()
	if !scene.IsDisposed() {
		t.Fatal("scene should be disposed")
	}
	if scene.EventBus() != nil {
		t.Fatal("scene event bus should be nil after dispose")
	}

	if _, ok := any(bus).(interface{ ClearAll() }); ok {
		bus.Publish(eventID, "after-dispose")
		if fired != 0 {
			t.Fatalf("disposed scene should clear event handlers, fired = %d", fired)
		}
	}

	scene.Dispose()
}
