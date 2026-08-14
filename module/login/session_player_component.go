package login

import (
	"log/slog"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/actorlocation"
)

// SessionPlayerComponent 保存 Session 到玩家的引用。
type SessionPlayerComponent struct {
	ecs.BaseComponent
	Player *Player
}

// Type 返回组件名称。
func (c *SessionPlayerComponent) Type() string { return "SessionPlayerComponent" }

// GetPlayerID 返回玩家 ID。
func (c *SessionPlayerComponent) GetPlayerID() int64 {
	if c == nil || c.Player == nil {
		return 0
	}
	return c.Player.ID()
}

// GetUnitID 返回玩家 Unit ID。
func (c *SessionPlayerComponent) GetUnitID() int64 {
	if c == nil || c.Player == nil {
		return 0
	}
	return c.Player.UnitId
}

// OnDestroy 清理玩家引用与定位信息。
func (c *SessionPlayerComponent) OnDestroy() {
	if c == nil || c.Player == nil {
		return
	}

	player := c.Player
	c.Player = nil

	entity := c.GetEntity()
	if entity == nil {
		return
	}
	scene := entity.Scene()
	if scene == nil || scene.IsDisposed() {
		if component, ok := player.GetComponent("PlayerSessionComponent"); ok {
			if sessionComponent, ok := component.(*PlayerSessionComponent); ok {
				sessionComponent.Session = nil
			}
		}
		return
	}

	if component, ok := scene.GetComponent("MessageLocationSenderComponent"); ok {
		if senderComponent, ok := component.(*actorlocation.MessageLocationSenderComponent); ok && player.UnitId > 0 {
			payload, err := marshalG2MSessionDisconnect(&G2MSessionDisconnect{})
			if err != nil {
				slog.Error("marshal session disconnect failed", "unit_id", player.UnitId, "err", err)
			} else {
				sender := senderComponent.Get(int(actorlocation.LocationTypeUnit))
				if sender != nil {
					if err := sender.Send(player.UnitId, MsgG2MSessionDisconnect, payload); err != nil {
						slog.Error("send session disconnect failed", "unit_id", player.UnitId, "err", err)
					}
				}
			}
		}
	}
	if component, ok := player.GetComponent("PlayerSessionComponent"); ok {
		if sessionComponent, ok := component.(*PlayerSessionComponent); ok {
			sessionComponent.Session = nil
		}
	}
}
