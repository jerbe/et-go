package inventory

import (
	"fmt"
	"sort"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

type itemSnapshot struct {
	ConfigId  int32 `bson:"configId"`
	Count     int32 `bson:"count"`
	SlotIndex int32 `bson:"slotIndex"`
	UniqueId  int64 `bson:"uniqueId"`
}

type containerSnapshot struct {
	MaxCapacity int32          `bson:"maxCapacity"`
	Slots       map[int]int64  `bson:"slots"`
	Items       []itemSnapshot `bson:"items"`
}

// BagComponent 表示玩家背包。
type BagComponent struct {
	ecs.BaseComponent

	MaxCapacity      int32             `bson:"maxCapacity"`
	Slots            map[int]int64     `bson:"slots"`
	Items            []itemSnapshot    `bson:"items"`
	ItemSlotMap      map[int64]int     `bson:"-"`
	ConfigIdItemsMap map[int32][]int64 `bson:"-"`

	items   map[int64]*Item
	pending *containerSnapshot
	closed  bool
}

// Type 返回组件名称。
func (c *BagComponent) Type() string { return "BagComponent" }

// CollectionName 返回持久化集合名。
func (c *BagComponent) CollectionName() string { return "bag" }

// GetID 返回所属实体 ID。
func (c *BagComponent) GetID() int64 {
	if c == nil || c.GetEntity() == nil {
		return 0
	}
	return c.GetEntity().ID()
}

// Awake 初始化运行时索引。
func (c *BagComponent) Awake() {
	if c == nil || c.closed {
		return
	}
	if c.MaxCapacity <= 0 {
		c.MaxCapacity = 20
	}
	if c.Slots == nil {
		c.Slots = make(map[int]int64)
	}
	if c.ItemSlotMap == nil {
		c.ItemSlotMap = make(map[int64]int)
	}
	if c.ConfigIdItemsMap == nil {
		c.ConfigIdItemsMap = make(map[int32][]int64)
	}
	if c.items == nil {
		c.items = make(map[int64]*Item)
	}
	if c.pending != nil {
		c.applySnapshot(c.pending)
		c.pending = nil
	}
	if len(c.items) == 0 && len(c.Items) > 0 {
		c.resetWithSnapshots(c.Items)
		return
	}
	c.rebuildIndexes()
}

// OnDestroy 清理内部索引。
func (c *BagComponent) OnDestroy() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	for _, item := range c.items {
		if item != nil && !item.IsDisposed() {
			item.Dispose()
		}
	}
	c.Slots = nil
	c.ItemSlotMap = nil
	c.ConfigIdItemsMap = nil
	c.items = nil
	c.pending = nil
}

// MarshalBSON 自定义持久化格式。
func (c *BagComponent) MarshalBSON() ([]byte, error) {
	if c == nil || c.closed {
		return nil, ErrBagClosed
	}
	return bson.Marshal(c.snapshot())
}

// UnmarshalBSON 恢复持久化格式。
func (c *BagComponent) UnmarshalBSON(data []byte) error {
	if c == nil || c.closed {
		return ErrBagClosed
	}
	var snapshot containerSnapshot
	if err := bson.Unmarshal(data, &snapshot); err != nil {
		return err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return err
	}
	c.MaxCapacity = snapshot.MaxCapacity
	c.Slots = make(map[int]int64)
	c.Items = snapshot.Items
	c.pending = &snapshot
	return nil
}

// Transfer 序列化跨地图迁移数据。
func (c *BagComponent) Transfer() ([]byte, error) {
	if c == nil || c.closed {
		return nil, ErrBagClosed
	}
	return bson.Marshal(c.snapshot())
}

// OnTransferIn 恢复跨地图迁移数据。
func (c *BagComponent) OnTransferIn(data []byte) error {
	if c == nil || c.closed {
		return ErrBagClosed
	}
	return c.UnmarshalBSON(data)
}

// GetEmptySlotCount 返回剩余空格数量。
func (c *BagComponent) GetEmptySlotCount() int {
	if c == nil || c.closed {
		return 0
	}
	return int(c.MaxCapacity) - len(c.Slots)
}

