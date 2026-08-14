package db

import "github.com/jerbe/et-go/engine/ecs"

// IDBEntityCollection 标记可自动持久化到 MongoDB 的 ECS 组件。
type IDBEntityCollection interface {
	ecs.Component
	// CollectionName 返回 MongoDB 集合名。
	CollectionName() string
	// GetID 返回文档 `_id`。
	GetID() int64
}
