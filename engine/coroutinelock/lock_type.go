package coroutinelock

import (
	"context"
	"sync"
)

const (
	// LockTypeLogin 防止同一账号并发登录。
	LockTypeLogin = 9001
	// LockTypeDB 用于数据库操作互斥。
	LockTypeDB = 9002
	// LockTypeLocation 用于 Actor 位置锁定。
	LockTypeLocation = 9003
)

// LockTypeManager 管理同一 LockType 下的所有 key 队列。
type LockTypeManager struct {
	mu     sync.Mutex
	queues map[int64]*lockQueue
}

// NewLockTypeManager 创建单类型锁管理器。
func NewLockTypeManager() *LockTypeManager {
	return &LockTypeManager{
		queues: make(map[int64]*lockQueue),
	}
}

// Acquire 获取指定 key 的锁。
func (m *LockTypeManager) Acquire(ctx context.Context, key int64) (func(), error) {
	if m == nil {
		return nil, ErrLockManagerMissing
	}

	m.mu.Lock()
	queue, ok := m.queues[key]
	if !ok {
		queue = newLockQueue()
		m.queues[key] = queue
	}
	m.mu.Unlock()

	rawRelease, err := queue.acquire(ctx)
	if err != nil {
		m.tryCleanup(key, queue)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			rawRelease()
			m.tryCleanup(key, queue)
		})
	}, nil
}

func (m *LockTypeManager) tryCleanup(key int64, queue *lockQueue) {
	if m == nil || queue == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.queues[key]; ok && current == queue && queue.isEmpty() {
		delete(m.queues, key)
	}
}

func (m *LockTypeManager) queueCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queues)
}