// IsFull 返回背包是否已满。
func (c *BagComponent) IsFull() bool {
	if c == nil || c.closed {
		return true
	}
	return len(c.Slots) >= int(c.MaxCapacity)
}

// GetFirstEmptySlot 返回第一个空格子。
func (c *BagComponent) GetFirstEmptySlot() int {
	if c == nil || c.closed {
		return -1
	}
	for index := 0; index < int(c.MaxCapacity); index++ {
		if _, ok := c.Slots[index]; !ok {
			return index
		}
	}
	return -1
}

// GetItem 按物品 ID 查询物品。
func (c *BagComponent) GetItem(itemID int64) *Item {
	if c == nil || c.closed {
		return nil
	}
	c.Awake()
	return c.items[itemID]
}

// GetItemBySlot 按格子查询物品。
func (c *BagComponent) GetItemBySlot(slotIndex int) *Item {
	if c == nil || c.closed {
		return nil
	}
	c.Awake()
	itemID, ok := c.Slots[slotIndex]
	if !ok {
		return nil
	}
	return c.items[itemID]
}

// GetItemsByConfigId 查询同配置的所有物品。
func (c *BagComponent) GetItemsByConfigId(configID int32) []*Item {
	if c == nil || c.closed {
		return nil
	}
	c.Awake()
	ids := c.ConfigIdItemsMap[configID]
	result := make([]*Item, 0, len(ids))
	for _, id := range ids {
		if item := c.items[id]; item != nil {
			result = append(result, item)
		}
	}
	return result
}

// GetItemCountByConfigId 返回指定配置的总数量。
func (c *BagComponent) GetItemCountByConfigId(configID int32) int32 {
	var total int32
	for _, item := range c.GetItemsByConfigId(configID) {
		total += item.Count
	}
	return total
}

// GetAllItems 返回所有物品。
func (c *BagComponent) GetAllItems() []*Item {
	if c == nil || c.closed {
		return nil
	}
	c.Awake()
	result := make([]*Item, 0, len(c.items))
	for _, item := range c.items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SlotIndex == result[j].SlotIndex {
			return result[i].UniqueId < result[j].UniqueId
		}
		return result[i].SlotIndex < result[j].SlotIndex
	})
	return result
}

// TryAddItem 尝试向背包添加物品。
func (c *BagComponent) TryAddItem(configID int32, count int32) (int, []*Item) {
	if c == nil || c.closed {
		return ERR_BagOperationInvalid, nil
	}
	c.Awake()
	itemType, ok := ResolveItemType(configID)
	if !ok {
		return ERR_ItemConfigNotFound, nil
	}
	if count <= 0 {
		return ERR_BagCountInvalid, nil
	}
	maxStack := GetMaxStackByType(itemType)
	if !c.canAdd(configID, count, maxStack) {
		return ERR_BagFull, nil
	}

	changed := make([]*Item, 0)
	remaining := count
	if IsStackableType(itemType) {
		for _, item := range c.GetItemsByConfigId(configID) {
			if remaining <= 0 {
				break
			}
			if item.Count >= maxStack {
				continue
			}
			addCount := minInt32(remaining, maxStack-item.Count)
			item.Count += addCount
			remaining -= addCount
			changed = append(changed, item)
		}
	}

	for remaining > 0 {
		slot := c.GetFirstEmptySlot()
		if slot < 0 {
			return ERR_BagFull, nil
		}
		addCount := remaining
		if addCount > maxStack {
			addCount = maxStack
		}
		item := NewItem(configID, addCount, int32(slot))
		c.attachItem(item)
		remaining -= addCount
		changed = append(changed, item)
	}
	return 0, changed
}

// RemoveItem 删除指定物品。
func (c *BagComponent) RemoveItem(itemID int64) int {
	if c == nil || c.closed {
		return ERR_BagOperationInvalid
	}
	item := c.GetItem(itemID)
	if item == nil {
		return ERR_BagItemNotFound
	}
	c.detachItem(item)
	return 0
}

