package statesync

import (
	"log/slog"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/unit"
)

// HandleStop 处理客户端停止移动请求。
func HandleStop(scene *ecs.Scene, u *unit.Unit, req *StopReq) {
	if scene == nil || u == nil || req == nil {
		return
	}

	moveComponent, ok := u.GetComponent("MoveComponent")
	if !ok {
		return
	}
	move, ok := moveComponent.(*move.MoveComponent)
	if !ok || move == nil {
		return
	}
	move.Stop(true)
	if err := BroadcastIncludeSelf(u, &Stop{
		Id:       u.ID(),
		Position: u.Position(),
		Rotation: u.Rotation(),
	}); err != nil {
		// One-way stop has no response channel; retain the error in logs rather
		// than reporting a successful notification.
		logStopBroadcastError(u, err)
	}
}

func sendStop(scene *ecs.Scene, u *unit.Unit, errorCode int) {
	if scene == nil || u == nil {
		return
	}
	if err := BroadcastIncludeSelf(u, &Stop{
		Error:    int32(errorCode),
		Id:       u.ID(),
		Position: u.Position(),
		Rotation: u.Rotation(),
	}); err != nil {
		logStopBroadcastError(u, err)
	}
}

func logStopBroadcastError(u *unit.Unit, err error) {
	if u == nil || err == nil {
		return
	}
	slog.Error("statesync stop broadcast failed", "unit_id", u.ID(), "err", err)
}
