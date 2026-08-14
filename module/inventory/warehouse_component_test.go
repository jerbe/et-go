package inventory

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestWarehouseTransfer(t *testing.T) {
	RegisterItemConfigType(4001, ItemTypeMaterial)
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 2}
	warehouse := &WarehouseComponent{}
	entity.AddComponent(bag)
	entity.AddComponent(warehouse)

	if errCode, _ := bag.TryAddItem(4001, 100); errCode != 0 {
		t.Fatalf("TryAddItem err = %d", errCode)
	}
	item := bag.GetAllItems()[0]
	if errCode := warehouse.StoreFromBag(bag, item.ID(), 50); errCode != 0 {
		t.Fatalf("StoreFromBag err = %d", errCode)
	}
	if bag.GetItemCountByConfigId(4001) != 50 || warehouse.GetItemCountByConfigId(4001) != 50 {
		t.Fatalf("bag=%d warehouse=%d", bag.GetItemCountByConfigId(4001), warehouse.GetItemCountByConfigId(4001))
	}
	warehouseItem := warehouse.GetAllItems()[0]
	if errCode := warehouse.TakeToBag(bag, warehouseItem.ID(), 25); errCode != 0 {
		t.Fatalf("TakeToBag err = %d", errCode)
	}
	if bag.GetItemCountByConfigId(4001) != 75 || warehouse.GetItemCountByConfigId(4001) != 25 {
		t.Fatalf("bag=%d warehouse=%d", bag.GetItemCountByConfigId(4001), warehouse.GetItemCountByConfigId(4001))
	}
}

func TestWarehouseTransferRollbackByItemID(t *testing.T) {
	RegisterItemConfigType(4002, ItemTypeWeapon)
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 1}
	warehouse := &WarehouseComponent{BagComponent: BagComponent{MaxCapacity: 1}}
	entity.AddComponent(bag)
	entity.AddComponent(warehouse)
	if errCode, _ := bag.TryAddItem(4002, 1); errCode != 0 {
		t.Fatalf("TryAddItem err = %d", errCode)
	}
	if errCode, _ := warehouse.TryAddItem(4002, 1); errCode != 0 {
		t.Fatalf("warehouse TryAddItem err = %d", errCode)
	}
	item := bag.GetAllItems()[0]
	if errCode := warehouse.StoreFromBag(bag, item.ID(), 1); errCode != ERR_WarehouseFull {
		t.Fatalf("StoreFromBag err = %d, want warehouse full", errCode)
	}
	if bag.GetItem(item.ID()) == nil {
		t.Fatal("source item should remain after rollback")
	}
}