// RemoveItemByConfigId 按配置扣减数量。
func (c *BagComponent) RemoveItemByConfigId(configID int32, count int32) int {
	if c == nil || c.closed {
		return ERR_BagOperationInvalid
	}
	if count <= 0 {
		return ERR_BagCountInvalid
	}
	if c.GetItemCountByConfigId(configID) < count {
		return ERR_BagItemCountNotEnough
	}
	items := c.GetItemsByConfigId(configID)
	sort.Slice(items, func(i, j int) bool {
		return items[i].SlotIndex > items[j].SlotIndex
	})
	remaining := count
	for _, item := range items {
		if remaining <= 0 {
			break
		}
		if item.Count > remaining {
			item.Count -= remaining
			remaining = 0
			continue
		}
		remaining -= item.Count
		c.detachItem(item)
	}
	return 0
}

// SwapSlots 交换两个格子的物品。
func (c *BagComponent) SwapSlots(slotA, slotB int) int {
	if c == nil || c.closed {
		return ERR_BagOperationInvalid
	}
	c.Awake()
	if slotA < 0 || slotB < 0 || slotA >= int(c.MaxCapacity) || slotB >= int(c.MaxCapacity) {
		return ERR_BagSlotInvalid
	}
	if slotA == slotB {
		return 0
	}
	itemA := c.GetItemBySlot(slotA)
	itemB := c.GetItemBySlot(slotB)
	delete(c.Slots, slotA)
	delete(c.Slots, slotB)
	if itemA != nil {
		itemA.SlotIndex = int32(slotB)
		c.Slots[slotB] = itemA.ID()
		c.ItemSlotMap[itemA.ID()] = slotB
	}
	if itemB != nil {
		itemB.SlotIndex = int32(slotA)
		c.Slots[slotA] = itemB.ID()
		c.ItemSlotMap[itemB.ID()] = slotA
	}
	c.syncSnapshotItems()
	return 0
}

// SortBag 对背包进行整理。
//
// 整理前会验证所有物品配置。任何一个配置无法解析时都保持原状态并返回
// 错误，不能把未知物品静默当成不可堆叠物品。
func (c *BagComponent) SortBag() error {
	if c == nil || c.closed {
		return ErrBagClosed
	}
	c.Awake()
	items := c.GetAllItems()
	type grouped struct {
		configID int32
		records  []itemSnapshot
	}
	recordsByConfig := make(map[int32][]itemSnapshot)
	for _, item := range items {
		recordsByConfig[item.ConfigId] = append(recordsByConfig[item.ConfigId], itemSnapshot{
			ConfigId:  item.ConfigId,
			Count:     item.Count,
			UniqueId:  item.UniqueId,
			SlotIndex: item.SlotIndex,
		})
	}
	configIDs := make([]int32, 0, len(recordsByConfig))
	for configID := range recordsByConfig {
		configIDs = append(configIDs, configID)
	}
	sort.Slice(configIDs, func(i, j int) bool { return configIDs[i] < configIDs[j] })

	snapshots := make([]itemSnapshot, 0)
	for _, configID := range configIDs {
		itemType, ok := ResolveItemType(configID)
		if !ok {
			return fmt.Errorf("%w: config id=%d", ErrItemConfigNotFound, configID)
		}
		if IsStackableType(itemType) {
			var total int32
			for _, record := range recordsByConfig[configID] {
				total += record.Count
			}
			maxStack := GetMaxStackByType(itemType)
			for total > 0 {
				count := minInt32(total, maxStack)
				snapshots = append(snapshots, itemSnapshot{
					ConfigId: configID,
					Count:    count,
				})
				total -= count
			}
			continue
		}
		snapshots = append(snapshots, recordsByConfig[configID]...)
	}
	for index := range snapshots {
		snapshots[index].SlotIndex = int32(index)
	}
	c.resetWithSnapshots(snapshots)
	return nil
}

// InitMapsFromChildren 从当前子物品重建运行时索引。
func (c *BagComponent) InitMapsFromChildren() {
	if c == nil || c.closed {
		return
	}
	if c.ItemSlotMap == nil {
		c.ItemSlotMap = make(map[int64]int)
	}
	if c.ConfigIdItemsMap == nil {
		c.ConfigIdItemsMap = make(map[int32][]int64)
	}
	if c.items == nil {
		c.items = make(map[int64]*Item)
	}
	if len(c.items) == 0 && len(c.Items) > 0 {
		c.resetWithSnapshots(c.Items)
		return
	}
	c.rebuildIndexes()
}

