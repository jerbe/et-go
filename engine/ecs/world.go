package ecs

import "sync"

// World 是全局单例管理中心，负责管理所有实体系统和全局状态。
type World struct {
	mu         sync.RWMutex
	singletons map[string]any
}

// NewWorld 创建 World 实例
func NewWorld() *World {
	return &World{
		singletons: make(map[string]any),
	}
}

// RegisterSingleton 注册全局单例
func (w *World) RegisterSingleton(name string, instance any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.singletons == nil {
		w.singletons = make(map[string]any)
	}
	w.singletons[name] = instance
}

// GetSingleton 获取全局单例
func (w *World) GetSingleton(name string) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.singletons == nil {
		return nil, false
	}
	v, ok := w.singletons[name]
	return v, ok
}

// RemoveSingleton 移除全局单例。
func (w *World) RemoveSingleton(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.singletons == nil {
		return
	}
	delete(w.singletons, name)
}

// Shutdown 关闭 World，释放所有资源
func (w *World) Shutdown() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.singletons = nil
}
