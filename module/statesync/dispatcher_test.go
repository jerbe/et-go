package statesync

import (
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/unit"
)

func TestOrderedDispatcherEnterMap(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	sender := &testLocationSender{}
	scene.AddComponent(sender)

	player, err := unit.CreatePlayer(scene, 100)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	other, err := unit.CreateNPC(scene, 101, 2001)
	if err != nil {
		t.Fatalf("CreateNPC error = %v", err)
	}
	otherAOI := aoi.NewAOIEntity(other.ID(), int(other.UnitType), 9000)
	other.AddComponent(otherAOI)
	playerAOI := mustAOI(t, player)
	playerAOI.SeeUnits[other.ID()] = otherAOI

	mailbox := mustMailbox(t, player)
	payload, err := marshalEnterMap(&EnterMap{RpcID: 1})
	if err != nil {
		t.Fatalf("marshal enter map err = %v", err)
	}
	respBytes, err := mailbox.Dispatch(MsgC2MEnterMap, payload)
	if err != nil {
		t.Fatalf("dispatch enter map err = %v", err)
	}
	resp, err := unmarshalEnterMapResponse(respBytes)
	if err != nil {
		t.Fatalf("unmarshal enter map resp err = %v", err)
	}
	if resp.Error != 0 || resp.RpcID != 1 {
		t.Fatalf("unexpected response %+v", resp)
	}
	if sender.Count() != 2 {
		t.Fatalf("expected 2 enter map messages, got %d", sender.Count())
	}
}

func TestOrderedDispatcherPathfindingAndStop(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	sender := &testLocationSender{}
	scene.AddComponent(sender)

	player, err := unit.CreatePlayer(scene, 10)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	pathfinding := &move.PathfindingComponent{}
	pathfinding.SetFinder(&testFinder{points: []etmath.Vector3{
		player.Position(),
		player.Position().Add(etmath.Vector3{X: 0, Y: 0, Z: 4}),
	}})
	player.AddComponent(pathfinding)
	mailbox := mustMailbox(t, player)

	pathPayload, err := marshalPathfindingResultReq(&PathfindingResultReq{
		RpcID:    1,
		Position: player.Position().Add(etmath.Vector3{X: 0, Y: 0, Z: 4}),
	})
	if err != nil {
		t.Fatalf("marshal pathfinding err = %v", err)
	}
	if _, err := mailbox.Dispatch(MsgC2MPathfindingResult, pathPayload); err != nil {
		t.Fatalf("dispatch pathfinding err = %v", err)
	}
	if sender.Count() == 0 {
		t.Fatal("expected pathfinding broadcast")
	}
	first := sender.Messages()[0]
	if first.msgID != MsgPathfindingResult {
		t.Fatalf("expected MsgPathfindingResult, got %d", first.msgID)
	}

	stopPayload, err := marshalStopReq(&StopReq{RpcID: 2})
	if err != nil {
		t.Fatalf("marshal stop err = %v", err)
	}
	if _, err := mailbox.Dispatch(MsgC2MStop, stopPayload); err != nil {
		t.Fatalf("dispatch stop err = %v", err)
	}
	last, ok := sender.Last()
	if !ok || last.msgID != MsgStop {
		t.Fatal("expected stop broadcast")
	}
}

func mustMailbox(t *testing.T, u *unit.Unit) *actor.MailBox {
	t.Helper()
	component, ok := u.GetComponent("MailBox")
	if !ok || component == nil {
		t.Fatal("MailBox missing")
	}
	mailbox, ok := component.(*actor.MailBox)
	if !ok || mailbox == nil {
		t.Fatal("MailBox type mismatch")
	}
	return mailbox
}
