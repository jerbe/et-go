package statesync

import (
	"sync"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/login"
	"github.com/jerbe/et-go/module/unit"
)

func init() {
	actor.RegisterMailBoxDispatcher(actor.MailBoxTypeOrderedMessage, &unitOrderedDispatcher{})
	gate.RegisterLocationRequestWithResponse(MsgC2MEnterMap, MsgM2CEnterMap)
	gate.RegisterLocationMessage(MsgC2MPathfindingResult)
	gate.RegisterLocationMessage(MsgC2MStop)
}

// UnitMessageHandler 处理投递到玩家单位有序邮箱的扩展消息。
type UnitMessageHandler func(scene *ecs.Scene, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)

var (
	unitMessageHandlerMu sync.RWMutex
	unitMessageHandlers  = make(map[uint16]UnitMessageHandler)
)

// RegisterUnitMessageHandler 注册单位有序消息扩展处理器。
//
// Map、Inventory 等业务包通过该入口接入 statesync 的公共有序邮箱分发器，
// 避免 statesync 反向依赖具体业务包造成 import cycle。
func RegisterUnitMessageHandler(msgID uint16, handler UnitMessageHandler) {
	unitMessageHandlerMu.Lock()
	defer unitMessageHandlerMu.Unlock()
	if handler == nil {
		delete(unitMessageHandlers, msgID)
		return
	}
	unitMessageHandlers[msgID] = handler
}

func unitMessageHandler(msgID uint16) UnitMessageHandler {
	unitMessageHandlerMu.RLock()
	defer unitMessageHandlerMu.RUnlock()
	return unitMessageHandlers[msgID]
}

type unitOrderedDispatcher struct{}

func (d *unitOrderedDispatcher) Handle(entity *ecs.Entity, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	u := resolveUnit(entity)
	if u == nil {
		return nil, actor.ErrActorNotFound
	}
	scene := u.Scene()
	if handler := unitMessageHandler(msgID); handler != nil {
		return handler(scene, actorID, msgID, payload)
	}
	switch msgID {
	case MsgC2MEnterMap:
		req, err := unmarshalEnterMap(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleEnterMap(scene, u, req)
		return marshalEnterMapResponse(&resp)
	case MsgC2MPathfindingResult:
		req, err := unmarshalPathfindingResultReq(payload)
		if err != nil {
			return nil, err
		}
		HandlePathfindingResult(scene, u, req)
		return nil, nil
	case MsgC2MStop:
		req, err := unmarshalStopReq(payload)
		if err != nil {
			return nil, err
		}
		HandleStop(scene, u, req)
		return nil, nil
	case inventory.MsgC2MGetBagInfo,
		inventory.MsgC2MBagOperation,
		inventory.MsgC2MGetWarehouseInfo,
		inventory.MsgC2MWarehouseOp:
		return inventory.HandleOrderedMessage(scene, msgID, payload)
	case login.MsgG2MSessionDisconnect:
		if component, ok := scene.GetComponent("RoomManagerComponent"); ok && component != nil {
			if handler, ok := component.(interface{ HandleUnitDisconnect(unitID int64) }); ok {
				handler.HandleUnitDisconnect(u.ID())
			}
		}
		return nil, nil
	default:
		return nil, actor.ErrHandlerNotFound
	}
}

func resolveUnit(entity *ecs.Entity) *unit.Unit {
	if entity == nil || entity.Scene() == nil {
		return nil
	}
	component, ok := entity.Scene().GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok {
		return nil
	}
	u, ok := unitComponent.Get(entity.ID())
	if !ok {
		return nil
	}
	return u
}
