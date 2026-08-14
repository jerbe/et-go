package lockstep

import (
	"context"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
)

type sentCall struct {
	playerID int64
	msgID    uint16
	payload  []byte
}

type stubRoomLocationSender struct {
	calls      []sentCall
	failPlayer int64
}

func (s *stubRoomLocationSender) Send(playerID int64, msgID uint16, payload []byte) error {
	if s.failPlayer == playerID && msgID == MsgMatch2GNotifyMatchSuccess {
		return errors.New("simulated gate notification failure")
	}
	s.calls = append(s.calls, sentCall{
		playerID: playerID,
		msgID:    msgID,
		payload:  append([]byte(nil), payload...),
	})
	return nil
}

type stubLocationSenderComponent struct {
	ecs.BaseComponent
	sender *stubRoomLocationSender
}

func (c *stubLocationSenderComponent) Type() string {
	return "MessageLocationSenderComponent"
}

func (c *stubLocationSenderComponent) Get(_ int) roomLocationSender {
	if c.sender == nil {
		c.sender = &stubRoomLocationSender{}
	}
	return c.sender
}

func TestHandleGetRoom(t *testing.T) {
	manager := fiber.NewManager(context.Background(), ecs.NewWorld(), etlog.New("error"))
	defer manager.StopAll()
	mapFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	if mapFiber == nil {
		t.Fatal("map fiber missing")
	}
	scene := mapFiber.Root()
	roomManager := &RoomManagerComponent{}
	roomManager.SetFiberManager(manager)
	scene.AddComponent(roomManager)
	resp := HandleGetRoom(scene, &Match2MapGetRoom{RpcId: 1, PlayerIds: []int64{1}})
	if resp.RoomActorId == 0 || resp.MapActorId == 0 {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestHandleChangeSceneFinishAndHash(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	room := NewLockstepRoom(nil)
	server := NewRoomServerComponent(room)
	server.AddPlayer(NewRoomPlayer(1))
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	updater.SetSnapshotProvider(func(*LockstepRoom) ([]byte, error) {
		return []byte("initial-room-state"), nil
	})
	scene.AddComponent(server)
	scene.AddComponent(updater)

	resp := HandleChangeSceneFinish(scene, &ChangeSceneFinish{RpcId: 1, PlayerId: 1})
	if resp.StartTime == 0 {
		t.Fatalf("expected start time, got %+v", resp)
	}
	if frame, snapshot, ok := room.FrameBuffer.GetNearestSnapshot(1); !ok ||
		frame != 0 || string(snapshot) != "initial-room-state" {
		t.Fatalf("initial room snapshot = frame=%d snapshot=%q ok=%v", frame, snapshot, ok)
	}

	updater.frameBuffer.SetHash(1, 10)
	check := HandleCheckHash(scene, &CheckHashRequest{RpcId: 2, PlayerId: 1, Frame: 1, Hash: 11})
	if check.Frame != 1 {
		t.Fatalf("unexpected check resp: %+v", check)
	}
}

func TestHandleCheckHashBroadcastsMismatchSnapshot(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	sender := &stubLocationSenderComponent{}
	scene.AddComponent(sender)

	room := NewLockstepRoom([]int64{1})
	room.FrameBuffer.SetHash(1, 10)
	room.FrameBuffer.SetSnapshot(1, []byte("snapshot"))
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	scene.AddComponent(updater)

	response, err := handleCheckHash(scene, &CheckHashRequest{
		RpcId:    7,
		PlayerId: 1,
		Frame:    1,
		Hash:     11,
	})
	if err != nil {
		t.Fatalf("handleCheckHash error = %v", err)
	}
	if response.RpcId != 7 || response.Frame != 1 ||
		response.SnapshotFrame != 1 || string(response.Snapshot) != "snapshot" {
		t.Fatalf("check hash response = %+v", response)
	}

	if sender.sender == nil || len(sender.sender.calls) != 1 {
		t.Fatalf("hash failure broadcasts = %+v", sender.sender)
	}
	call := sender.sender.calls[0]
	if call.playerID != 1 || call.msgID != MsgRoom2CCheckHashFail {
		t.Fatalf("hash failure call = %+v", call)
	}
	failure, err := unmarshalRoom2CCheckHashFail(call.payload)
	if err != nil {
		t.Fatalf("decode hash failure payload: %v", err)
	}
	if failure.RpcId != 7 || failure.Frame != 1 ||
		failure.SnapshotFrame != 1 || string(failure.Snapshot) != "snapshot" {
		t.Fatalf("hash failure payload = %+v", failure)
	}
}

func TestFrameBufferCheckAndSetHashIsAtomic(t *testing.T) {
	fb := NewFrameBuffer()
	const attempts = 32
	results := make(chan bool, attempts)
	for index := 0; index < attempts; index++ {
		hash := int64(index + 1)
		go func() {
			_, _, mismatch := fb.CheckAndSetHash(1, hash)
			results <- mismatch
		}()
	}

	mismatches := 0
	for index := 0; index < attempts; index++ {
		if <-results {
			mismatches++
		}
	}
	if mismatches != attempts-1 {
		t.Fatalf("mismatch count = %d, want %d", mismatches, attempts-1)
	}
}

func TestHandleMatchCreatesRoom(t *testing.T) {
	manager := fiber.NewManager(context.Background(), ecs.NewWorld(), etlog.New("error"))
	defer manager.StopAll()

	mapFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	if mapFiber == nil {
		t.Fatal("map fiber missing")
	}
	mapScene := mapFiber.Root()
	roomManager := &RoomManagerComponent{}
	roomManager.SetFiberManager(manager)
	mapScene.AddComponent(roomManager)
	RegisterMapScene(mapScene)

	matchFiber := manager.Create(ecs.SceneTypeMatch, 1, 1, nil)
	if matchFiber == nil {
		t.Fatal("match fiber missing")
	}
	matchFiber.Root().AddComponent(&stubLocationSenderComponent{})
	resp := HandleMatch(matchFiber.Root(), &G2MatchMatch{RpcId: 1, PlayerId: 1001})
	if resp.Error != 0 {
		t.Fatalf("unexpected match resp: %+v", resp)
	}
	component, ok := matchFiber.Root().GetComponent("MatchComponent")
	if !ok {
		t.Fatal("match component missing")
	}
	matchComponent := component.(*MatchComponent)
	room := matchComponent.LastRoom()
	if room == nil || room.RoomActorId == 0 || room.MapActorId == 0 {
		t.Fatalf("room result missing: %+v", room)
	}
}

type stubCancelRoomMessageSender struct {
	ecs.BaseComponent
	mapScene           *ecs.Scene
	cancelledRoom      actor.ActorID
	cancelledPlayerIDs []int64
}

func (s *stubCancelRoomMessageSender) Type() string { return "MessageSender" }

func (s *stubCancelRoomMessageSender) Call(_ context.Context, _ actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	if msgID != MsgMatch2MapCancelRoom {
		return nil, errors.New("unexpected lockstep compensation message")
	}
	req, err := unmarshalMatch2MapCancelRoom(payload)
	if err != nil {
		return nil, err
	}
	s.cancelledRoom = req.RoomActor
	s.cancelledPlayerIDs = append([]int64(nil), req.PlayerIds...)
	resp, err := handleCancelRoom(s.mapScene, req)
	if err != nil {
		return nil, err
	}
	return marshalMap2MatchCancelRoom(resp)
}

func TestHandleMatchCompensatesAfterPartialGateNotification(t *testing.T) {
	manager := fiber.NewManager(context.Background(), ecs.NewWorld(), etlog.New("error"))
	defer manager.StopAll()

	mapFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	if mapFiber == nil {
		t.Fatal("map fiber missing")
	}
	mapScene := mapFiber.Root()
	roomManager := &RoomManagerComponent{}
	roomManager.SetFiberManager(manager)
	mapScene.AddComponent(roomManager)
	RegisterMapScene(mapScene)
	t.Cleanup(func() {
		UnregisterMapScene(mapScene)
		mapScene.Dispose()
	})

	matchScene := ecs.NewScene(ecs.SceneTypeMatch, 1, "match")
	locationSender := &stubLocationSenderComponent{}
	locationSender.sender = &stubRoomLocationSender{failPlayer: 1002}
	matchScene.AddComponent(locationSender)
	cancelSender := &stubCancelRoomMessageSender{mapScene: mapScene}
	matchScene.AddComponent(cancelSender)

	room, err := handleGetRoom(mapScene, &Match2MapGetRoom{
		RpcId:     1,
		PlayerIds: []int64{1001, 1002},
	})
	if err != nil {
		t.Fatalf("handleGetRoom error = %v", err)
	}
	if err := notifyMatchSuccess(matchScene, []int64{1001, 1002}, room); err == nil {
		t.Fatal("partial match notification should fail")
	}
	if err := cancelCreatedRoom(matchScene, []int64{1001, 1002}, room); err != nil {
		t.Fatalf("cancelCreatedRoom error = %v", err)
	}

	if !cancelSender.cancelledRoom.IsValid() {
		t.Fatalf("compensation sender = %#v, want cancelled room actor", cancelSender)
	}
	if _, exists := roomManager.FindRoomByActorID(cancelSender.cancelledRoom.InstanceID); exists {
		t.Fatalf("cancelled room %v still exists", cancelSender.cancelledRoom)
	}

	cancelCount := 0
	for _, call := range locationSender.sender.calls {
		if call.msgID != MsgMatch2GCancelMatchSuccess {
			continue
		}
		cancelCount++
		msg, err := unmarshalMatch2GCancelMatchSuccess(call.payload)
		if err != nil {
			t.Fatalf("decode compensation message: %v", err)
		}
		if msg.PlayerId != call.playerID || msg.RoomActor != cancelSender.cancelledRoom {
			t.Fatalf("compensation message = %+v for player %d", msg, call.playerID)
		}
	}
	if cancelCount != 2 {
		t.Fatalf("compensation notifications = %d, want 2; calls=%+v players=%v", cancelCount, locationSender.sender.calls, cancelSender.cancelledPlayerIDs)
	}
}

func TestHandleMatchRejectsMissingComponentAndInvalidPlayer(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMatch, 1, "match")
	if response := HandleMatch(scene, &G2MatchMatch{RpcId: 1, PlayerId: 1}); response.Error == 0 {
		t.Fatalf("missing MatchComponent should be an error: %+v", response)
	}

	scene.AddComponent(NewMatchComponent())
	if response := HandleMatch(scene, &G2MatchMatch{RpcId: 2, PlayerId: 0}); response.Error == 0 {
		t.Fatalf("invalid player should be an error: %+v", response)
	}
}

func TestHandleMatchRequeuesPlayerWhenMapIsUnavailable(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMatch, 1, "match")
	component := NewMatchComponent()
	scene.AddComponent(component)

	response := HandleMatch(scene, &G2MatchMatch{RpcId: 3, PlayerId: 101})
	if response.Error == 0 {
		t.Fatalf("missing map scene should fail: %+v", response)
	}
	players := component.WaitingPlayers()
	if len(players) != 1 || players[0] != 101 {
		t.Fatalf("waiting players after map failure = %v, want [101]", players)
	}
}

