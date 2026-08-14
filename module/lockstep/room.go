package lockstep

import "github.com/jerbe/et-go/engine/ecs"

type LockstepRoom struct {
	ecs.Entity
	World           *LSWorld
	StartTime       int64
	PlayerIds       []int64
	AuthorityFrame  int
	PredictionFrame int
	FrameBuffer     *FrameBuffer
	Replay          *Replay
}

// NewLockstepRoomWithError 创建一个带确定性世界的 Room。
//
// 空玩家列表用于构造尚未注入玩家的目标 Room（例如快照恢复目标），
// 非空列表必须通过原子世界初始化，否则直接返回错误。
func NewLockstepRoomWithError(playerIds []int64) (*LockstepRoom, error) {
	room := &LockstepRoom{
		Entity:         *ecs.NewEntity(),
		World:          NewLSWorld(int(ecs.SceneTypeLockStep)),
		PlayerIds:      append([]int64(nil), playerIds...),
		FrameBuffer:    NewFrameBuffer(),
		Replay:         NewReplay(),
		AuthorityFrame: 0,
	}
	if len(playerIds) == 0 {
		return room, nil
	}
	if err := room.World.InitializePlayers(playerIds); err != nil {
		return nil, err
	}
	return room, nil
}

// NewLockstepRoom 保留无错误返回值的内部构造入口。
//
// 玩家 ID 非法时返回 nil；生产创建路径应使用
// NewLockstepRoomWithError，以便把初始化错误传递给调用方。
func NewLockstepRoom(playerIds []int64) *LockstepRoom {
	room, err := NewLockstepRoomWithError(playerIds)
	if err != nil {
		return nil
	}
	return room
}

// SyncWorldPlayers 将 Room 的玩家集合同步到确定性世界。
func (r *LockstepRoom) SyncWorldPlayers(playerIds []int64) error {
	if r == nil || r.World == nil {
		return ErrLSWorldMissing
	}
	if len(playerIds) == 0 {
		return ErrRoomPlayersInvalid
	}
	return r.World.SyncPlayers(playerIds)
}
