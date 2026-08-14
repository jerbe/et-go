package ecs

import (
	"log/slog"
	"sync/atomic"
)

// instanceIDGen 全局实例 ID 生成器
var instanceIDGen atomic.Int64

// nextInstanceID 生成下一个全局唯一的实例 ID
func nextInstanceID() int64 {
	return instanceIDGen.Add(1)
}

// Entity 是所有游戏对象的基类，实现 ECS 架构中的"实体"概念。
// 每个 Entity 拥有唯一 InstanceID，支持父子层级关系和组件组合。
type Entity struct {
	id         int64
	instanceID int64
	parent     *Entity
	children   map[int64]*Entity
	components map[string]Component
	status     EntityStatus

	isDisposed bool
	scene      *Scene
}

// NewEntity 创建新实体
func NewEntity() *Entity {
	e := &Entity{
		instanceID: nextInstanceID(),
		children:   make(map[int64]*Entity),
		components: make(map[string]Component),
		status:     StatusIsNew,
	}
	return e
}

// ID 返回实体 ID
func (e *Entity) ID() int64 { return e.id }

// SetID 设置实体 ID
func (e *Entity) SetID(id int64) { e.id = id }

// InstanceID 返回实例 ID（全局唯一）
func (e *Entity) InstanceID() int64 { return e.instanceID }

// Parent 返回父实体
func (e *Entity) Parent() *Entity { return e.parent }

// IsDisposed 检查实体是否已销毁
func (e *Entity) IsDisposed() bool { return e.isDisposed }

// Scene 返回实体所属场景。
func (e *Entity) Scene() *Scene { return e.scene }

// SetStatus 设置状态标志。
func (e *Entity) SetStatus(flag EntityStatus) {
	e.status = e.status.Set(flag)
}

// HasStatus 检查状态标志。
func (e *Entity) HasStatus(flag EntityStatus) bool {
	return e.status.Has(flag)
}

// ClearStatus 清除状态标志。
func (e *Entity) ClearStatus(flag EntityStatus) {
	e.status = e.status.Clear(flag)
}

// AddComponent 添加组件到实体
func (e *Entity) AddComponent(c Component) {
	e.addComponent(c, 0, false)
}

// AddComponentWithID 指定 ID 添加组件。
func (e *Entity) AddComponentWithID(id int64, c Component) {
	e.addComponent(c, id, true)
}

// GetComponent 获取指定类型的组件
func (e *Entity) GetComponent(typeName string) (Component, bool) {
	c, ok := e.components[typeName]
	return c, ok
}

// RemoveComponent 移除指定类型的组件
func (e *Entity) RemoveComponent(typeName string) {
	if e == nil {
		return
	}
	c, ok := e.components[typeName]
	if !ok {
		return
	}
	e.destroyComponent(c)
	delete(e.components, typeName)
	c.SetEntity(nil)
}

// AddChild 添加子实体
func (e *Entity) AddChild(child *Entity) {
	e.addChild(child, 0, false)
}

// AddChildWithID 指定 ID 添加子实体。
func (e *Entity) AddChildWithID(id int64, child *Entity) {
	e.addChild(child, id, true)
}

func (e *Entity) addComponent(c Component, id int64, hasID bool) {
	if e == nil || e.isDisposed || c == nil {
		return
	}
	if e.components == nil {
		e.components = make(map[string]Component)
	}

	name := c.Type()
	if existing, ok := e.components[name]; ok {
		e.destroyComponent(existing)
		existing.SetEntity(nil)
		delete(e.components, name)
	}

	c.SetEntity(e)
	if hasID {
		if setter, ok := c.(interface{ SetID(int64) }); ok {
			setter.SetID(id)
		}
	}
	e.components[name] = c
	if sys, ok := c.(AwakeSystem); ok {
		sys.Awake()
	}
}

func (e *Entity) addChild(child *Entity, id int64, hasID bool) {
	if e == nil || e.isDisposed || child == nil || child.isDisposed || child == e {
		return
	}
	if hasID {
		child.SetID(id)
	}
	if child.parent != nil && child.parent != e {
		child.parent.RemoveChild(child.instanceID)
	}
	if e.children == nil {
		e.children = make(map[int64]*Entity)
	}
	child.parent = e
	e.children[child.instanceID] = child
	if scene := e.findScene(); scene != nil {
		scene.RegisterEntity(child)
	}
}

// RemoveChild 移除子实体
func (e *Entity) RemoveChild(instanceID int64) {
	if e == nil {
		return
	}
	child, ok := e.children[instanceID]
	if !ok {
		return
	}
	delete(e.children, instanceID)
	if child.scene != nil {
		child.scene.unregisterEntityTree(child)
	}
	child.parent = nil
}

// Children 返回所有子实体
func (e *Entity) Children() map[int64]*Entity {
	return e.children
}

// Components 返回所有组件
func (e *Entity) Components() map[string]Component {
	return e.components
}

// GetTransferComponents 返回所有支持迁移的组件。
func (e *Entity) GetTransferComponents() []Component {
	if e == nil || len(e.components) == 0 {
		return nil
	}
	components := make([]Component, 0)
	for _, component := range e.components {
		if _, ok := component.(TransferSystem); ok {
			components = append(components, component)
		}
	}
	return components
}

// Dispose 销毁实体及其所有子实体和组件
func (e *Entity) Dispose() {
	if e == nil || e.isDisposed {
		return
	}
	e.isDisposed = true

	children := make([]*Entity, 0, len(e.children))
	for _, child := range e.children {
		children = append(children, child)
	}
	for _, child := range children {
		child.Dispose()
	}

	components := make([]struct {
		name      string
		component Component
	}, 0, len(e.components))
	for name, component := range e.components {
		components = append(components, struct {
			name      string
			component Component
		}{
			name:      name,
			component: component,
		})
	}
	for _, item := range components {
		e.destroyComponent(item.component)
		item.component.SetEntity(nil)
		delete(e.components, item.name)
	}

	if e.parent != nil {
		parent := e.parent
		e.parent = nil
		parent.RemoveChild(e.instanceID)
	}

	if e.scene != nil {
		e.scene.unregisterEntityTree(e)
	}

	e.children = nil
	e.components = nil
	e.scene = nil
}

func (e *Entity) destroyComponent(c Component) {
	if c == nil {
		return
	}
	if sys, ok := c.(DestroySystem); ok {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("ECS component destroy panicked", "component", c.Type(), "panic", recovered)
				}
			}()
			sys.OnDestroy()
		}()
	}
}

func (e *Entity) findScene() *Scene {
	if e == nil {
		return nil
	}
	if e.scene != nil {
		return e.scene
	}
	for parent := e.parent; parent != nil; parent = parent.parent {
		if parent.scene != nil {
			return parent.scene
		}
	}
	return nil
}
