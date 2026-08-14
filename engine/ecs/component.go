package ecs

// Component 是 ECS 架构中"组件"的接口。
// 组件仅包含数据和类型信息，业务逻辑由 System 接口驱动。
type Component interface {
	// Type 返回组件类型名称（用于注册和查找）
	Type() string
	// SetEntity 设置组件所属的实体
	SetEntity(e *Entity)
	// GetEntity 获取组件所属的实体
	GetEntity() *Entity
}

// BaseComponent 提供 Component 接口的基础实现，所有组件可嵌入此结构体。
type BaseComponent struct {
	entity *Entity
	id     int64
}

// SetEntity 设置组件所属的实体
func (bc *BaseComponent) SetEntity(e *Entity) { bc.entity = e }

// GetEntity 获取组件所属的实体
func (bc *BaseComponent) GetEntity() *Entity { return bc.entity }

// SetID 设置组件 ID。
func (bc *BaseComponent) SetID(id int64) { bc.id = id }

// ID 返回组件 ID。
func (bc *BaseComponent) ID() int64 { return bc.id }
