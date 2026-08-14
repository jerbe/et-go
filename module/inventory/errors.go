package inventory

import "errors"

const (
	ERR_BagFull                     = 100030001
	ERR_BagItemNotFound             = 100030002
	ERR_BagItemCountNotEnough       = 100030003
	ERR_BagSlotInvalid              = 100030004
	ERR_BagOperationInvalid         = 100030005
	ERR_SessionPlayerError          = 100030006
	ERR_WarehouseFull               = 100030051
	ERR_WarehouseItemNotFound       = 100030052
	ERR_WarehouseItemCountNotEnough = 100030053
	ERR_WarehouseSlotInvalid        = 100030054
	ERR_WarehouseOperationInvalid   = 100030055
	ERR_ItemConfigNotFound          = 100030101
	ERR_ItemCannotStack             = 100030102
	ERR_ItemStackOverflow           = 100030103

	// 以下是 Go 严格化新增的协议错误，保留在 Inventory 包的扩展段。
	ERR_InventoryRequestInvalid   = 100030201
	ERR_InventoryUnitMissing      = 100030202
	ERR_InventoryComponentMissing = 100030203
	ERR_InventoryNotifyFailed     = 100030204
	ERR_BagCountInvalid           = 100030205
	ERR_WarehouseCountInvalid     = 100030206
)

var (
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("inventory: message is nil")
	// ErrBagClosed 表示 BagComponent 已销毁。
	ErrBagClosed = errors.New("inventory: bag component closed")
	// ErrBagSnapshotInvalid 表示持久化或迁移快照违反背包不变量。
	ErrBagSnapshotInvalid = errors.New("inventory: invalid bag snapshot")
	// ErrItemConfigNotFound 表示背包中存在未注册的物品配置。
	ErrItemConfigNotFound = errors.New("inventory: item config not found")
)