func TestNotifyMatchSuccessRejectsMissingSender(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMatch, 1, "match")
	err := notifyMatchSuccess(scene, []int64{1}, &Map2MatchGetRoom{
		MapActorId:  1,
		RoomActorId: 2,
		MapActor:    actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 1},
		RoomActor:   actor.ActorID{ProcessID: 1, FiberID: 3, InstanceID: 2},
	})
	if !errors.Is(err, ErrMatchNotificationMissing) {
		t.Fatalf("missing match notification sender error = %v, want %v", err, ErrMatchNotificationMissing)
	}
}

func TestHandleFrameMessageAdjustsTime(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	scene.AddComponent(&stubLocationSenderComponent{})
	room := NewLockstepRoom([]int64{1})
	server := NewRoomServerComponent(room)
	server.AddPlayer(NewRoomPlayer(1))
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	scene.AddComponent(server)
	scene.AddComponent(updater)

	req := &FrameMessageRequest{
		RpcId:    1,
		PlayerId: 1,
		Frame:    AdjustTimeThreshold + 1,
		Input:    &LSInput{},
	}
	resp := HandleFrameMessage(scene, req)
	if !resp.Accepted {
		t.Fatalf("frame message rejected: %+v", resp)
	}
	component, _ := scene.GetComponent("MessageLocationSenderComponent")
	senderComponent, ok := component.(*stubLocationSenderComponent)
	if !ok || senderComponent.sender == nil {
		t.Fatal("missing sender component")
	}
	if len(senderComponent.sender.calls) == 0 {
		t.Fatal("expected adjust message")
	}
	call := senderComponent.sender.calls[0]
	if call.msgID != MsgRoom2CAdjustUpdateTime {
		t.Fatalf("unexpected msgID %d", call.msgID)
	}
	adjust, err := unmarshalRoom2CAdjustUpdateTime(call.payload)
	if err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	expected := int64(req.Frame-updater.room.AuthorityFrame) * UpdateIntervalMillis
	if adjust.DiffTime != expected {
		t.Fatalf("unexpected diff time %d, want %d", adjust.DiffTime, expected)
	}
}

