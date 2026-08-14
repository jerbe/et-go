package inventory

import "github.com/jerbe/et-go/engine/ecs"

// HandleWarehouseOperation 执行仓库操作。
func HandleWarehouseOperation(scene *ecs.Scene, req *C2MWarehouseOperation) *M2CWarehouseOperation {
	if req == nil {
		return &M2CWarehouseOperation{Error: ERR_InventoryRequestInvalid}
	}
	if req.Count <= 0 && (req.OpType == 1 || req.OpType == 2) {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_WarehouseCountInvalid}
	}
	u := getUnit(scene, req.UnitId)
	if u == nil {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_InventoryUnitMissing}
	}
	bagComponent, ok := u.GetComponent("BagComponent")
	if !ok {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	warehouseComponent, ok := u.GetComponent("WarehouseComponent")
	if !ok {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	bag, ok := bagComponent.(*BagComponent)
	if !ok || bag == nil {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	warehouse, ok := warehouseComponent.(*WarehouseComponent)
	if !ok || warehouse == nil {
		return &M2CWarehouseOperation{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}

	var errCode int
	switch req.OpType {
	case 1:
		errCode = warehouse.StoreFromBag(bag, req.ItemId, req.Count)
	case 2:
		errCode = warehouse.TakeToBag(bag, req.ItemId, req.Count)
	case 3:
		errCode = warehouse.SwapSlots(int(req.SourceSlot), int(req.TargetSlot))
	default:
		errCode = ERR_WarehouseOperationInvalid
	}
	if errCode == 0 {
		if err := NotifyItemChanges(u, ChangeTypeUpdate, ContainerBag, bag.GetAllItems()); err != nil {
			return &M2CWarehouseOperation{
				RpcId:   req.RpcId,
				Error:   ERR_InventoryNotifyFailed,
				Message: err.Error(),
			}
		}
		if err := NotifyItemChanges(u, ChangeTypeUpdate, ContainerWarehouse, warehouse.GetAllItems()); err != nil {
			return &M2CWarehouseOperation{
				RpcId:   req.RpcId,
				Error:   ERR_InventoryNotifyFailed,
				Message: err.Error(),
			}
		}
	}
	return &M2CWarehouseOperation{RpcId: req.RpcId, Error: int32(errCode)}
}
