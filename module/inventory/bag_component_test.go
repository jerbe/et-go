package inventory

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBagComponentOperations(t *testing.T) {
	RegisterItemConfigType(1001, ItemTypeMaterial)
	RegisterItemConfigType(2001, ItemTypeWeapon)

	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 5}
	entity.AddComponent(bag)

	if errCode, items := bag.TryAddItem(1001, 10000); errCode != 0 || len(items) != 2 {
		t.Fatalf("TryAddItem err=%d len=%d", errCode, len(items))
	}
	if bag.GetItemCountByConfigId(1001) != 10000 {
		t.Fatalf("count = %d", bag.GetItemCountByConfigId(1001))
	}

	if errCode, _ := bag.TryAddItem(2001, 1); errCode != 0 {
		t.Fatalf("TryAddItem weapon err = %d", errCode)
	}
	items := bag.GetAllItems()
	if len(items) != 3 {
		t.Fatalf("len(items) = %d", len(items))
	}

	if errCode := bag.SwapSlots(0, 2); errCode != 0 {
		t.Fatalf("SwapSlots err = %d", errCode)
	}
	if err := bag.SortBag(); err != nil {
		t.Fatalf("SortBag error = %v", err)
	}
	items = bag.GetAllItems()
	for index, item := range items {
		if int(item.SlotIndex) != index {
			t.Fatalf("slotIndex = %d, want %d", item.SlotIndex, index)
		}
	}

	if errCode := bag.RemoveItemByConfigId(1001, 1); errCode != 0 {
		t.Fatalf("RemoveItemByConfigId err = %d", errCode)
	}
	if bag.GetItemCountByConfigId(1001) != 9999 {
		t.Fatalf("count = %d", bag.GetItemCountByConfigId(1001))
	}
}

func TestBagComponentSwapOneEmptyAndEnumString(t *testing.T) {
	RegisterItemConfigType(6001, ItemTypeWeapon)
	if ItemTypeWeapon.String() != "Weapon" || ItemQualityRed.String() != "Red" {
		t.Fatal("unexpected string output")
	}
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 3}
	entity.AddComponent(bag)
	if errCode, _ := bag.TryAddItem(6001, 1); errCode != 0 {
		t.Fatalf("TryAddItem err = %d", errCode)
	}
	item := bag.GetAllItems()[0]
	if errCode := bag.SwapSlots(0, 2); errCode != 0 {
		t.Fatalf("SwapSlots err = %d", errCode)
	}
	if bag.GetItem(item.ID()) == nil || bag.GetItemBySlot(2) == nil || bag.GetItemBySlot(0) != nil {
		t.Fatal("one-empty swap should move item to target slot")
	}
}

func TestBagComponentInitMapsFromChildren(t *testing.T) {
	RegisterItemConfigType(3001, ItemTypeConsumable)
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 4}
	entity.AddComponent(bag)
	if errCode, _ := bag.TryAddItem(3001, 10); errCode != 0 {
		t.Fatalf("TryAddItem err = %d", errCode)
	}
	bag.ItemSlotMap = nil
	bag.ConfigIdItemsMap = nil
	bag.InitMapsFromChildren()
	if len(bag.ItemSlotMap) == 0 || len(bag.ConfigIdItemsMap) == 0 {
		t.Fatal("maps should be rebuilt")
	}
}

func TestUnknownItemTypeHasNoStackCapacity(t *testing.T) {
	if got := GetMaxStackByType(ItemType(999)); got != 0 {
		t.Fatalf("unknown item type max stack = %d, want 0", got)
	}
	item := NewItem(999, 1, 0)
	if got := item.GetMaxStackCount(); got != 0 {
		t.Fatalf("unknown item max stack = %d, want 0", got)
	}
}

