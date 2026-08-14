package coroutinelock

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"
)

// Option 定义协程锁构造选项。
type Option func(*Lock)

// Lock 协程锁管理器，支持多 LockType 和 FIFO 等待队列。
type Lock struct {
	mu          sync.RWMutex
	managers    map[int]*LockTypeManager
	nameMu      sync.Mutex
	nameQueues  map[string]*lockQueue
	warnTimeout time.Duration
	logger      *slog.Logger
}

// New 创建协程锁管理器。
func New(opts ...Option) *Lock {
	lock := &Lock{
		managers:   make(map[int]*LockTypeManager),
		nameQueues: make(map[string]*lockQueue),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(lock)
		}
	}
	return lock
}

// WithWarnTimeout 配置锁持有超时告警阈值。
func WithWarnTimeout(duration time.Duration) Option {
	return func(lock *Lock) {
		lock.warnTimeout = duration
	}
}

// WithLogger 配置协程锁使用的日志器。
func WithLogger(logger *slog.Logger) Option {
	return func(lock *Lock) {
		lock.logger = logger
	}
}

// Acquire 获取指定 lockType 和 key 的锁。
func (l *Lock) Acquire(ctx context.Context, lockType int, key int64) (func(), error) {
	if l == nil {
		return nil, ErrLockManagerMissing
	}
	if ctx == nil {
		return nil, ErrLockContextRequired
	}
	manager := l.getManager(lockType)
	rawRelease, err := manager.Acquire(ctx, key)
	if err != nil {
		return nil, err
	}
	return l.wrapRelease(lockType, key, time.Now(), rawRelease), nil
}

// AcquireByType 使用 lockType 和 key 获取锁。
func (l *Lock) AcquireByType(ctx context.Context, lockType int, key int64) (func(), error) {
	return l.Acquire(ctx, lockType, key)
}

// AcquireByName 使用字符串 key 获取兼容锁。
func (l *Lock) AcquireByName(ctx context.Context, key string) (func(), error) {
	if l == nil {
		return nil, ErrLockManagerMissing
	}
	if ctx == nil {
		return nil, ErrLockContextRequired
	}

	l.nameMu.Lock()
	queue, ok := l.nameQueues[key]
	if !ok {
		queue = newLockQueue()
		l.nameQueues[key] = queue
	}
	l.nameMu.Unlock()

	rawRelease, err := queue.acquire(ctx)
	if err != nil {
		l.tryCleanupName(key, queue)
		return nil, err
	}

	keyHash := hashStringKey(key)
	return l.wrapRelease(0, keyHash, time.Now(), func() {
		rawRelease()
		l.tryCleanupName(key, queue)
	}), nil
}

func (l *Lock) getManager(lockType int) *LockTypeManager {
	l.mu.RLock()
	manager, ok := l.managers[lockType]
	l.mu.RUnlock()
	if ok {
		return manager
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	manager, ok = l.managers[lockType]
	if ok {
		return manager
	}
	manager = NewLockTypeManager()
	l.managers[lockType] = manager
	return manager
}

func (l *Lock) wrapRelease(lockType int, key int64, acquiredAt time.Time, release func()) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			held := time.Since(acquiredAt)
			if l.warnTimeout > 0 && held > l.warnTimeout {
				logger := l.logger
				if logger == nil {
					logger = slog.Default()
				}
				logger.Warn("协程锁持有时间过长",
					"lockType", lockType,
					"key", key,
					"held", held.String(),
					"threshold", l.warnTimeout.String(),
				)
			}
			release()
		})
	}
}

func (l *Lock) tryCleanupName(key string, queue *lockQueue) {
	if l == nil || queue == nil {
		return
	}

	l.nameMu.Lock()
	defer l.nameMu.Unlock()
	if current, ok := l.nameQueues[key]; ok && current == queue && queue.isEmpty() {
		delete(l.nameQueues, key)
	}
}

func hashStringKey(key string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(key))
	return int64(hasher.Sum64())
}
