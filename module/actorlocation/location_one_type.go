package actorlocation

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
)

// LocationOneType 表示单一类型的位置信息管理器。
type LocationOneType struct {
	ecs.Entity
	mu        sync.RWMutex
	locations map[int64]actor.ActorID
	lockInfos map[int64]*LockInfo
	lock      *coroutinelock.Lock
	closed    bool
}

// NewLocationOneType 创建单类型位置管理器。
func NewLocationOneType(locationType LocationType, lock *coroutinelock.Lock) *LocationOneType {
	lot := &LocationOneType{
		Entity: *ecs.NewEntity(),
		lock:   lock,
	}
	lot.SetID(int64(locationType))
	lot.Awake()
	return lot
}

// Awake 初始化内部状态。
func (lot *LocationOneType) Awake() {
	if lot == nil {
		return
	}
	lot.mu.Lock()
	defer lot.mu.Unlock()
	if lot.closed {
		return
	}
	if lot.locations == nil {
		lot.locations = make(map[int64]actor.ActorID)
	}
	if lot.lockInfos == nil {
		lot.lockInfos = make(map[int64]*LockInfo)
	}
	if lot.lock == nil {
		lot.lock = coroutinelock.New()
	}
}

// Close 释放所有持有锁并销毁实体。
func (lot *LocationOneType) Close() {
	if lot == nil {
		return
	}

	lot.mu.Lock()
	if lot.closed {
		lot.mu.Unlock()
		return
	}
	lot.closed = true
	infos := make([]*LockInfo, 0, len(lot.lockInfos))
	for key, info := range lot.lockInfos {
		infos = append(infos, info)
		delete(lot.lockInfos, key)
	}
	lot.locations = nil
	lot.lock = nil
	lot.mu.Unlock()

	for _, info := range infos {
		if info != nil {
			info.Dispose()
		}
	}

	if !lot.IsDisposed() {
		lot.Entity.Dispose()
	}
}

// Add 注册位置。
func (lot *LocationOneType) Add(key int64, actorID actor.ActorID) error {
	if lot == nil {
		return ErrLocationClosed
	}
	if key <= 0 {
		return ErrZeroLocationKey
	}
	if !actorID.IsValid() {
		return ErrInvalidActorID
	}
	release, err := lot.acquire(key)
	if err != nil {
		return err
	}
	defer release()

	lot.mu.Lock()
	if lot.closed {
		lot.mu.Unlock()
		return ErrLocationClosed
	}
	lot.locations[key] = actorID
	lot.mu.Unlock()
	slog.Debug("actorlocation add", "type", lot.ID(), "key", key)
	return nil
}

// Get 查询位置。
func (lot *LocationOneType) Get(key int64) (actor.ActorID, error) {
	if lot == nil {
		return actor.ActorID{}, ErrLocationClosed
	}
	if key <= 0 {
		return actor.ActorID{}, ErrZeroLocationKey
	}
	release, err := lot.acquire(key)
	if err != nil {
		return actor.ActorID{}, err
	}
	defer release()

	lot.mu.RLock()
	defer lot.mu.RUnlock()
	if lot.closed {
		return actor.ActorID{}, ErrLocationClosed
	}
	return lot.locations[key], nil
}

// TryGet 查询位置但不等待显式位置锁，供 Actor handler 使用。
func (lot *LocationOneType) TryGet(key int64) (actor.ActorID, error) {
	if lot == nil {
		return actor.ActorID{}, ErrLocationClosed
	}
	if key <= 0 {
		return actor.ActorID{}, ErrZeroLocationKey
	}
	lot.mu.RLock()
	defer lot.mu.RUnlock()
	if lot.closed {
		return actor.ActorID{}, ErrLocationClosed
	}
	if _, locked := lot.lockInfos[key]; locked {
		return actor.ActorID{}, ErrLocationLocked
	}
	return lot.locations[key], nil
}

