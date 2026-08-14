package actor

import (
	"context"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

func TestResolveSceneActorsSkipsDisposedScene(t *testing.T) {
	sceneFiber := fiber.New(context.Background(), ecs.SceneTypeMap, 1, 1)
	scene := sceneFiber.Root()
	UpdateSceneRegistry(scene)
	defer RemoveSceneRegistry(scene)

	if _, ok := ResolveSceneActor(1, ecs.SceneTypeMap, scene.Name()); !ok {
		t.Fatal("registered scene should be resolvable")
	}

	scene.Dispose()

	if _, ok := ResolveSceneActor(1, ecs.SceneTypeMap, scene.Name()); ok {
		t.Fatal("disposed scene must not be returned by registry")
	}
}

func TestResolveSceneActorRejectsAmbiguousUnnamedMatch(t *testing.T) {
	first := fiber.New(context.Background(), ecs.SceneTypeCentral, 1, 1)
	second := fiber.New(context.Background(), ecs.SceneTypeCentral, 1, 1)
	first.Root().SetName("Central1")
	second.Root().SetName("Central2")
	UpdateSceneRegistry(first.Root())
	UpdateSceneRegistry(second.Root())
	t.Cleanup(func() {
		RemoveSceneRegistry(first.Root())
		RemoveSceneRegistry(second.Root())
		first.Root().Dispose()
		second.Root().Dispose()
	})

	if _, ok := ResolveSceneActor(1, ecs.SceneTypeCentral, ""); ok {
		t.Fatal("unnamed resolution should reject multiple matching scenes")
	}
	if _, ok := ResolveSceneActor(1, ecs.SceneTypeCentral, first.Root().Name()); !ok {
		t.Fatal("named resolution should select the requested scene")
	}
}
