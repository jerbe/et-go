package lockstep

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/login"
	locksteppb "github.com/jerbe/et-go/proto/locksteppb"
	gproto "google.golang.org/protobuf/proto"
)

func TestGateRoutesRoomMessagesToAuthenticatedRoomActor(t *testing.T) {
	manager := fiber.NewManager(context.Background(), ecs.NewWorld(), nil)
	defer manager.StopAll()

	roomFiber := manager.Create(ecs.SceneTypeRoom, 1, 1, func(f *fiber.Fiber, message fiber.Message) {
		_, _ = actor.DispatchFiberMessage(f.Root(), message)
	})
	if roomFiber == nil {
		t.Fatal("room fiber missing")
	}
	roomMailboxComponent, ok := roomFiber.Root().GetComponent("MailBox")
	if !ok {
		t.Fatal("room mailbox missing")
	}
	roomMailbox, ok := roomMailboxComponent.(*actor.MailBox)
	if !ok {
		t.Fatal("invalid room mailbox")
	}
	received := make(chan struct {
		msgID   uint16
		payload []byte
	}, 3)
	for _, msgID := range []uint16{MsgC2RoomChangeSceneFinish, MsgFrameMessage, MsgC2RoomCheckHash} {
		currentMsgID := msgID
		roomMailbox.RegisterHandler(currentMsgID, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
			received <- struct {
				msgID   uint16
				payload []byte
			}{msgID: currentMsgID, payload: append([]byte(nil), payload...)}
			return nil, nil
		})
	}

	gateScene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	innerSender := actor.NewProcessInnerSender(1, manager, actor.NewRpcManager())
	gateScene.AddComponent(actor.NewMessageSender(1, innerSender, nil))

	player := login.NewPlayer(10, 1001)
	player.AddComponent(&login.PlayerRoomComponent{
		RoomActorID: sceneActorID(roomFiber.Root()),
	})
	session := network.NewSession(context.Background(), 1, nil, nil)
	sessionEntity := ecs.NewEntity()
	sessionEntity.AddComponent(&login.SessionPlayerComponent{Player: player})
	session.SetEntity(sessionEntity)

	roomActorID := sceneActorID(roomFiber.Root())
	handler := &gate.GateMessageHandler{}
	changePayload, err := marshalChangeSceneFinish(&ChangeSceneFinish{RpcId: 11, PlayerId: 9999})
	if err != nil {
		t.Fatalf("marshal change scene payload: %v", err)
	}
	if resp, err := handler.Handle(gateScene, session, &codec.Packet{
		MsgID:   MsgC2RoomChangeSceneFinish,
		Payload: changePayload,
	}); err != nil || resp != nil {
		t.Fatalf("change scene route response=%v err=%v", resp, err)
	}

	framePayload, err := marshalFrameMessageRequest(&FrameMessageRequest{
		RpcId:    12,
		PlayerId: 9999,
		Frame:    1,
		Input:    &LSInput{Button: 3},
	})
	if err != nil {
		t.Fatalf("marshal frame payload: %v", err)
	}
	if _, err := handler.Handle(gateScene, session, &codec.Packet{
		MsgID:   MsgFrameMessage,
		Payload: framePayload,
	}); err != nil {
		t.Fatalf("frame route error: %v", err)
	}

	hashPayload, err := marshalCheckHashRequest(&CheckHashRequest{
		RpcId:    13,
		PlayerId: 9999,
		Frame:    2,
		Hash:     77,
	})
	if err != nil {
		t.Fatalf("marshal hash payload: %v", err)
	}
	if _, err := handler.Handle(gateScene, session, &codec.Packet{
		MsgID:   MsgC2RoomCheckHash,
		Payload: hashPayload,
	}); err != nil {
		t.Fatalf("hash route error: %v", err)
	}

	want := map[uint16]struct {
		playerID int64
		rpcID    uint32
	}{
		MsgC2RoomChangeSceneFinish: {playerID: 1001, rpcID: 11},
		MsgFrameMessage:            {playerID: 1001, rpcID: 12},
		MsgC2RoomCheckHash:         {playerID: 1001, rpcID: 13},
	}
	for len(want) > 0 {
		select {
		case message := <-received:
			expected, ok := want[message.msgID]
			if !ok {
				t.Fatalf("unexpected room message id %d", message.msgID)
			}
			switch message.msgID {
			case MsgC2RoomChangeSceneFinish:
				req, err := unmarshalChangeSceneFinish(message.payload)
				if err != nil {
					t.Fatalf("decode change scene payload: %v", err)
				}
				if req.PlayerId != expected.playerID || req.RpcId != expected.rpcID {
					t.Fatalf("change scene request=%+v", req)
				}
			case MsgFrameMessage:
				req, err := unmarshalFrameMessageRequest(message.payload)
				if err != nil {
					t.Fatalf("decode frame payload: %v", err)
				}
				if req.PlayerId != expected.playerID || req.RpcId != expected.rpcID {
					t.Fatalf("frame request=%+v", req)
				}
			case MsgC2RoomCheckHash:
				req, err := unmarshalCheckHashRequest(message.payload)
				if err != nil {
					t.Fatalf("decode hash payload: %v", err)
				}
				if req.PlayerId != expected.playerID || req.RpcId != expected.rpcID {
					t.Fatalf("hash request=%+v", req)
				}
			}
			delete(want, message.msgID)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for room actor %v", roomActorID)
		}
	}
}

