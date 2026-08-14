package inventory

const (
	// MsgItemInfo 表示物品结构协议编号。
	MsgItemInfo uint16 = 4001
	// MsgC2MGetBagInfo 表示查询背包请求。
	MsgC2MGetBagInfo uint16 = 4002
	// MsgM2CGetBagInfo 表示查询背包响应。
	MsgM2CGetBagInfo uint16 = 4003
	// MsgC2MBagOperation 表示背包操作请求。
	MsgC2MBagOperation uint16 = 4004
	// MsgM2CBagOperation 表示背包操作响应。
	MsgM2CBagOperation uint16 = 4005
	// MsgC2MGetWarehouseInfo 表示查询仓库请求。
	MsgC2MGetWarehouseInfo uint16 = 4006
	// MsgM2CGetWarehouseInfo 表示查询仓库响应。
	MsgM2CGetWarehouseInfo uint16 = 4007
	// MsgC2MWarehouseOp 表示仓库操作请求。
	MsgC2MWarehouseOp uint16 = 4008
	// MsgM2CWarehouseOp 表示仓库操作响应。
	MsgM2CWarehouseOp uint16 = 4009
	// MsgM2CItemChange 表示物品变化通知。
	MsgM2CItemChange uint16 = 4010
)

const (
	ContainerBag       int32 = 1
	ContainerWarehouse int32 = 2
)

const (
	ChangeTypeAdd    int32 = 1
	ChangeTypeUpdate int32 = 2
	ChangeTypeDelete int32 = 3
)

// ItemInfo 表示同步给客户端的物品结构。
type ItemInfo struct {
	ItemId    int64 `json:"item_id"`
	ConfigId  int32 `json:"config_id"`
	Count     int32 `json:"count"`
	SlotIndex int32 `json:"slot_index"`
	UniqueId  int64 `json:"unique_id"`
}

// ItemChangeInfo 表示单个物品变更记录。
type ItemChangeInfo struct {
	ChangeType int32    `json:"change_type"`
	Container  int32    `json:"container"`
	Item       ItemInfo `json:"item"`
}

// C2MGetBagInfo 表示查询背包请求。
type C2MGetBagInfo struct {
	RpcId  uint32 `json:"rpc_id"`
	UnitId int64  `json:"unit_id"`
}

// M2CGetBagInfo 表示背包响应。
type M2CGetBagInfo struct {
	RpcId       uint32     `json:"rpc_id"`
	Error       int32      `json:"error"`
	Message     string     `json:"message,omitempty"`
	MaxCapacity int32      `json:"max_capacity"`
	Items       []ItemInfo `json:"items"`
}

// C2MBagOperation 表示背包操作请求。
type C2MBagOperation struct {
	RpcId      uint32 `json:"rpc_id"`
	UnitId     int64  `json:"unit_id"`
	OpType     int32  `json:"op_type"`
	ItemId     int64  `json:"item_id"`
	ConfigId   int32  `json:"config_id"`
	Count      int32  `json:"count"`
	SourceSlot int32  `json:"source_slot"`
	TargetSlot int32  `json:"target_slot"`
}

// M2CBagOperation 表示背包操作响应。
type M2CBagOperation struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
}

// C2MGetWarehouseInfo 表示查询仓库请求。
type C2MGetWarehouseInfo struct {
	RpcId  uint32 `json:"rpc_id"`
	UnitId int64  `json:"unit_id"`
}

// M2CGetWarehouseInfo 表示仓库响应。
type M2CGetWarehouseInfo struct {
	RpcId       uint32     `json:"rpc_id"`
	Error       int32      `json:"error"`
	Message     string     `json:"message,omitempty"`
	MaxCapacity int32      `json:"max_capacity"`
	Items       []ItemInfo `json:"items"`
}

// C2MWarehouseOperation 表示仓库操作请求。
type C2MWarehouseOperation struct {
	RpcId      uint32 `json:"rpc_id"`
	UnitId     int64  `json:"unit_id"`
	OpType     int32  `json:"op_type"`
	ItemId     int64  `json:"item_id"`
	Count      int32  `json:"count"`
	SourceSlot int32  `json:"source_slot"`
	TargetSlot int32  `json:"target_slot"`
}

// M2CWarehouseOperation 表示仓库操作响应。
type M2CWarehouseOperation struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
}

// M2CItemChange 表示物品变更通知。
type M2CItemChange struct {
	ChangeType int32    `json:"change_type"`
	Container  int32    `json:"container"`
	Item       ItemInfo `json:"item"`
}