func (c *BagComponent) attachItem(item *Item) {
	if item == nil {
		return
	}
	entity := c.GetEntity()
	if entity != nil {
		entity.AddChildWithID(item.ID(), &item.Entity)
	}
	c.items[item.ID()] = item
	c.Slots[int(item.SlotIndex)] = item.ID()
	c.ItemSlotMap[item.ID()] = int(item.SlotIndex)
	c.ConfigIdItemsMap[item.ConfigId] = append(c.ConfigIdItemsMap[item.ConfigId], item.ID())
	c.syncSnapshotItems()
}

func (c *BagComponent) detachItem(item *Item) {
	if item == nil {
		return
	}
	delete(c.Slots, int(item.SlotIndex))
	delete(c.ItemSlotMap, item.ID())
	delete(c.items, item.ID())
	ids := c.ConfigIdItemsMap[item.ConfigId]
	next := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != item.ID() {
			next = append(next, id)
		}
	}
	if len(next) == 0 {
		delete(c.ConfigIdItemsMap, item.ConfigId)
	} else {
		c.ConfigIdItemsMap[item.ConfigId] = next
	}
	if !item.IsDisposed() {
		item.Dispose()
	}
	c.syncSnapshotItems()
}

func (c *BagComponent) removeExactItemCount(item *Item, count int32) int {
	if item == nil {
		return ERR_BagItemNotFound
	}
	if count <= 0 {
		return ERR_BagCountInvalid
	}
	if item.Count < count {
		return ERR_BagItemCountNotEnough
	}
	if item.Count == count {
		return c.RemoveItem(item.ID())
	}
	item.Count -= count
	c.syncSnapshotItems()
	return 0
}

func (c *BagComponent) resetWithSnapshots(records []itemSnapshot) {
	for _, item := range c.items {
		if item != nil && !item.IsDisposed() {
			item.Dispose()
		}
	}
	c.Slots = make(map[int]int64)
	c.ItemSlotMap = make(map[int64]int)
	c.ConfigIdItemsMap = make(map[int32][]int64)
	c.items = make(map[int64]*Item)
	c.Items = append([]itemSnapshot(nil), records...)
	for _, record := range records {
		item := NewItem(record.ConfigId, record.Count, record.SlotIndex)
		if record.UniqueId > 0 {
			item.UniqueId = record.UniqueId
			item.SetID(record.UniqueId)
			ensureItemIDAtLeast(record.UniqueId)
		}
		c.attachItem(item)
	}
	c.syncSnapshotItems()
}

func (c *BagComponent) canAdd(configID int32, count int32, maxStack int32) bool {
	available := int32(0)
	for _, item := range c.GetItemsByConfigId(configID) {
		if item.Count < maxStack {
			available += maxStack - item.Count
		}
	}
	available += int32(c.GetEmptySlotCount()) * maxStack
	return available >= count
}

func (c *BagComponent) snapshot() containerSnapshot {
	items := c.GetAllItems()
	itemSnapshots := make([]itemSnapshot, 0, len(items))
	for _, item := range items {
		itemSnapshots = append(itemSnapshots, itemSnapshot{
			ConfigId:  item.ConfigId,
			Count:     item.Count,
			SlotIndex: item.SlotIndex,
			UniqueId:  item.UniqueId,
		})
	}
	return containerSnapshot{
		MaxCapacity: c.MaxCapacity,
		Slots:       c.Slots,
		Items:       itemSnapshots,
	}
}

func (c *BagComponent) applySnapshot(snapshot *containerSnapshot) {
	if snapshot == nil {
		return
	}
	c.MaxCapacity = snapshot.MaxCapacity
	c.Slots = make(map[int]int64)
	c.Items = append([]itemSnapshot(nil), snapshot.Items...)
	if len(snapshot.Items) > 0 {
		c.resetWithSnapshots(snapshot.Items)
	}
}

