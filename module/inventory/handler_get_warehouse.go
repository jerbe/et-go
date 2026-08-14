package inventory

import "github.com/jerbe/et-go/engine/ecs"

// HandleGetWarehouseInfo 返回仓库数据。
func HandleGetWarehouseInfo(scene *ecs.Scene, req *C2MGetWarehouseInfo) *M2CGetWarehouseInfo {
	if req == nil {
		return &M2CGetWarehouseInfo{Error: ERR_InventoryRequestInvalid}
	}
	u := getUnit(scene, req.UnitId)
	if u == nil {
		return &M2CGetWarehouseInfo{RpcId: req.RpcId, Error: ERR_InventoryUnitMissing}
	}
	component, ok := u.GetComponent("WarehouseComponent")
	if !ok {
		return &M2CGetWarehouseInfo{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	warehouse, ok := component.(*WarehouseComponent)
	if !ok || warehouse == nil {
		return &M2CGetWarehouseInfo{RpcId: req.RpcId, Error: ERR_InventoryComponentMissing}
	}
	items := warehouse.GetAllItems()
	resp := &M2CGetWarehouseInfo{
		RpcId:       req.RpcId,
		MaxCapacity: warehouse.MaxCapacity,
		Items:       make([]ItemInfo, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, toItemInfo(item))
	}
	return resp
}