func TestHandleFrameMessageRejectsMissingFrameBuffer(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	room := NewLockstepRoom([]int64{1})
	server := NewRoomServerComponent(room)
	updater := NewLSServerUpdater(nil)
	updater.room = room
	scene.AddComponent(server)
	scene.AddComponent(updater)

	resp := HandleFrameMessage(scene, &FrameMessageRequest{
		RpcId:    10,
		PlayerId: 1,
		Frame:    1,
		Input:    &LSInput{},
	})
	if resp.Accepted {
		t.Fatalf("missing FrameBuffer response = %+v, want rejected", resp)
	}
	if resp.RpcId != 10 {
		t.Fatalf("response RpcID = %d, want 10", resp.RpcId)
	}
}

func TestHandleCheckHashRejectsMissingFrameBuffer(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	updater := NewLSServerUpdater(nil)
	scene.AddComponent(updater)

	response, err := handleCheckHash(scene, &CheckHashRequest{
		RpcId:    11,
		PlayerId: 1,
		Frame:    1,
	})
	if !errors.Is(err, ErrFrameBufferMissing) {
		t.Fatalf("missing FrameBuffer error = %v, want %v", err, ErrFrameBufferMissing)
	}
	if response.RpcId != 11 {
		t.Fatalf("response RpcID = %d, want 11", response.RpcId)
	}
}

