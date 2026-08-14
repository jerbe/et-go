package pool

import "sync"

// Pool 是泛型对象池，用于减少频繁创建/销毁对象的 GC 压力。
// 对应 ET 框架的 ObjectPool<T>。
func New[T any](newFunc func() T) *TypedPool[T] {
	return &TypedPool[T]{
		pool: sync.Pool{
			New: func() any {
				return newFunc()
			},
		},
	}
}

// TypedPool 类型安全的对象池
type TypedPool[T any] struct {
	pool sync.Pool
}

// Get 从池中获取对象
func (p *TypedPool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put 归还对象到池中
func (p *TypedPool[T]) Put(v T) {
	p.pool.Put(v)
}
