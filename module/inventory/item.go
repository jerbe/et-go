package inventory

import (
	"sync/atomic"

	"github.com/jerbe/et-go/engine/ecs"
)

var itemIDGen atomic.Int64

// Item 表示背包或仓库中的物品实体。
type Item struct {
	ecs.Entity
	ConfigId  int32 `bson:"configId"`
	Count     int32 `bson:"count"`
	SlotIndex int32 `bson:"slotIndex"`
	UniqueId  int64 `bson:"uniqueId"`
}

// NewItem 创建物品实体。
func NewItem(configID int32, count int32, slotIndex int32) *Item {
	id := itemIDGen.Add(1)
	item := &Item{
		Entity:    *ecs.NewEntity(),
		ConfigId:  configID,
		Count:     count,
		SlotIndex: slotIndex,
		UniqueId:  id,
	}
	item.SetID(id)
	return item
}

func ensureItemIDAtLeast(id int64) {
	if id <= 0 {
		return
	}
	for {
		current := itemIDGen.Load()
		if current >= id {
			return
		}
		if itemIDGen.CompareAndSwap(current, id) {
			return
		}
	}
}

// IsStackable 返回物品是否支持堆叠。
func (i *Item) IsStackable() bool {
	itemType, ok := ResolveItemType(i.ConfigId)
	if !ok {
		return false
	}
	return IsStackableType(itemType)
}

// GetMaxStackCount 返回物品最大堆叠数量。
func (i *Item) GetMaxStackCount() int32 {
	itemType, ok := ResolveItemType(i.ConfigId)
	if !ok {
		return 0
	}
	return GetMaxStackByType(itemType)
}
