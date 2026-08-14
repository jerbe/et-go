package unit

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// UnitComponent 管理场景内所有单位。
type UnitComponent struct {
	ecs.BaseComponent
	mu     sync.RWMutex
	units  map[int64]*Unit
	closed bool
}

// Type 返回组件名称。
func (c *UnitComponent) Type() string { return "UnitComponent" }

// Awake 初始化注册表。
func (c *UnitComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.units == nil {
		c.units = make(map[int64]*Unit)
	}
}

// Add 注册单位。
func (c *UnitComponent) Add(unit *Unit) error {
	if c == nil {
		return ErrUnitComponentClosed
	}
	if unit == nil || unit.ID() <= 0 {
		return ErrInvalidUnitID
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrUnitComponentClosed
	}
	if existing, ok := c.units[unit.ID()]; ok && existing != unit {
		c.mu.Unlock()
		return ErrUnitAlreadyExists
	}
	c.units[unit.ID()] = unit
	c.mu.Unlock()
	return nil
}

// Get 按 ID 查询单位。
func (c *UnitComponent) Get(id int64) (*Unit, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.units == nil {
		return nil, false
	}
	unit, ok := c.units[id]
	return unit, ok
}

// Remove 从注册表移除单位。
func (c *UnitComponent) Remove(id int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed || c.units == nil {
		c.mu.Unlock()
		return
	}
	delete(c.units, id)
	c.mu.Unlock()
}

// GetAll 返回所有单位注册表。
func (c *UnitComponent) GetAll() map[int64]*Unit {
	if c == nil {
		return nil
	}
	c.Awake()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil
	}
	result := make(map[int64]*Unit, len(c.units))
	for id, current := range c.units {
		result[id] = current
	}
	return result
}

// Count 返回单位数量。
func (c *UnitComponent) Count() int {
	if c == nil {
		return 0
	}
	return len(c.GetAll())
}

// OnDestroy 销毁所有单位。
func (c *UnitComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	units := make([]*Unit, 0, len(c.units))
	for id, current := range c.units {
		units = append(units, current)
		delete(c.units, id)
	}
	c.units = nil
	c.mu.Unlock()

	for _, unit := range units {
		if unit != nil && !unit.IsDisposed() {
			unit.Dispose()
		}
	}
}
