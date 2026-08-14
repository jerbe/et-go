package lockstep

import (
	"errors"
	"log/slog"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/actorlocation"
)

type gateBroadcaster interface {
	SendToGate(playerID int64, msgID uint16, payload []byte) error
}

func broadcastToGate(scene *ecs.Scene, playerIDs []int64, _ uint16, message any) error {
	if scene == nil {
		return ErrRoomSceneMissing
	}
	if len(playerIDs) == 0 {
		return ErrRoomPlayersInvalid
	}
	if message == nil {
		return ErrMessageNil
	}
	for _, playerID := range playerIDs {
		if playerID <= 0 {
			return ErrPlayerInvalid
		}
	}
	msgID, payload, err := marshalByMessage(message)
	if err != nil {
		return err
	}
	if msgID == 0 {
		return ErrMessageNil
	}
	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return ErrRoomMessageSenderMissing
	}
	var sendErr error
	if broadcaster, ok := component.(gateBroadcaster); ok {
		for _, playerID := range playerIDs {
			if err := broadcaster.SendToGate(playerID, msgID, payload); err != nil {
				slog.Error("lockstep broadcast to gate failed", "player_id", playerID, "msg_id", msgID, "err", err)
				sendErr = errors.Join(sendErr, err)
			}
		}
		return sendErr
	}
	var sender roomLocationSender
	if locationSenders, ok := component.(*actorlocation.MessageLocationSenderComponent); ok {
		sender = locationSenders.Get(int(actorlocation.LocationTypeGateSession))
	} else if senderComponent, ok := component.(roomLocationSenderComponent); ok {
		sender = senderComponent.Get(int(actorlocation.LocationTypeGateSession))
	}
	if sender == nil {
		return ErrRoomMessageSenderMissing
	}
	for _, playerID := range playerIDs {
		if err := sender.Send(playerID, msgID, payload); err != nil {
			slog.Error("lockstep broadcast location message failed", "player_id", playerID, "msg_id", msgID, "err", err)
			sendErr = errors.Join(sendErr, err)
		}
	}
	return sendErr
}

func buildUnitInfosFromWorld(world *LSWorld, playerIDs []int64) ([]*LockStepUnitInfo, error) {
	if world == nil {
		return nil, ErrLSWorldMissing
	}
	if err := validateRoomPlayers(playerIDs); err != nil {
		return nil, err
	}
	infos := make([]*LockStepUnitInfo, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		unit, ok := world.Unit(playerID)
		if !ok || unit == nil {
			return nil, ErrLSWorldPlayerMissing
		}
		infos = append(infos, &LockStepUnitInfo{
			PlayerId: playerID,
			Position: unit.Position,
			Rotation: unit.Rotation,
		})
	}
	return infos, nil
}

type roomLocationSender interface {
	Send(int64, uint16, []byte) error
}

type roomLocationSenderComponent interface {
	Get(int) roomLocationSender
}

func broadcastRoomMessage(scene *ecs.Scene, playerIDs []int64, msg any) {
	for _, playerID := range playerIDs {
		if err := sendRoomMessage(scene, playerID, msg); err != nil {
			slog.Error("lockstep room message failed", "player_id", playerID, "err", err)
		}
	}
}

func sendRoomMessage(scene *ecs.Scene, playerID int64, msg any) error {
	if scene == nil {
		return ErrRoomSceneMissing
	}
	if playerID <= 0 {
		return ErrPlayerInvalid
	}
	if msg == nil {
		return ErrMessageNil
	}
	sender := gateSender(scene)
	if sender == nil {
		return ErrRoomMessageSenderMissing
	}
	msgID, payload, err := marshalByMessage(msg)
	if err != nil {
		return err
	}
	if msgID == 0 {
		return ErrMessageNil
	}
	return sender.Send(playerID, msgID, payload)
}

func sendRoomMessageToMap(scene *ecs.Scene, target actor.ActorID, msg any) bool {
	if scene == nil || !target.IsValid() || msg == nil {
		return false
	}
	msgID, payload, err := marshalByMessage(msg)
	if err != nil || msgID == 0 {
		return false
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return false
	}
	sender, ok := component.(*actor.MessageSender)
	if !ok {
		return false
	}
	return sender.Send(target, msgID, payload) == nil
}

func gateSender(scene *ecs.Scene) roomLocationSender {
	if scene == nil {
		return nil
	}
	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return nil
	}
	if senderComponent, ok := component.(*actorlocation.MessageLocationSenderComponent); ok {
		return senderComponent.Get(int(actorlocation.LocationTypeGateSession))
	}
	if senderComponent, ok := component.(roomLocationSenderComponent); ok {
		return senderComponent.Get(int(actorlocation.LocationTypeGateSession))
	}
	return nil
}
