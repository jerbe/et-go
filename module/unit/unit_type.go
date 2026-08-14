package unit

// UnitType 单位类型枚举。
type UnitType int32

const (
	// UnitTypePlayer 玩家单位。
	UnitTypePlayer UnitType = 5001
	// UnitTypeMonster 怪物单位。
	UnitTypeMonster UnitType = 5002
	// UnitTypeNPC NPC 单位。
	UnitTypeNPC UnitType = 5003
)

// String 返回单位类型名。
func (t UnitType) String() string {
	switch t {
	case UnitTypePlayer:
		return "Player"
	case UnitTypeMonster:
		return "Monster"
	case UnitTypeNPC:
		return "NPC"
	default:
		return "Unknown"
	}
}

// IsPlayer 返回是否为玩家单位。
func (t UnitType) IsPlayer() bool {
	return t == UnitTypePlayer
}