func TestHandleReconnectReturnsReplay(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRoom, 1, "room")
	room := NewLockstepRoom([]int64{1})
	room.StartTime = 1000
	room.AuthorityFrame = 3
	room.FrameBuffer.SetSnapshot(2, []byte("snap"))
	room.Replay.AddFrameInput(NewOneFrameInputs())
	room.Replay.AddFrameInput(NewOneFrameInputs())
	room.Replay.AddFrameInput(NewOneFrameInputs())
	server := NewRoomServerComponent(room)
	server.AddPlayer(NewRoomPlayer(1))
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	scene.AddComponent(server)
	scene.AddComponent(updater)

	resp := HandleReconnect(scene, &ReconnectRequest{RpcId: 5, PlayerId: 1})
	if resp.StartTime != room.StartTime {
		t.Fatalf("unexpected start time %d", resp.StartTime)
	}
	if resp.Frame != room.AuthorityFrame {
		t.Fatalf("unexpected frame %d", resp.Frame)
	}
	if resp.SnapshotFrame != 2 {
		t.Fatalf("unexpected snapshot frame %d", resp.SnapshotFrame)
	}
	if len(resp.FrameInputs) != room.AuthorityFrame-resp.SnapshotFrame {
		t.Fatalf("unexpected frame inputs %d", len(resp.FrameInputs))
	}
	if !server.PlayerOnline(1) {
		t.Fatal("player should be online")
	}
}

func TestHandleRoomDisposeRemovesRoom(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	manager := &RoomManagerComponent{}
	scene.AddComponent(manager)
	room := manager.AddRoom(1)
	room.RoomActorId = 42
	room.RoomActor = actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 42}
	req := &Room2MNotifyRoomDispose{RoomActorId: 42, RoomActor: room.RoomActor}
	resp := HandleRoomDispose(scene, req)
	if resp.RoomActorId != req.RoomActorId {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if _, ok := manager.GetRoom(1); ok {
		t.Fatal("room not removed")
	}
}
