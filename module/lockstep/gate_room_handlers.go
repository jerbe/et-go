package lockstep

import (
	"fmt"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/login"
)

func init() {
	gate.RegisterSessionPacketHandler(MsgC2RoomChangeSceneFinish, handleGateChangeSceneFinish)
	gate.RegisterSessionPacketHandler(MsgFrameMessage, handleGateFrameMessage)
	gate.RegisterSessionPacketHandler(MsgC2RoomCheckHash, handleGateCheckHash)
	gate.RegisterGateSessionActorMessageHandler(MsgMatch2GNotifyMatchSuccess, handleGateMatchSuccess)
	gate.RegisterGateSessionActorMessageHandler(MsgMatch2GCancelMatchSuccess, handleGateCancelMatchSuccess)
}

func handleGateChangeSceneFinish(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
	if packet == nil {
		return nil, codec.ErrInvalidPacket
	}
	playerID, roomActorID, err := gateRoomRoute(session)
	if err != nil {
		return nil, err
	}
	req, err := unmarshalChangeSceneFinish(packet.Payload)
	if err != nil {
		return nil, err
	}
	req.PlayerId = playerID
	payload, err := marshalChangeSceneFinish(req)
	if err != nil {
		return nil, err
	}
	if err := sendGateRoomMessage(scene, roomActorID, MsgC2RoomChangeSceneFinish, payload); err != nil {
		return nil, err
	}
	return nil, nil
}

func handleGateFrameMessage(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
	if packet == nil {
		return nil, codec.ErrInvalidPacket
	}
	playerID, roomActorID, err := gateRoomRoute(session)
	if err != nil {
		return nil, err
	}
	req, err := unmarshalFrameMessageRequest(packet.Payload)
	if err != nil {
		return nil, err
	}
	req.PlayerId = playerID
	payload, err := marshalFrameMessageRequest(req)
	if err != nil {
		return nil, err
	}
	if err := sendGateRoomMessage(scene, roomActorID, MsgFrameMessage, payload); err != nil {
		return nil, err
	}
	return nil, nil
}

func handleGateCheckHash(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
	if packet == nil {
		return nil, codec.ErrInvalidPacket
	}
	playerID, roomActorID, err := gateRoomRoute(session)
	if err != nil {
		return nil, err
	}
	req, err := unmarshalCheckHashRequest(packet.Payload)
	if err != nil {
		return nil, err
	}
	req.PlayerId = playerID
	payload, err := marshalCheckHashRequest(req)
	if err != nil {
		return nil, err
	}
	if err := sendGateRoomMessage(scene, roomActorID, MsgC2RoomCheckHash, payload); err != nil {
		return nil, err
	}
	return nil, nil
}

func handleGateMatchSuccess(entity *ecs.Entity, _ uint16, payload []byte) (uint16, []byte, bool, error) {
	if entity == nil {
		return 0, nil, true, gate.ErrGateSessionMissing
	}
	component, ok := entity.GetComponent("SessionPlayerComponent")
	if !ok || component == nil {
		return 0, nil, true, gate.ErrNotLoggedIn
	}
	sessionPlayer, ok := component.(*login.SessionPlayerComponent)
	if !ok || sessionPlayer == nil || sessionPlayer.Player == nil {
		return 0, nil, true, gate.ErrNotLoggedIn
	}
	message, err := unmarshalMatch2GNotifyMatchSuccess(payload)
	if err != nil {
		return 0, nil, true, err
	}
	if message.PlayerId <= 0 || message.PlayerId != sessionPlayer.Player.ID() {
		return 0, nil, true, fmt.Errorf("%w: match notification player mismatch", ErrPlayerInvalid)
	}
	if !message.MapActor.IsValid() || !message.RoomActor.IsValid() {
		return 0, nil, true, ErrRoomRouteMissing
	}

	roomComponent, ok := sessionPlayer.Player.GetComponent("PlayerRoomComponent")
	if !ok || roomComponent == nil {
		roomComponent = &login.PlayerRoomComponent{}
		sessionPlayer.Player.AddComponent(roomComponent)
	}
	playerRoom, ok := roomComponent.(*login.PlayerRoomComponent)
	if !ok || playerRoom == nil {
		return 0, nil, true, ErrRoomRouteMissing
	}
	playerRoom.MapActorID = message.MapActor
	playerRoom.RoomActorID = message.RoomActor

	responsePayload, err := marshalG2CNotifyMatchSuccess(message)
	if err != nil {
		return 0, nil, true, err
	}
	return MsgG2CNotifyMatchSuccess, responsePayload, true, nil
}