func validateSnapshot(snapshot containerSnapshot) error {
	if snapshot.MaxCapacity <= 0 {
		return fmt.Errorf("%w: max capacity=%d", ErrBagSnapshotInvalid, snapshot.MaxCapacity)
	}
	if len(snapshot.Items) > int(snapshot.MaxCapacity) {
		return fmt.Errorf("%w: item count=%d capacity=%d", ErrBagSnapshotInvalid, len(snapshot.Items), snapshot.MaxCapacity)
	}

	itemsByID := make(map[int64]itemSnapshot, len(snapshot.Items))
	slots := make(map[int]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ConfigId <= 0 || item.Count <= 0 || item.UniqueId <= 0 {
			return fmt.Errorf("%w: invalid item %+v", ErrBagSnapshotInvalid, item)
		}
		if item.SlotIndex < 0 || item.SlotIndex >= snapshot.MaxCapacity {
			return fmt.Errorf("%w: slot=%d capacity=%d", ErrBagSnapshotInvalid, item.SlotIndex, snapshot.MaxCapacity)
		}
		if _, exists := itemsByID[item.UniqueId]; exists {
			return fmt.Errorf("%w: duplicate item id=%d", ErrBagSnapshotInvalid, item.UniqueId)
		}
		if _, exists := slots[int(item.SlotIndex)]; exists {
			return fmt.Errorf("%w: duplicate slot=%d", ErrBagSnapshotInvalid, item.SlotIndex)
		}
		itemType, ok := ResolveItemType(item.ConfigId)
		if !ok {
			return fmt.Errorf("%w: unknown config id=%d", ErrBagSnapshotInvalid, item.ConfigId)
		}
		maxStack := GetMaxStackByType(itemType)
		if maxStack <= 0 || item.Count > maxStack {
			return fmt.Errorf("%w: item id=%d count=%d max=%d", ErrBagSnapshotInvalid, item.UniqueId, item.Count, maxStack)
		}
		itemsByID[item.UniqueId] = item
		slots[int(item.SlotIndex)] = struct{}{}
	}

	if len(snapshot.Slots) != 0 && len(snapshot.Slots) != len(snapshot.Items) {
		return fmt.Errorf("%w: slots=%d items=%d", ErrBagSnapshotInvalid, len(snapshot.Slots), len(snapshot.Items))
	}
	for slot, itemID := range snapshot.Slots {
		if slot < 0 || slot >= int(snapshot.MaxCapacity) {
			return fmt.Errorf("%w: invalid slot map slot=%d", ErrBagSnapshotInvalid, slot)
		}
		item, ok := itemsByID[itemID]
		if !ok || int(item.SlotIndex) != slot {
			return fmt.Errorf("%w: slot map mismatch slot=%d item=%d", ErrBagSnapshotInvalid, slot, itemID)
		}
	}
	return nil
}

func (c *BagComponent) rebuildIndexes() {
	c.ItemSlotMap = make(map[int64]int)
	c.ConfigIdItemsMap = make(map[int32][]int64)
	for id, item := range c.items {
		if item == nil {
			continue
		}
		c.ItemSlotMap[id] = int(item.SlotIndex)
		c.ConfigIdItemsMap[item.ConfigId] = append(c.ConfigIdItemsMap[item.ConfigId], id)
	}
}

func (c *BagComponent) syncSnapshotItems() {
	items := make([]*Item, 0, len(c.items))
	for _, item := range c.items {
		if item != nil {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].SlotIndex == items[j].SlotIndex {
			return items[i].UniqueId < items[j].UniqueId
		}
		return items[i].SlotIndex < items[j].SlotIndex
	})
	c.Items = c.Items[:0]
	for _, item := range items {
		c.Items = append(c.Items, itemSnapshot{
			ConfigId:  item.ConfigId,
			Count:     item.Count,
			SlotIndex: item.SlotIndex,
			UniqueId:  item.UniqueId,
		})
	}
}

func minInt32(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

var (
	_ ecs.TransferSystem     = (*BagComponent)(nil)
	_ db.IDBEntityCollection = (*BagComponent)(nil)
)
