package login

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// PlayerComponent 管理 Gate 上的在线玩家。
type PlayerComponent struct {
	ecs.BaseComponent
	mu      sync.RWMutex
	players map[int64]*Player
	closed  bool
}

// Type 返回组件名称。
func (c *PlayerComponent) Type() string { return "PlayerComponent" }

// Awake 初始化玩家注册表。
func (c *PlayerComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.players == nil {
		c.players = make(map[int64]*Player)
	}
}

// OnDestroy 销毁所有玩家实体。
func (c *PlayerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	players := make([]*Player, 0, len(c.players))
	for key, player := range c.players {
		players = append(players, player)
		delete(c.players, key)
	}
	c.players = nil
	c.mu.Unlock()

	for _, player := range players {
		if player != nil && !player.IsDisposed() {
			player.Dispose()
		}
	}
}

// Add 注册玩家。
func (c *PlayerComponent) Add(accountId int64, player *Player) error {
	if c == nil {
		return ErrPlayerComponentClosed
	}
	if accountId <= 0 || player == nil {
		return ErrInvalidLoginRequest
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrPlayerComponentClosed
	}
	if existing, ok := c.players[accountId]; ok && existing != player {
		c.mu.Unlock()
		return ErrPlayerAlreadyExists
	}
	c.players[accountId] = player
	c.mu.Unlock()
	return nil
}

// Get 查询玩家。
func (c *PlayerComponent) Get(accountId int64) (*Player, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, false
	}
	player, ok := c.players[accountId]
	return player, ok
}

// Remove 删除玩家。
func (c *PlayerComponent) Remove(accountId int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	player, ok := c.players[accountId]
	if ok {
		delete(c.players, accountId)
	}
	c.mu.Unlock()

	if ok && player != nil && !player.IsDisposed() {
		player.Dispose()
	}
}

// Count 返回在线玩家数量。
func (c *PlayerComponent) Count() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return 0
	}
	return len(c.players)
}
