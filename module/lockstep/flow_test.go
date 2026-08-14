package lockstep

import (
	"context"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/engine/network"
)

type sentRoomMessage struct {
	playerID int64
	msgID    uint16
	payload  []byte
}

type fakeGateLocationSenderComponent struct {
	ecs.BaseComponent
	sent []sentRoomMessage
}

func (c *fakeGateLocationSenderComponent) Type() string { return "MessageLocationSenderComponent" }

func (c *fakeGateLocationSenderComponent) SendToGate(playerID int64, msgID uint16, payload []byte) error {
	c.sent = append(c.sent, sentRoomMessage{
		playerID: playerID,
		msgID:    msgID,
		payload:  append([]byte(nil), payload...),
	})
	return nil
}

func (c *fakeGateLocationSenderComponent) Get(int) roomLocationSender {
	return fakeGateLocationSender{component: c}
}

type fakeGateLocationSender struct {
	component *fakeGateLocationSenderComponent
}

func (s fakeGateLocationSender) Send(playerID int64, msgID uint16, payload []byte) error {
	return s.component.SendToGate(playerID, msgID, payload)
}

type fakeSessionPlayerComponent struct {
	ecs.BaseComponent
	unitID int64
}

func (c *fakeSessionPlayerComponent) Type() string { return "SessionPlayerComponent" }

func (c *fakeSessionPlayerComponent) GetUnitID() int64 { return c.unitID }

func TestHandleChangeSceneFinishBroadcastsStart(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	sender := &fakeGateLocationSenderComponent{}
	scene.AddComponent(sender)

	room := NewLockstepRoom([]int64{1, 2})
	server := NewRoomServerComponent(room)
	scene.AddComponent(server)

	resp := HandleChangeSceneFinish(scene, &ChangeSceneFinish{RpcId: 1, PlayerId: 1})
	if resp.StartTime != 0 {
		t.Fatalf("unexpected start on first ready: %+v", resp)
	}
	resp = HandleChangeSceneFinish(scene, &ChangeSceneFinish{RpcId: 2, PlayerId: 2})
	if resp.StartTime == 0 || len(resp.UnitInfos) != 2 {
		t.Fatalf("expected start response, got %+v", resp)
	}
	if len(sender.sent) != 2 {
		t.Fatalf("expected 2 start broadcasts, got %d", len(sender.sent))
	}
	for _, msg := range sender.sent {
		if msg.msgID != MsgRoom2CStart {
			t.Fatalf("unexpected msg id %d", msg.msgID)
		}
	}
}

func TestHandleFrameMessageAdjustsAndStoresInput(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	sender := &fakeGateLocationSenderComponent{}
	scene.AddComponent(sender)

	room := NewLockstepRoom([]int64{1})
	room.AuthorityFrame = 0
	room.StartTime = time.Now().Add(-time.Second).UnixMilli()
	server := NewRoomServerComponent(room)
	scene.AddComponent(server)
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	scene.AddComponent(updater)

	resp := HandleFrameMessage(scene, &FrameMessageRequest{
		RpcId:    1,
		PlayerId: 1,
		Frame:    4,
		Input:    &LSInput{Button: 7},
	})
	if !resp.Accepted {
		t.Fatalf("expected accepted response, got %+v", resp)
	}
	frameInputs, ok := room.FrameBuffer.GetFrameInputs(4)
	if !ok || frameInputs.Inputs[1] == nil || frameInputs.Inputs[1].Button != 7 {
		t.Fatalf("frame input missing: %+v", frameInputs)
	}
	if len(sender.sent) != 1 || sender.sent[0].msgID != MsgRoom2CAdjustUpdateTime {
		t.Fatalf("expected adjust-time message, got %+v", sender.sent)
	}
}

func TestHandleReconnectIncludesReplayTail(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	room := NewLockstepRoom([]int64{1})
	room.StartTime = 12345
	room.AuthorityFrame = 2
	room.FrameBuffer.SetSnapshot(1, []byte("snap"))
	room.Replay.AddFrameInput(&OneFrameInputs{Inputs: map[int64]*LSInput{1: &LSInput{Button: 1}}})
	room.Replay.AddFrameInput(&OneFrameInputs{Inputs: map[int64]*LSInput{1: &LSInput{Button: 2}}})

	server := NewRoomServerComponent(room)
	scene.AddComponent(server)
	server.SetPlayerOnline(1, false)

	resp := HandleReconnect(scene, &ReconnectRequest{RpcId: 9, PlayerId: 1})
	if resp.StartTime != room.StartTime || resp.Frame != room.AuthorityFrame {
		t.Fatalf("unexpected reconnect resp: %+v", resp)
	}
	if resp.SnapshotFrame != 1 || string(resp.Snapshot) != "snap" {
		t.Fatalf("unexpected snapshot info: %+v", resp)
	}
	if len(resp.FrameInputs) != 1 || resp.FrameInputs[0].Inputs[1].Button != 2 {
		t.Fatalf("unexpected replay tail: %+v", resp.FrameInputs)
	}
	if !server.PlayerOnline(1) {
		t.Fatal("player should be marked online")
	}
}