func handleGateCancelMatchSuccess(entity *ecs.Entity, _ uint16, payload []byte) (uint16, []byte, bool, error) {
	if entity == nil {
		return 0, nil, true, gate.ErrGateSessionMissing
	}
	component, ok := entity.GetComponent("SessionPlayerComponent")
	if !ok || component == nil {
		return 0, nil, true, gate.ErrNotLoggedIn
	}
	sessionPlayer, ok := component.(*login.SessionPlayerComponent)
	if !ok || sessionPlayer == nil || sessionPlayer.Player == nil {
		return 0, nil, true, gate.ErrNotLoggedIn
	}
	message, err := unmarshalMatch2GCancelMatchSuccess(payload)
	if err != nil {
		return 0, nil, true, err
	}
	if message.PlayerId <= 0 || message.PlayerId != sessionPlayer.Player.ID() ||
		!message.MapActor.IsValid() || !message.RoomActor.IsValid() {
		return 0, nil, true, ErrRoomRouteMissing
	}
	roomComponent, ok := sessionPlayer.Player.GetComponent("PlayerRoomComponent")
	if !ok || roomComponent == nil {
		// 没有成功通知过该玩家时，取消是幂等 no-op。
		return 0, nil, true, nil
	}
	playerRoom, ok := roomComponent.(*login.PlayerRoomComponent)
	if !ok || playerRoom == nil {
		return 0, nil, true, ErrRoomRouteMissing
	}
	if playerRoom.MapActorID == message.MapActor && playerRoom.RoomActorID == message.RoomActor {
		playerRoom.MapActorID = actor.ActorID{}
		playerRoom.RoomActorID = actor.ActorID{}
	}
	return 0, nil, true, nil
}

func gateRoomRoute(session *network.Session) (int64, actor.ActorID, error) {
	if session == nil || session.Entity() == nil {
		return 0, actor.ActorID{}, gate.ErrNotLoggedIn
	}
	component, ok := session.Entity().GetComponent("SessionPlayerComponent")
	if !ok || component == nil {
		return 0, actor.ActorID{}, gate.ErrNotLoggedIn
	}
	sessionPlayer, ok := component.(*login.SessionPlayerComponent)
	if !ok || sessionPlayer == nil || sessionPlayer.Player == nil {
		return 0, actor.ActorID{}, gate.ErrNotLoggedIn
	}
	player := sessionPlayer.Player
	roomComponent, ok := player.GetComponent("PlayerRoomComponent")
	if !ok || roomComponent == nil {
		return 0, actor.ActorID{}, ErrRoomRouteMissing
	}
	playerRoom, ok := roomComponent.(*login.PlayerRoomComponent)
	if !ok || playerRoom == nil || !playerRoom.RoomActorID.IsValid() {
		return 0, actor.ActorID{}, ErrRoomRouteMissing
	}
	if player.ID() <= 0 {
		return 0, actor.ActorID{}, ErrPlayerInvalid
	}
	return player.ID(), playerRoom.RoomActorID, nil
}

func sendGateRoomMessage(scene *ecs.Scene, roomActorID actor.ActorID, msgID uint16, payload []byte) error {
	if scene == nil {
		return gate.ErrSceneMissing
	}
	if !roomActorID.IsValid() {
		return ErrRoomRouteMissing
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return ErrGateMessageSenderMissing
	}
	sender, ok := component.(*actor.MessageSender)
	if !ok || sender == nil {
		return ErrGateMessageSenderMissing
	}
	if err := sender.Send(roomActorID, msgID, payload); err != nil {
		return fmt.Errorf("%w: %v", ErrGateMessageSenderMissing, err)
	}
	return nil
}