func TestGateRejectsRoomMessageWithoutRoomRoute(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	session := network.NewSession(context.Background(), 2, nil, nil)
	entity := ecs.NewEntity()
	entity.AddComponent(&login.SessionPlayerComponent{Player: login.NewPlayer(10, 1001)})
	session.SetEntity(entity)

	payload, err := marshalChangeSceneFinish(&ChangeSceneFinish{RpcId: 1})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, err = (&gate.GateMessageHandler{}).Handle(scene, session, &codec.Packet{
		MsgID:   MsgC2RoomChangeSceneFinish,
		Payload: payload,
	})
	if err != ErrRoomRouteMissing {
		t.Fatalf("route error = %v, want %v", err, ErrRoomRouteMissing)
	}
}

func TestGateSessionDispatcherBindsRoomAndEmitsExternalNotification(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	session := network.NewSession(context.Background(), 3, serverConn, nil)
	player := login.NewPlayer(10, 1001)
	entity := ecs.NewEntity()
	entity.AddComponent(&gate.GateSessionComponent{Session: session})
	entity.AddComponent(&login.SessionPlayerComponent{Player: player})
	session.SetEntity(entity)
	session.StartWriteLoop()

	message := &Match2GNotifyMatchSuccess{
		PlayerId:    1001,
		MapActorId:  2,
		RoomActorId: 3,
		MapActor:    actor.ActorID{ProcessID: 1, FiberID: 10, InstanceID: 2},
		RoomActor:   actor.ActorID{ProcessID: 1, FiberID: 11, InstanceID: 3},
	}
	payload, err := marshalMatch2GNotifyMatchSuccess(message)
	if err != nil {
		t.Fatalf("marshal match notification: %v", err)
	}
	if _, err := (&gate.GateSessionDispatcher{}).Handle(entity, actor.ActorID{}, MsgMatch2GNotifyMatchSuccess, payload); err != nil {
		t.Fatalf("dispatch match notification: %v", err)
	}

	packet, err := codec.Decode(clientConn)
	if err != nil {
		t.Fatalf("decode client packet: %v", err)
	}
	if packet.MsgID != MsgG2CNotifyMatchSuccess || packet.Type != codec.PacketTypeMessage {
		t.Fatalf("client packet = %+v", packet)
	}
	wire := &locksteppb.G2C_NotifyMatchSuccess{}
	if err := gproto.Unmarshal(packet.Payload, wire); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if wire.GetRoomActor() == nil || wire.GetRoomActor().GetFiberId() != 11 {
		t.Fatalf("room actor payload = %+v", wire.GetRoomActor())
	}
	roomComponent, ok := player.GetComponent("PlayerRoomComponent")
	if !ok {
		t.Fatal("PlayerRoomComponent missing")
	}
	playerRoom, ok := roomComponent.(*login.PlayerRoomComponent)
	if !ok || playerRoom.RoomActorID != message.RoomActor || playerRoom.MapActorID != message.MapActor {
		t.Fatalf("player room = %+v", playerRoom)
	}

	cancelPayload, err := marshalMatch2GCancelMatchSuccess(&Match2GCancelMatchSuccess{
		PlayerId:  1001,
		MapActor:  message.MapActor,
		RoomActor: message.RoomActor,
	})
	if err != nil {
		t.Fatalf("marshal cancel notification: %v", err)
	}
	if _, err := (&gate.GateSessionDispatcher{}).Handle(entity, actor.ActorID{}, MsgMatch2GCancelMatchSuccess, cancelPayload); err != nil {
		t.Fatalf("dispatch cancel notification: %v", err)
	}
	if playerRoom.RoomActorID.IsValid() || playerRoom.MapActorID.IsValid() {
		t.Fatalf("player room binding should be cleared: %+v", playerRoom)
	}
}
