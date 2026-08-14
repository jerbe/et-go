package lockstep

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestResolveMatchSceneRejectsAmbiguousCandidates(t *testing.T) {
	matchRegistryMu.Lock()
	previous := matchScenes
	matchScenes = make(map[int64]*ecs.Scene)
	matchRegistryMu.Unlock()
	t.Cleanup(func() {
		matchRegistryMu.Lock()
		matchScenes = previous
		matchRegistryMu.Unlock()
	})

	first := ecs.NewScene(ecs.SceneTypeMatch, 1, "Match1")
	second := ecs.NewScene(ecs.SceneTypeMatch, 1, "Match2")
	RegisterMatchScene(first)
	RegisterMatchScene(second)
	t.Cleanup(func() {
		first.Dispose()
		second.Dispose()
	})

	scene, err := ResolveMatchScene()
	if scene != nil || err != ErrMatchSceneAmbiguous {
		t.Fatalf("ResolveMatchScene = (%v, %v), want (nil, %v)", scene, err, ErrMatchSceneAmbiguous)
	}
}

func TestResolveMatchSceneUsesExplicitZoneAndName(t *testing.T) {
	matchRegistryMu.Lock()
	previous := matchScenes
	matchScenes = make(map[int64]*ecs.Scene)
	matchRegistryMu.Unlock()
	t.Cleanup(func() {
		matchRegistryMu.Lock()
		matchScenes = previous
		matchRegistryMu.Unlock()
	})

	first := ecs.NewScene(ecs.SceneTypeMatch, 1, "Match1")
	second := ecs.NewScene(ecs.SceneTypeMatch, 2, "Match2")
	RegisterMatchScene(first)
	RegisterMatchScene(second)
	t.Cleanup(func() {
		first.Dispose()
		second.Dispose()
	})

	scene, err := ResolveMatchSceneForZone(1)
	if err != nil || scene != first {
		t.Fatalf("ResolveMatchSceneForZone(1) = (%v, %v), want first scene", scene, err)
	}
	scene, err = ResolveMatchSceneByName(2, "match2")
	if err != nil || scene != second {
		t.Fatalf("ResolveMatchSceneByName(2, match2) = (%v, %v), want second scene", scene, err)
	}
	if _, err := ResolveMatchSceneForZone(3); err != ErrMatchSceneMissing {
		t.Fatalf("ResolveMatchSceneForZone(3) error = %v, want %v", err, ErrMatchSceneMissing)
	}
	if _, err := ResolveMatchSceneByName(1, ""); err != ErrMatchSceneNameRequired {
		t.Fatalf("ResolveMatchSceneByName empty name error = %v, want %v", err, ErrMatchSceneNameRequired)
	}
}
