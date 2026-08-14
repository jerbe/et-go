package router

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

func sceneActorID(scene *ecs.Scene) actor.ActorID {
	if scene == nil {
		return actor.ActorID{}
	}
	if fiberRef, ok := scene.Fiber().(interface {
		ID() int64
		ProcessID() int
	}); ok {
		return actor.ActorID{
			ProcessID:  fiberRef.ProcessID(),
			FiberID:    fiberRef.ID(),
			InstanceID: scene.InstanceID(),
		}
	}
	return actor.ActorID{}
}
