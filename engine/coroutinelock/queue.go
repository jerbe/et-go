package coroutinelock

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// lockQueue 管理单个 key 的锁等待队列。
type lockQueue struct {
	mu      sync.Mutex
	waiters []chan struct{}
	held    bool
}

func newLockQueue() *lockQueue {
	return &lockQueue{
		waiters: make([]chan struct{}, 0),
	}
}

func (q *lockQueue) acquire(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, ErrLockContextRequired
	}
	q.mu.Lock()
	if !q.held {
		q.held = true
		q.mu.Unlock()
		return q.onceRelease(), nil
	}

	waiter := make(chan struct{}, 1)
	q.waiters = append(q.waiters, waiter)
	q.mu.Unlock()

	select {
	case <-waiter:
		return q.onceRelease(), nil
	case <-ctx.Done():
		q.mu.Lock()
		removed := q.removeWaiterLocked(waiter)
		q.mu.Unlock()
		if removed {
			return nil, mapContextErr(ctx.Err())
		}
		return q.onceRelease(), nil
	}
}

func (q *lockQueue) onceRelease() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			q.release()
		})
	}
}

func (q *lockQueue) release() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.held {
		return
	}
	if len(q.waiters) == 0 {
		q.held = false
		return
	}

	next := q.waiters[0]
	q.waiters = q.waiters[1:]
	next <- struct{}{}
}

func (q *lockQueue) isEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return !q.held && len(q.waiters) == 0
}

func (q *lockQueue) removeWaiterLocked(target chan struct{}) bool {
	for index, waiter := range q.waiters {
		if waiter != target {
			continue
		}
		q.waiters = append(q.waiters[:index], q.waiters[index+1:]...)
		return true
	}
	return false
}

func (q *lockQueue) waiterCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.waiters)
}

func mapContextErr(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrLockTimeout, err)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: %w", ErrLockCanceled, err)
	default:
		return err
	}
}
