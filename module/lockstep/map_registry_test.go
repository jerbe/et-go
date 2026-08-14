package lockstep

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestPickMapSceneRemovesDisposedScenes(t *testing.T) {
	mapRegistryMu.Lock()
	previous := mapScenes
	mapScenes = make(map[int64]*ecs.Scene)
	mapRegistryMu.Unlock()
	t.Cleanup(func() {
		mapRegistryMu.Lock()
		mapScenes = previous
		mapRegistryMu.Unlock()
	})

	stale := ecs.NewScene(ecs.SceneTypeMap, 1, "stale")
	stale.Dispose()
	RegisterMapScene(stale)

	if scene, err := PickMapScene(); scene != nil || !errors.Is(err, ErrMapSceneMissing) {
		t.Fatalf("PickMapScene = (%v, %v), want (nil, %v)", scene, err, ErrMapSceneMissing)
	}
}

func TestResolveMapSceneRejectsAmbiguousCandidates(t *testing.T) {
	mapRegistryMu.Lock()
	previous := mapScenes
	mapScenes = make(map[int64]*ecs.Scene)
	mapRegistryMu.Unlock()
	t.Cleanup(func() {
		mapRegistryMu.Lock()
		mapScenes = previous
		mapRegistryMu.Unlock()
	})

	first := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	second := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	RegisterMapScene(first)
	RegisterMapScene(second)
	t.Cleanup(func() {
		first.Dispose()
		second.Dispose()
	})

	scene, err := ResolveMapScene()
	if scene != nil || err != ErrMapSceneAmbiguous {
		t.Fatalf("ResolveMapScene = (%v, %v), want (nil, %v)", scene, err, ErrMapSceneAmbiguous)
	}
}

func TestResolveMapSceneUsesExplicitZoneAndName(t *testing.T) {
	mapRegistryMu.Lock()
	previous := mapScenes
	mapScenes = make(map[int64]*ecs.Scene)
	mapRegistryMu.Unlock()
	t.Cleanup(func() {
		mapRegistryMu.Lock()
		mapScenes = previous
		mapRegistryMu.Unlock()
	})

	first := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	second := ecs.NewScene(ecs.SceneTypeMap, 2, "Map2")
	RegisterMapScene(first)
	RegisterMapScene(second)
	t.Cleanup(func() {
		first.Dispose()
		second.Dispose()
	})

	scene, err := ResolveMapSceneForZone(1)
	if err != nil || scene != first {
		t.Fatalf("ResolveMapSceneForZone(1) = (%v, %v), want first scene", scene, err)
	}
	scene, err = ResolveMapSceneByName(2, "map2")
	if err != nil || scene != second {
		t.Fatalf("ResolveMapSceneByName(2, map2) = (%v, %v), want second scene", scene, err)
	}
	if _, err := ResolveMapSceneForZone(3); err != ErrMapSceneMissing {
		t.Fatalf("ResolveMapSceneForZone(3) error = %v, want %v", err, ErrMapSceneMissing)
	}
	if _, err := ResolveMapSceneByName(1, ""); err != ErrMapSceneNameRequired {
		t.Fatalf("ResolveMapSceneByName empty name error = %v, want %v", err, ErrMapSceneNameRequired)
	}
}

func TestResolveDefaultMapScenePrefersHome(t *testing.T) {
	mapRegistryMu.Lock()
	previous := mapScenes
	mapScenes = make(map[int64]*ecs.Scene)
	mapRegistryMu.Unlock()
	t.Cleanup(func() {
		mapRegistryMu.Lock()
		mapScenes = previous
		mapRegistryMu.Unlock()
	})

	home := ecs.NewScene(ecs.SceneTypeMap, 1, "Home")
	secondary := ecs.NewScene(ecs.SceneTypeMap, 1, "Map2")
	RegisterMapScene(home)
	RegisterMapScene(secondary)
	t.Cleanup(func() {
		home.Dispose()
		secondary.Dispose()
	})

	scene, err := ResolveDefaultMapSceneForZone(1)
	if err != nil || scene != home {
		t.Fatalf("ResolveDefaultMapSceneForZone(1) = (%v, %v), want Home scene", scene, err)
	}
}
