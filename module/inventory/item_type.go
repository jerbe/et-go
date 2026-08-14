package inventory

import "sync"

// ItemType 表示物品类型。
type ItemType int32

const (
	// ItemTypeMaterial 表示材料。
	ItemTypeMaterial ItemType = 1
	// ItemTypeConsumable 表示消耗品。
	ItemTypeConsumable ItemType = 2
	// ItemTypeWeapon 表示武器。
	ItemTypeWeapon ItemType = 10
	// ItemTypeArmor 表示防具。
	ItemTypeArmor ItemType = 11
	// ItemTypeAccessory 表示饰品。
	ItemTypeAccessory ItemType = 12
	// ItemTypePet 表示宠物。
	ItemTypePet ItemType = 20
)

// ItemQuality 表示物品品质。
type ItemQuality int32

const (
	ItemQualityWhite  ItemQuality = 0
	ItemQualityGreen  ItemQuality = 1
	ItemQualityBlue   ItemQuality = 2
	ItemQualityPurple ItemQuality = 3
	ItemQualityOrange ItemQuality = 4
	ItemQualityRed    ItemQuality = 5
)

// String 返回物品类型名称。
func (t ItemType) String() string {
	switch t {
	case ItemTypeMaterial:
		return "Material"
	case ItemTypeConsumable:
		return "Consumable"
	case ItemTypeWeapon:
		return "Weapon"
	case ItemTypeArmor:
		return "Armor"
	case ItemTypeAccessory:
		return "Accessory"
	case ItemTypePet:
		return "Pet"
	default:
		return "Unknown"
	}
}

// String 返回物品品质名称。
func (q ItemQuality) String() string {
	switch q {
	case ItemQualityWhite:
		return "White"
	case ItemQualityGreen:
		return "Green"
	case ItemQualityBlue:
		return "Blue"
	case ItemQualityPurple:
		return "Purple"
	case ItemQualityOrange:
		return "Orange"
	case ItemQualityRed:
		return "Red"
	default:
		return "Unknown"
	}
}

var (
	itemConfigMu    sync.RWMutex
	itemConfigTypes = make(map[int32]ItemType)
)

// RegisterItemConfigType 注册配置 ID 对应的物品类型。
func RegisterItemConfigType(configID int32, itemType ItemType) {
	itemConfigMu.Lock()
	itemConfigTypes[configID] = itemType
	itemConfigMu.Unlock()
}

// ResolveItemType 查询配置 ID 对应的物品类型。
func ResolveItemType(configID int32) (ItemType, bool) {
	itemConfigMu.RLock()
	defer itemConfigMu.RUnlock()
	itemType, ok := itemConfigTypes[configID]
	return itemType, ok
}

// IsStackableType 判断物品类型是否支持堆叠。
func IsStackableType(itemType ItemType) bool {
	return itemType == ItemTypeMaterial || itemType == ItemTypeConsumable
}

// GetMaxStackByType 返回指定物品类型的最大堆叠数量。
func GetMaxStackByType(itemType ItemType) int32 {
	switch itemType {
	case ItemTypeMaterial:
		return 9999
	case ItemTypeConsumable:
		return 99
	case ItemTypeWeapon, ItemTypeArmor, ItemTypeAccessory, ItemTypePet:
		return 1
	default:
		// 未注册类型不是合法的业务物品；返回 0 让调用方无法把它
		// 当成一个容量为 1 的有效物品继续处理。
		return 0
	}
}
