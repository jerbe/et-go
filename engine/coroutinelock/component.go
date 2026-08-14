package coroutinelock

import (
	"context"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// CoroutineLockComponent 协程锁 ECS 组件，挂载于 Scene 使用。
type CoroutineLockComponent struct {
	ecs.BaseComponent
	lock   *Lock
	closed bool
}

// Type 返回组件类型名称。
func (c *CoroutineLockComponent) Type() string { return "CoroutineLockComponent" }

// Awake 初始化底层锁实现。
func (c *CoroutineLockComponent) Awake() {
	if c == nil || c.closed {
		return
	}
	if c.lock != nil {
		return
	}
	c.lock = New(WithWarnTimeout(30 * time.Second))
}

// OnDestroy 清理组件资源。
func (c *CoroutineLockComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.closed = true
	c.lock = nil
}

// Acquire 获取指定 lockType 和 key 的协程锁。
func (c *CoroutineLockComponent) Acquire(ctx context.Context, lockType int, key int64) (func(), error) {
	if c == nil {
		return nil, ErrComponentNotAwake
	}
	if c.closed || c.lock == nil {
		return nil, ErrComponentNotAwake
	}
	return c.lock.Acquire(ctx, lockType, key)
}

// Lock 返回底层锁实现。
func (c *CoroutineLockComponent) Lock() *Lock {
	if c == nil {
		return nil
	}
	if c.closed {
		return nil
	}
	return c.lock
}
