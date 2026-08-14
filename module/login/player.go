package login

import "github.com/jerbe/et-go/engine/ecs"

// Player 表示已登录的玩家实体。
type Player struct {
	ecs.Entity
	AccountId int64
	UnitId    int64
}

// NewPlayer 创建玩家实体。
func NewPlayer(accountId int64, unitId int64) *Player {
	player := &Player{
		Entity:    *ecs.NewEntity(),
		AccountId: accountId,
		UnitId:    unitId,
	}
	if unitId > 0 {
		player.SetID(unitId)
	}
	return player
}
