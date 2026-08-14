package actorlocation

// LocationType 表示 Actor 位置记录类型。
type LocationType int

const (
	// LocationTypeUnit 表示单位所在地图的 ActorID。
	LocationTypeUnit LocationType = 9001
	// LocationTypePlayer 表示玩家所在 Fiber 的 ActorID。
	LocationTypePlayer LocationType = 9002
	// LocationTypeGateSession 表示玩家 Gate 会话的 ActorID。
	LocationTypeGateSession LocationType = 9003
	// LocationTypeAccount 表示账号锁定位置。
	LocationTypeAccount LocationType = 9004
)

// String 返回位置类型名称。
func (t LocationType) String() string {
	switch t {
	case LocationTypeUnit:
		return "Unit"
	case LocationTypePlayer:
		return "Player"
	case LocationTypeGateSession:
		return "GateSession"
	case LocationTypeAccount:
		return "Account"
	default:
		return "Unknown"
	}
}
