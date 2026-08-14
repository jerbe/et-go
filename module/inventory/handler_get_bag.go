package inventory

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/unit"
)

func init() {
	gate.RegisterLocationRequestWithResponse(MsgC2MGetBagInfo, MsgM2CGetBagInfo)
	gate.RegisterLocationRequestWithResponse(MsgC2MBagOperation, MsgM2CBagOperation)
	gate.RegisterLocationRequestWithResponse(MsgC2MGetWarehouseInfo, MsgM2CGetWarehouseInfo)
	gate.RegisterLocationRequestWithResponse(MsgC2MWarehouseOp, MsgM2CWarehouseOp)
}

// RegisterMapHandlers 注册背包与仓库相关的地图处理器。
func RegisterMapHandlers(scene *ecs.Scene, mailbox *actor.MailBox) {
	if scene == nil || mailbox == nil {
		return
	}
	mailbox.RegisterHandler(MsgC2MGetBagInfo, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalGetBagInfoReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalGetBagInfoResp(HandleGetBagInfo(scene, req))
	})
	mailbox.RegisterHandler(MsgC2MBagOperation, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalBagOperationReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalBagOperationResp(HandleBagOperation(scene, req))
	})
	mailbox.RegisterHandler(MsgC2MGetWarehouseInfo, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalGetWarehouseInfoReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalGetWarehouseInfoResp(HandleGetWarehouseInfo(scene, req))
	})
	mailbox.RegisterHandler(MsgC2MWarehouseOp, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalWarehouseOperationReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalWarehouseOperationResp(HandleWarehouseOperation(scene, req))
	})
}

// HandleOrderedMessage 处理玩家单位有序邮箱上的背包/仓库消息。
//
// 客户端消息经 Gate 按玩家单位位置投递时，目标邮箱类型为
// MailBoxTypeOrderedMessage，因此不能只注册到 Map 根实体邮箱。
func HandleOrderedMessage(scene *ecs.Scene, msgID uint16, payload []byte) ([]byte, error) {
	switch msgID {
	case MsgC2MGetBagInfo:
		req, err := unmarshalGetBagInfoReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalGetBagInfoResp(HandleGetBagInfo(scene, req))
	case MsgC2MBagOperation:
		req, err := unmarshalBagOperationReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalBagOperationResp(HandleBagOperation(scene, req))
	case MsgC2MGetWarehouseInfo:
		req, err := unmarshalGetWarehouseInfoReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalGetWarehouseInfoResp(HandleGetWarehouseInfo(scene, req))
	case MsgC2MWarehouseOp:
		req, err := unmarshalWarehouseOperationReq(payload)
		if err != nil {
			return nil, err
		}
		return marshalWarehouseOperationResp(HandleWarehouseOperation(scene, req))
	default:
		return nil, actor.ErrHandlerNotFound
	}
}

// HandleGetBagInfo 返回背包数据。
func HandleGetBagInfo(scene *ecs.Scene, req *C2MGetBagInfo) *M2CGetBagInfo {
	if req == nil {
		return &M2CGetBagInfo{Error: ERR_InventoryRequestInvalid}
	}
	u := getUnit(scene, req.UnitId)
	if u == nil {
		return &M2CGetBagInfo{RpcId: req.RpcId, Error: ERR_InventoryUnitMissing}
	}
	component, ok := u.GetComponent("BagComponent")
	if !ok {
		return &M2CGetBagInfo{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	bag, ok := component.(*BagComponent)
	if !ok || bag == nil {
		return &M2CGetBagInfo{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	items := bag.GetAllItems()
	resp := &M2CGetBagInfo{
		RpcId:       req.RpcId,
		MaxCapacity: bag.MaxCapacity,
		Items:       make([]ItemInfo, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, toItemInfo(item))
	}
	return resp
}

func getUnit(scene *ecs.Scene, unitID int64) *unit.Unit {
	if scene == nil {
		return nil
	}
	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok {
		return nil
	}
	u, ok := unitComponent.Get(unitID)
	if !ok {
		return nil
	}
	return u
}