func TestHandleRoomDisposeRemovesRoomByActorID(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &RoomManagerComponent{}
	scene.AddComponent(manager)
	room := manager.AddRoom(1)
	room.RoomActorId = 999
	room.RoomActor = actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}

	HandleRoomDispose(scene, &Room2MNotifyRoomDispose{
		RoomActorId: 999,
		RoomActor:   room.RoomActor,
	})
	if _, ok := manager.GetRoom(1); ok {
		t.Fatal("room should be removed")
	}
}

func TestHandleCancelRoomIsIdempotent(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &RoomManagerComponent{}
	scene.AddComponent(manager)
	room := manager.AddRoom(1)
	room.RoomActor = actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}
	room.RoomActorId = room.RoomActor.InstanceID
	defer scene.Dispose()

	first := HandleCancelRoom(scene, &Match2MapCancelRoom{
		RpcId:     11,
		RoomActor: room.RoomActor,
		PlayerIds: []int64{1001},
	})
	if first.RpcId != 11 || !first.Cancelled {
		t.Fatalf("first cancel response = %+v", first)
	}
	if _, ok := manager.GetRoom(1); ok {
		t.Fatal("first cancel should remove room")
	}

	second := HandleCancelRoom(scene, &Match2MapCancelRoom{
		RpcId:     12,
		RoomActor: room.RoomActor,
		PlayerIds: []int64{1001},
	})
	if second.RpcId != 12 || !second.Cancelled {
		t.Fatalf("repeated cancel response = %+v", second)
	}
}

func TestHandleC2GMatchUsesSessionUnitID(t *testing.T) {
	manager := fiber.NewManager(context.Background(), ecs.NewWorld(), nil)
	defer manager.StopAll()
	mapFiber := fiber.New(context.Background(), ecs.SceneTypeMap, 1, 1)
	t.Cleanup(func() {
		UnregisterMapScene(mapFiber.Root())
		mapFiber.Root().Dispose()
	})
	mapScene := mapFiber.Root()
	roomManager := &RoomManagerComponent{}
	roomManager.SetFiberManager(manager)
	mapScene.AddComponent(roomManager)
	RegisterMapScene(mapScene)

	matchScene := ecs.NewScene(ecs.SceneTypeMatch, 1, "match")
	matchScene.AddComponent(NewMatchComponent())
	matchScene.AddComponent(&stubLocationSenderComponent{})
	RegisterMatchScene(matchScene)

	session := network.NewSession(context.Background(), 1, nil, nil)
	sessionEntity := ecs.NewEntity()
	sessionEntity.AddComponent(&fakeSessionPlayerComponent{unitID: 1001})
	session.SetEntity(sessionEntity)

	resp, err := HandleC2GMatch(nil, session, &C2GMatch{RpcId: 7})
	if err != nil {
		t.Fatalf("HandleC2GMatch err = %v", err)
	}
	if resp.Error != 0 {
		t.Fatalf("unexpected match response: %+v", resp)
	}
}

func TestHandleC2GMatchRejectsSessionPlayerSpoofing(t *testing.T) {
	session := network.NewSession(context.Background(), 1, nil, nil)
	sessionEntity := ecs.NewEntity()
	sessionEntity.AddComponent(&fakeSessionPlayerComponent{unitID: 1001})
	session.SetEntity(sessionEntity)

	resp, err := HandleC2GMatch(nil, session, &C2GMatch{RpcId: 8, PlayerId: 1002})
	if err != nil {
		t.Fatalf("HandleC2GMatch error = %v", err)
	}
	if resp == nil || resp.Error == 0 {
		t.Fatalf("spoofed player response = %+v, want error", resp)
	}
}

func TestStartBroadcastPayloadShape(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	sender := &fakeGateLocationSenderComponent{}
	scene.AddComponent(sender)
	room := NewLockstepRoom([]int64{1})
	server := NewRoomServerComponent(room)
	scene.AddComponent(server)

	HandleChangeSceneFinish(scene, &ChangeSceneFinish{RpcId: 1, PlayerId: 1})
	if len(sender.sent) != 1 {
		t.Fatalf("expected one broadcast, got %d", len(sender.sent))
	}
	payload, err := unmarshalRoom2CStart(sender.sent[0].payload)
	if err != nil {
		t.Fatalf("unmarshal start payload err = %v", err)
	}
	if payload.StartTime == 0 || len(payload.UnitInfos) != 1 || payload.UnitInfos[0].PlayerId != 1 {
		t.Fatalf("unexpected payload %+v", payload)
	}
	if payload.UnitInfos[0].Position != (etmath.Vector3{X: 20, Y: 0, Z: -10}) ||
		etmath.QuaternionDistance(payload.UnitInfos[0].Rotation, etmath.QuaternionIdentity) > 0.000001 {
		t.Fatalf("unexpected unit transform %+v", payload.UnitInfos[0])
	}
}
