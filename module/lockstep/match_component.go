package lockstep

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

type MatchComponent struct {
	ecs.BaseComponent
	mu             sync.Mutex
	waitingPlayers []int64
	lastRoom       *Map2MatchGetRoom
}

func NewMatchComponent() *MatchComponent {
	return &MatchComponent{}
}

func (m *MatchComponent) Type() string { return "MatchComponent" }

func (m *MatchComponent) Match(playerId int64) []int64 {
	if m == nil || playerId <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range m.waitingPlayers {
		if id == playerId {
			return nil
		}
	}
	m.waitingPlayers = append(m.waitingPlayers, playerId)
	if len(m.waitingPlayers) >= MatchCount {
		result := append([]int64(nil), m.waitingPlayers...)
		m.waitingPlayers = nil
		return result
	}
	return nil
}

// Requeue 将尚未完成房间通知的玩家放回匹配队列。
//
// 只有在 Map 房间已成功回收、且 Gate 补偿通知全部成功后才能调用；
// 如果外部状态是否已建立无法确定，调用方必须保留错误而不能重复匹配。
func (m *MatchComponent) Requeue(playerIDs []int64) {
	if m == nil || len(playerIDs) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	existing := make(map[int64]struct{}, len(m.waitingPlayers)+len(playerIDs))
	for _, playerID := range playerIDs {
		if playerID > 0 {
			existing[playerID] = struct{}{}
		}
	}
	restored := make([]int64, 0, len(playerIDs)+len(m.waitingPlayers))
	for _, playerID := range playerIDs {
		if playerID <= 0 {
			continue
		}
		restored = append(restored, playerID)
	}
	for _, playerID := range m.waitingPlayers {
		if playerID <= 0 {
			continue
		}
		if _, exists := existing[playerID]; exists {
			continue
		}
		existing[playerID] = struct{}{}
		restored = append(restored, playerID)
	}
	m.waitingPlayers = restored
}

// WaitingPlayers 返回当前等待中的玩家快照。
func (m *MatchComponent) WaitingPlayers() []int64 {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]int64(nil), m.waitingPlayers...)
}

func (m *MatchComponent) setLastRoom(room *Map2MatchGetRoom) {
	m.mu.Lock()
	m.lastRoom = room
	m.mu.Unlock()
}

func (m *MatchComponent) LastRoom() *Map2MatchGetRoom {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastRoom == nil {
		return nil
	}
	copy := *m.lastRoom
	return &copy
}
