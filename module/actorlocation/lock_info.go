package actorlocation

import (
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

// LockInfo 表示位置锁信息。
type LockInfo struct {
	ecs.Entity
	LockActorID actor.ActorID
	release     func()
	timer       *time.Timer
	mu          sync.Mutex
	once        sync.Once
	disposed    bool
}

// NewLockInfo 创建锁信息实体。
func NewLockInfo(lockActorID actor.ActorID, release func()) *LockInfo {
	return &LockInfo{
		Entity:      *ecs.NewEntity(),
		LockActorID: lockActorID,
		release:     release,
	}
}

// SetTimer 设置超时定时器。
func (l *LockInfo) SetTimer(timer *time.Timer) {
	if l == nil || timer == nil {
		return
	}
	l.mu.Lock()
	if l.disposed {
		l.mu.Unlock()
		timer.Stop()
		return
	}
	l.timer = timer
	l.mu.Unlock()
}

// Dispose 释放锁和定时器。
func (l *LockInfo) Dispose() {
	l.once.Do(func() {
		l.mu.Lock()
		l.disposed = true
		if l.timer != nil {
			l.timer.Stop()
		}
		if l.release != nil {
			l.release()
		}
		l.release = nil
		l.timer = nil
		l.LockActorID = actor.ActorID{}
		l.mu.Unlock()
		l.Entity.Dispose()
	})
}
