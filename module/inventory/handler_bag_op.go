package inventory

import (
	"errors"

	"github.com/jerbe/et-go/engine/ecs"
)

// HandleBagOperation 执行背包操作。
func HandleBagOperation(scene *ecs.Scene, req *C2MBagOperation) *M2CBagOperation {
	if req == nil {
		return &M2CBagOperation{Error: ERR_InventoryRequestInvalid}
	}
	u := getUnit(scene, req.UnitId)
	if u == nil {
		return &M2CBagOperation{RpcId: req.RpcId, Error: ERR_InventoryUnitMissing}
	}
	component, ok := u.GetComponent("BagComponent")
	if !ok {
		return &M2CBagOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	bag, ok := component.(*BagComponent)
	if !ok {
		return &M2CBagOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}

	var (
		errCode int
		changed []*Item
	)
	switch req.OpType {
	case 1:
		item := bag.GetItem(req.ItemId)
		if item == nil {
			errCode = ERR_BagItemNotFound
			break
		}
		errCode = bag.RemoveItemByConfigId(item.ConfigId, 1)
		if errCode == 0 {
			changed = []*Item{item}
		}
	case 2:
		item := bag.GetItem(req.ItemId)
		errCode = bag.RemoveItem(req.ItemId)
		if errCode == 0 {
			changed = []*Item{item}
		}
	case 3:
		errCode = bag.SwapSlots(int(req.SourceSlot), int(req.TargetSlot))
		if errCode == 0 {
			changed = bag.GetAllItems()
		}
	case 4:
		if err := bag.SortBag(); err != nil {
			if errors.Is(err, ErrItemConfigNotFound) {
				errCode = ERR_ItemConfigNotFound
			} else {
				errCode = ERR_BagOperationInvalid
			}
			break
		}
		changed = bag.GetAllItems()
	default:
		errCode = ERR_BagOperationInvalid
	}
	if errCode == 0 {
		if err := NotifyItemChanges(u, ChangeTypeUpdate, ContainerBag, changed); err != nil {
			return &M2CBagOperation{
				RpcId:   req.RpcId,
				Error:   ERR_InventoryNotifyFailed,
				Message: err.Error(),
			}
		}
	}
	return &M2CBagOperation{RpcId: req.RpcId, Error: int32(errCode)}
}