func TestBagSortRejectsUnknownConfigWithoutMutation(t *testing.T) {
	RegisterItemConfigType(7002, ItemTypeMaterial)
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 2}
	entity.AddComponent(bag)
	if errCode, _ := bag.TryAddItem(7002, 1); errCode != 0 {
		t.Fatalf("TryAddItem err = %d", errCode)
	}
	unknown := NewItem(799999, 1, 1)
	bag.attachItem(unknown)
	before := append([]itemSnapshot(nil), bag.Items...)

	err := bag.SortBag()
	if !errors.Is(err, ErrItemConfigNotFound) {
		t.Fatalf("SortBag error = %v, want %v", err, ErrItemConfigNotFound)
	}
	if len(bag.Items) != len(before) {
		t.Fatalf("items changed after rejected sort: got %v, before %v", bag.Items, before)
	}
	for index := range before {
		if bag.Items[index] != before[index] {
			t.Fatalf("item %d changed after rejected sort: got %+v, before %+v", index, bag.Items[index], before[index])
		}
	}
}

func TestBagRejectsNonPositiveCounts(t *testing.T) {
	RegisterItemConfigType(7001, ItemTypeMaterial)
	bag := &BagComponent{MaxCapacity: 2}
	if errCode, _ := bag.TryAddItem(7001, 0); errCode != ERR_BagCountInvalid {
		t.Fatalf("TryAddItem zero count = %d, want %d", errCode, ERR_BagCountInvalid)
	}
	if errCode := bag.RemoveItemByConfigId(7001, -1); errCode != ERR_BagCountInvalid {
		t.Fatalf("RemoveItemByConfigId negative count = %d, want %d", errCode, ERR_BagCountInvalid)
	}
}

func TestBagDoesNotReopenAfterDestroy(t *testing.T) {
	bag := &BagComponent{}
	bag.Awake()
	bag.OnDestroy()
	bag.Awake()

	if errCode, _ := bag.TryAddItem(7001, 1); errCode != ERR_BagOperationInvalid {
		t.Fatalf("TryAddItem after destroy = %d, want %d", errCode, ERR_BagOperationInvalid)
	}
	if _, err := bag.MarshalBSON(); err != ErrBagClosed {
		t.Fatalf("MarshalBSON after destroy = %v, want %v", err, ErrBagClosed)
	}
}

func TestBagRestorePreservesSlotsAndAdvancesItemID(t *testing.T) {
	RegisterItemConfigType(7101, ItemTypeMaterial)
	entity := ecs.NewEntity()
	bag := &BagComponent{MaxCapacity: 4}
	entity.AddComponent(bag)
	if errCode, _ := bag.TryAddItem(7101, 10); errCode != 0 {
		t.Fatalf("TryAddItem error = %d", errCode)
	}
	item := bag.GetAllItems()[0]
	if errCode := bag.SwapSlots(0, 3); errCode != 0 {
		t.Fatalf("SwapSlots error = %d", errCode)
	}
	raw, err := bag.MarshalBSON()
	if err != nil {
		t.Fatalf("MarshalBSON error = %v", err)
	}

	restored := &BagComponent{}
	if err := restored.UnmarshalBSON(raw); err != nil {
		t.Fatalf("UnmarshalBSON error = %v", err)
	}
	restored.Awake()
	restoredItem := restored.GetItem(item.ID())
	if restoredItem == nil || restoredItem.SlotIndex != 3 {
		t.Fatalf("restored item = %+v, want slot 3", restoredItem)
	}

	next := NewItem(7101, 1, 0)
	if next.ID() <= item.ID() {
		t.Fatalf("next item id = %d, restored id = %d", next.ID(), item.ID())
	}
}

func TestBagRejectsInvalidSnapshot(t *testing.T) {
	RegisterItemConfigType(7102, ItemTypeMaterial)
	raw, err := bson.Marshal(containerSnapshot{
		MaxCapacity: 1,
		Items: []itemSnapshot{{
			ConfigId:  7102,
			Count:     1,
			SlotIndex: 0,
			UniqueId:  99,
		}, {
			ConfigId:  7102,
			Count:     1,
			SlotIndex: 0,
			UniqueId:  100,
		}},
	})
	if err != nil {
		t.Fatalf("marshal invalid snapshot error = %v", err)
	}
	if err := (&BagComponent{}).UnmarshalBSON(raw); err == nil {
		t.Fatal("invalid snapshot should be rejected")
	}
}
