package statesync

import (
	"errors"
	"log/slog"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/unit"
)

type locationMessageSender interface {
	SendToPlayer(playerID int64, msgID uint16, payload []byte) error
}

// Broadcast 向单位的所有可见玩家广播。
func Broadcast(u *unit.Unit, msg any) error {
	if u == nil {
		return ErrUnitMissing
	}
	aoiComponent, ok := u.GetComponent("AOIEntity")
	if !ok {
		return ErrUnitMissing
	}
	aoiEntity, ok := aoiComponent.(*aoi.AOIEntity)
	if !ok || aoiEntity == nil {
		return ErrUnitMissing
	}
	var sendErr error
	for _, player := range aoiEntity.BeSeePlayers {
		if player == nil {
			continue
		}
		sendErr = errors.Join(sendErr, sendToPlayer(u.Scene(), player.ID, msg))
	}
	return sendErr
}

// BroadcastIncludeSelf 广播并在玩家单位时包含自身。
func BroadcastIncludeSelf(u *unit.Unit, msg any) error {
	sendErr := Broadcast(u, msg)
	if u == nil || !u.UnitType.IsPlayer() {
		return sendErr
	}
	return errors.Join(sendErr, sendToPlayer(u.Scene(), u.ID(), msg))
}

func sendToPlayer(scene *ecs.Scene, playerID int64, msg any) error {
	if scene == nil {
		slog.Error("statesync send to player skipped: scene missing", "player_id", playerID)
		return ErrSceneMissing
	}
	if playerID <= 0 {
		slog.Error("statesync send to player skipped: invalid player", "player_id", playerID)
		return ErrPlayerIDInvalid
	}
	if msg == nil {
		slog.Error("statesync send to player skipped: message missing", "player_id", playerID)
		return ErrMessageNil
	}
	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		slog.Error("statesync send to player failed: sender component missing", "scene", scene.Name(), "player_id", playerID)
		return ErrMessageSenderMissing
	}

	msgID, payload, err := marshalMessage(msg)
	if err != nil {
		slog.Error("statesync marshal message failed", "scene", scene.Name(), "player_id", playerID, "err", err)
		return err
	}
	if msgID == 0 {
		slog.Error("statesync marshal message returned zero message id", "scene", scene.Name(), "player_id", playerID)
		return ErrMessageIDMissing
	}
	if payload == nil {
		slog.Error("statesync marshal message returned nil payload", "scene", scene.Name(), "player_id", playerID, "msg_id", msgID)
		return ErrMessagePayloadMissing
	}

	if sender, ok := component.(locationMessageSender); ok {
		if err := sender.SendToPlayer(playerID, msgID, payload); err != nil {
			slog.Error("statesync send to player failed", "player_id", playerID, "msg_id", msgID, "err", err)
			return err
		}
		return nil
	}
	locationSender, ok := component.(*actorlocation.MessageLocationSenderComponent)
	if !ok {
		slog.Error("statesync send to player failed: sender component type invalid", "scene", scene.Name(), "player_id", playerID)
		return ErrMessageSenderMissing
	}
	sender := locationSender.Get(int(actorlocation.LocationTypeGateSession))
	if sender == nil {
		slog.Error("statesync send to player failed: gate sender missing", "scene", scene.Name(), "player_id", playerID)
		return ErrMessageSenderMissing
	}
	if err := sender.Send(playerID, msgID, payload); err != nil {
		slog.Error("statesync send location message failed", "player_id", playerID, "msg_id", msgID, "err", err)
		return err
	}
	return nil
}
