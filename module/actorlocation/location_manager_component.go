package actorlocation

import (
	"sync"

	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
)

// LocationManagerComponent 管理所有 LocationOneType。
type LocationManagerComponent struct {
	ecs.BaseComponent
	mu     sync.RWMutex
	types  map[int]*LocationOneType
	lock   *coroutinelock.Lock
	closed bool
}

// Type 返回组件类型名称。
func (m *LocationManagerComponent) Type() string { return "LocationManagerComponent" }

// Awake 初始化内部状态。
func (m *LocationManagerComponent) Awake() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if m.types == nil {
		m.types = make(map[int]*LocationOneType)
	}
	if m.lock == nil {
		m.lock = coroutinelock.New()
	}
}

// OnDestroy 清理所有子类型管理器。
func (m *LocationManagerComponent) OnDestroy() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	types := make([]*LocationOneType, 0, len(m.types))
	for key, lot := range m.types {
		types = append(types, lot)
		delete(m.types, key)
	}
	m.lock = nil
	m.mu.Unlock()

	for _, lot := range types {
		if lot != nil {
			lot.Close()
		}
	}
}

// SetLock 设置共享协程锁。
func (m *LocationManagerComponent) SetLock(lock *coroutinelock.Lock) {
	if m == nil {
		return
	}
	m.mu.Lock()
	if !m.closed {
		m.lock = lock
	}
	m.mu.Unlock()
}

// Get 获取指定类型的位置管理器。
func (m *LocationManagerComponent) Get(locationType int) *LocationOneType {
	if m == nil {
		return nil
	}
	m.Awake()
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil
	}
	if lot, ok := m.types[locationType]; ok {
		m.mu.RUnlock()
		return lot
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	if lot, ok := m.types[locationType]; ok {
		return lot
	}
	lot := NewLocationOneType(LocationType(locationType), m.lock)
	if owner := m.GetEntity(); owner != nil {
		owner.AddChildWithID(int64(locationType), &lot.Entity)
	}
	m.types[locationType] = lot
	return lot
}
