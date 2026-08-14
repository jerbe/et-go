package central

import (
	"context"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
)

// PlayerProfileStore 是玩家档案的显式持久化依赖。
// 生产环境默认由 DBManagerComponent 提供；测试或特殊部署必须显式注入实现，
// 不允许在持久化失败时隐式生成内存档案。
type PlayerProfileStore interface {
	LoadOrCreatePlayerProfile(ctx context.Context, zone int, accountID int64) (*db.CPlayerProfile, error)
}

// PlayerProfileStoreComponent 将显式档案存储实现挂载到 Scene。
type PlayerProfileStoreComponent struct {
	ecs.BaseComponent
	store PlayerProfileStore
}

// Type 返回组件类型。
func (c *PlayerProfileStoreComponent) Type() string { return "PlayerProfileStoreComponent" }

// SetStore 设置档案存储实现。
func (c *PlayerProfileStoreComponent) SetStore(store PlayerProfileStore) {
	c.store = store
}

// Store 返回档案存储实现。
func (c *PlayerProfileStoreComponent) Store() PlayerProfileStore {
	if c == nil {
		return nil
	}
	return c.store
}
