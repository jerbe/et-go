package login

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

// PlayerRoomComponent 保存玩家当前所在 Map/Room 的完整 ActorID。
//
// Gate 的外部 Room 消息只能路由到这里记录的 RoomActorID；不能根据
// 客户端提交的 PlayerID 或旧版数值 RoomActorId 猜测目标。
type PlayerRoomComponent struct {
	ecs.BaseComponent
	MapActorID  actor.ActorID
	RoomActorID actor.ActorID
}

// Type 返回组件名称。
func (c *PlayerRoomComponent) Type() string { return "PlayerRoomComponent" }
