package inventory

// WarehouseComponent 表示玩家仓库。
type WarehouseComponent struct {
	BagComponent
}

// Type 返回组件名称。
func (c *WarehouseComponent) Type() string { return "WarehouseComponent" }

// CollectionName 返回持久化集合名。
func (c *WarehouseComponent) CollectionName() string { return "warehouse" }

// StoreFromBag 将背包物品放入仓库。
func (c *WarehouseComponent) StoreFromBag(bag *BagComponent, itemID int64, count int32) int {
	if bag == nil || count <= 0 {
		return ERR_WarehouseCountInvalid
	}
	item := bag.GetItem(itemID)
	if item == nil {
		return ERR_BagItemNotFound
	}
	if item.Count < count {
		return ERR_BagItemCountNotEnough
	}
	before := c.snapshot()
	if errCode, _ := c.TryAddItem(item.ConfigId, count); errCode != 0 {
		if errCode == ERR_BagFull {
			return ERR_WarehouseFull
		}
		return errCode
	}
	errCode := bag.removeExactItemCount(item, count)
	if errCode != 0 {
		c.MaxCapacity = before.MaxCapacity
		c.resetWithSnapshots(before.Items)
	}
	return errCode
}

// TakeToBag 将仓库物品取回背包。
func (c *WarehouseComponent) TakeToBag(bag *BagComponent, itemID int64, count int32) int {
	if bag == nil || count <= 0 {
		return ERR_WarehouseCountInvalid
	}
	item := c.GetItem(itemID)
	if item == nil {
		return ERR_WarehouseItemNotFound
	}
	if item.Count < count {
		return ERR_BagItemCountNotEnough
	}
	before := c.snapshot()
	if errCode, _ := bag.TryAddItem(item.ConfigId, count); errCode != 0 {
		return errCode
	}
	errCode := c.removeExactItemCount(item, count)
	if errCode != 0 {
		bag.MaxCapacity = before.MaxCapacity
		bag.resetWithSnapshots(before.Items)
	}
	return errCode
}
