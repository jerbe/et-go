package lockstep

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

type RoomPlayer struct {
	ecs.Entity
	IsOnline    bool
	Progress    int
	GateActorID actor.ActorID
}

func NewRoomPlayer(playerId int64) *RoomPlayer {
	p := &RoomPlayer{
		Entity:   *ecs.NewEntity(),
		IsOnline: true,
	}
	p.SetID(playerId)
	return p
}

func (p *RoomPlayer) Awake() {
	p.IsOnline = true
	p.Progress = 0
}

func (p *RoomPlayer) OnDestroy() {
	p.IsOnline = false
	p.Progress = 0
	p.GateActorID = actor.ActorID{}
}