// Lock 锁定位置。
func (lot *LocationOneType) Lock(key int64, actorID actor.ActorID, timeMs int) error {
	if lot == nil {
		return ErrLocationClosed
	}
	if key <= 0 {
		return ErrZeroLocationKey
	}
	if !actorID.IsValid() {
		return ErrInvalidActorID
	}
	release, err := lot.acquire(key)
	if err != nil {
		return err
	}

	info := NewLockInfo(actorID, release)
	lot.mu.Lock()
	if lot.closed || lot.IsDisposed() {
		lot.mu.Unlock()
		info.Dispose()
		return ErrLocationClosed
	}
	lot.Entity.AddChildWithID(key, &info.Entity)
	lot.lockInfos[key] = info
	lot.mu.Unlock()

	if timeMs > 0 {
		instanceID := info.InstanceID()
		timer := time.AfterFunc(time.Duration(timeMs)*time.Millisecond, func() {
			lot.mu.RLock()
			current := lot.lockInfos[key]
			lot.mu.RUnlock()
			if current == nil || current.InstanceID() != instanceID {
				return
			}
			if err := lot.Unlock(key, actorID, actorID); err != nil {
				slog.Error("actorlocation timed unlock failed", "type", lot.ID(), "key", key, "err", err)
			}
		})
		info.SetTimer(timer)
	}
	return nil
}

// Unlock 解锁位置。
func (lot *LocationOneType) Unlock(key int64, oldActorID, newActorID actor.ActorID) error {
	if lot == nil {
		return ErrLocationClosed
	}
	if key <= 0 {
		return ErrZeroLocationKey
	}
	lot.mu.Lock()
	if lot.closed {
		lot.mu.Unlock()
		return ErrLocationClosed
	}
	info, ok := lot.lockInfos[key]
	if !ok {
		lot.mu.Unlock()
		slog.Error("actorlocation unlock failed: lock info not found", "type", lot.ID(), "key", key)
		return ErrUnlockFailed
	}
	if oldActorID.IsValid() && info.LockActorID != oldActorID {
		lot.mu.Unlock()
		slog.Error("actorlocation unlock failed: old actor mismatch", "type", lot.ID(), "key", key)
		return ErrUnlockFailed
	}
	if newActorID != (actor.ActorID{}) && !newActorID.IsValid() {
		lot.mu.Unlock()
		return ErrInvalidActorID
	}
	if newActorID.IsValid() {
		lot.locations[key] = newActorID
	}
	delete(lot.lockInfos, key)
	lot.mu.Unlock()

	info.Dispose()
	return nil
}

// Remove 删除位置。
func (lot *LocationOneType) Remove(key int64) error {
	if lot == nil {
		return ErrLocationClosed
	}
	if key <= 0 {
		return ErrZeroLocationKey
	}
	release, err := lot.acquire(key)
	if err != nil {
		return err
	}
	defer release()

	lot.mu.Lock()
	if lot.closed {
		lot.mu.Unlock()
		return ErrLocationClosed
	}
	delete(lot.locations, key)
	lot.mu.Unlock()
	return nil
}

func (lot *LocationOneType) getCoroutineLockType() int {
	return int((lot.ID() << 32) | int64(coroutinelock.LockTypeLocation))
}

func (lot *LocationOneType) acquire(key int64) (func(), error) {
	if lot == nil {
		return nil, ErrLocationClosed
	}
	if key <= 0 {
		return nil, ErrZeroLocationKey
	}
	lot.Awake()

	ctx, cancel := timeLimitedContext(defaultAcquireTimeout)
	lot.mu.RLock()
	if lot.closed || lot.lock == nil {
		lot.mu.RUnlock()
		cancel()
		return nil, ErrLocationClosed
	}
	lock := lot.lock
	lot.mu.RUnlock()
	release, err := lock.Acquire(ctx, lot.getCoroutineLockType(), key)
	cancel()
	if err != nil {
		return nil, err
	}
	return release, nil
}
