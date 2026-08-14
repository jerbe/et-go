package coroutinelock

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockTypeConstants(t *testing.T) {
	if LockTypeLogin != 9001 {
		t.Fatalf("LockTypeLogin = %d, want 9001", LockTypeLogin)
	}
	if LockTypeDB != 9002 {
		t.Fatalf("LockTypeDB = %d, want 9002", LockTypeDB)
	}
	if LockTypeLocation != 9003 {
		t.Fatalf("LockTypeLocation = %d, want 9003", LockTypeLocation)
	}
}

func TestLockTypeManagerDifferentKeysCanRunConcurrently(t *testing.T) {
	manager := NewLockTypeManager()
	var active int32
	var maxActive int32
	var waitGroup sync.WaitGroup

	for index := 0; index < 100; index++ {
		waitGroup.Add(1)
		go func(key int64) {
			defer waitGroup.Done()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			release, err := manager.Acquire(ctx, key)
			if err != nil {
				t.Errorf("acquire key %d error: %v", key, err)
				return
			}

			current := atomic.AddInt32(&active, 1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if current <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, current) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
			release()
		}(int64(index))
	}
	waitGroup.Wait()

	if atomic.LoadInt32(&maxActive) <= 1 {
		t.Fatalf("maxActive = %d, want > 1", maxActive)
	}
	if manager.queueCount() != 0 {
		t.Fatalf("queueCount = %d, want 0", manager.queueCount())
	}
}

func TestLockTypeManagerCleanupAfterRelease(t *testing.T) {
	manager := NewLockTypeManager()

	release, err := manager.Acquire(context.Background(), 1001)
	if err != nil {
		t.Fatalf("acquire error: %v", err)
	}
	if manager.queueCount() != 1 {
		t.Fatalf("queueCount = %d, want 1", manager.queueCount())
	}

	release()
	if manager.queueCount() != 0 {
		t.Fatalf("queueCount = %d, want 0", manager.queueCount())
	}
}
