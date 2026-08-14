package statesync

import (
	"context"
	"log/slog"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/numeric"
	"github.com/jerbe/et-go/module/unit"
)

// HandlePathfindingResult 处理客户端寻路请求。
func HandlePathfindingResult(scene *ecs.Scene, u *unit.Unit, req *PathfindingResultReq) {
	if scene == nil || u == nil || req == nil {
		return
	}

	speedComponent, ok := u.GetComponent("NumericComponent")
	if !ok {
		sendStop(scene, u, 2)
		return
	}
	numericComponent, ok := speedComponent.(*numeric.NumericComponent)
	if !ok || numericComponent == nil {
		sendStop(scene, u, 2)
		return
	}
	speed := numericComponent.GetAsFloat(numeric.Speed)
	if speed < 0.01 {
		sendStop(scene, u, 2)
		return
	}

	pathfindingComponent, ok := u.GetComponent("PathfindingComponent")
	if !ok {
		sendStop(scene, u, 3)
		return
	}
	pathfinding, ok := pathfindingComponent.(*move.PathfindingComponent)
	if !ok || pathfinding == nil {
		sendStop(scene, u, 3)
		return
	}
	points, err := pathfinding.Find(u.Position(), req.Position)
	if err != nil {
		sendStop(scene, u, 3)
		return
	}

	moveComponentRaw, ok := u.GetComponent("MoveComponent")
	if !ok {
		sendStop(scene, u, 3)
		return
	}
	moveComponent, ok := moveComponentRaw.(*move.MoveComponent)
	if !ok || moveComponent == nil {
		sendStop(scene, u, 3)
		return
	}
	done, err := moveComponent.StartMove(points, float32(speed), 0)
	if err != nil {
		sendStop(scene, u, 3)
		return
	}
	if err := BroadcastIncludeSelf(u, &PathfindingResult{
		Id:       u.ID(),
		Position: req.Position,
		Points:   points,
	}); err != nil {
		moveComponent.Stop(false)
		slog.Error("statesync pathfinding broadcast failed", "unit_id", u.ID(), "err", err)
		return
	}
	go func() {
		if err := <-done; err != nil {
			sendStopOnSceneFiber(scene, u, 4)
			return
		}
		sendStopOnSceneFiber(scene, u, 0)
	}()
}

func sendStopOnSceneFiber(scene *ecs.Scene, u *unit.Unit, errorCode int) {
	if scene == nil || u == nil {
		return
	}
	if scene.Fiber() == nil {
		// 独立 ECS 单测没有 Fiber 所有权；真实运行 Scene 必须由 Fiber 驱动。
		sendStop(scene, u, errorCode)
		return
	}
	fiberRef, ok := scene.Fiber().(interface {
		Call(context.Context, func() ([]byte, error)) ([]byte, error)
	})
	if !ok || fiberRef == nil {
		slog.Error("statesync schedule stop failed: scene fiber missing", "unit_id", u.ID())
		return
	}
	if _, err := fiberRef.Call(context.Background(), func() ([]byte, error) {
		sendStop(scene, u, errorCode)
		return nil, nil
	}); err != nil {
		slog.Error("statesync schedule stop failed", "unit_id", u.ID(), "err", err)
	}
}
