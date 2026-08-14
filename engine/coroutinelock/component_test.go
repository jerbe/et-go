package coroutinelock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestCoroutineLockComponentLifecycle(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMain, 1, "main")
	component := &CoroutineLockComponent{}
	if component.Type() != "CoroutineLockComponent" {
		t.Fatalf("Type() = %q, want %q", component.Type(), "CoroutineLockComponent")
	}

	_, err := component.Acquire(context.Background(), 9001, 1)
	if !errors.Is(err, ErrComponentNotAwake) {
		t.Fatalf("Acquire before Awake error = %v, want %v", err, ErrComponentNotAwake)
	}

	scene.AddComponent(component)
	release, err := component.Acquire(context.Background(), 9001, 1)
	if err != nil {
		t.Fatalf("Acquire after Awake error = %v", err)
	}
	release()

	scene.RemoveComponent(component.Type())
	_, err = component.Acquire(context.Background(), 9001, 1)
	if !errors.Is(err, ErrComponentNotAwake) {
		t.Fatalf("Acquire after OnDestroy error = %v, want %v", err, ErrComponentNotAwake)
	}
	component.Awake()
	if _, err := component.Acquire(context.Background(), 9001, 1); !errors.Is(err, ErrComponentNotAwake) {
		t.Fatalf("Acquire after repeated Awake error = %v, want %v", err, ErrComponentNotAwake)
	}
}

func TestCoroutineLockComponentSerialForSameLock(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	firstRelease, err := component.Acquire(context.Background(), 9001, 100)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}

	var started atomic.Bool
	var acquired atomic.Bool
	done := make(chan struct{})
	go func() {
		started.Store(true)
		release, err := component.Acquire(context.Background(), 9001, 100)
		if err == nil {
			acquired.Store(true)
			release()
		}
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	if !started.Load() {
		t.Fatal("second goroutine should start")
	}
	if acquired.Load() {
		t.Fatal("second acquire should block before first release")
	}

	firstRelease()

	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("second acquire should finish after first release")
	}
}

func TestCoroutineLockComponentAwakeDoesNotReplaceActiveLock(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()
	firstRelease, err := component.Acquire(context.Background(), 9001, 500)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()

	component.Awake()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := component.Acquire(ctx, 9001, 500); !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("second acquire after Awake error = %v, want %v", err, ErrLockTimeout)
	}
}

func TestCoroutineLockComponentDifferentLockTypeIsolation(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	firstRelease, err := component.Acquire(context.Background(), 9001, 200)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	secondRelease, err := component.Acquire(ctx, 9002, 200)
	if err != nil {
		t.Fatalf("second acquire should not block across lockType, error = %v", err)
	}
	secondRelease()

	if elapsed := time.Since(start); elapsed > 80*time.Millisecond {
		t.Fatalf("acquire across lockType took too long: %v", elapsed)
	}
}

func TestCoroutineLockComponentTimeout(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	firstRelease, err := component.Acquire(context.Background(), 9003, 300)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = component.Acquire(ctx, 9003, 300)
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("timeout error = %v, want %v", err, ErrLockTimeout)
	}
}

func TestCoroutineLockComponentMixedScenario(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	const workers = 12
	var wg sync.WaitGroup
	wg.Add(workers)

	var critical int64
	var maxCritical int64
	var failures int64

	for index := 0; index < workers; index++ {
		lockType := 9001 + (index % 3)
		key := int64(index % 2)
		go func(lockType int, key int64) {
			defer wg.Done()
			release, err := component.Acquire(context.Background(), lockType, key)
			if err != nil {
				atomic.AddInt64(&failures, 1)
				return
			}
			cur := atomic.AddInt64(&critical, 1)
			for {
				old := atomic.LoadInt64(&maxCritical)
				if cur <= old {
					break
				}
				if atomic.CompareAndSwapInt64(&maxCritical, old, cur) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt64(&critical, -1)
			release()
		}(lockType, key)
	}

	wg.Wait()
	if failures != 0 {
		t.Fatalf("acquire failures = %d, want 0", failures)
	}
	if maxCritical <= 0 {
		t.Fatalf("maxCritical = %d, want > 0", maxCritical)
	}
}

func TestCoroutineLockComponentDBBuckets(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	sameBucketKeyA := int64(32 % 32)
	sameBucketKeyB := int64(64 % 32)
	crossBucketKey := int64(65 % 32)

	firstRelease, err := component.Acquire(context.Background(), LockTypeDB, sameBucketKeyA)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}

	blocked := make(chan struct{})
	unblocked := make(chan struct{})
	go func() {
		close(blocked)
		release, acquireErr := component.Acquire(context.Background(), LockTypeDB, sameBucketKeyB)
		if acquireErr == nil {
			release()
		}
		close(unblocked)
	}()

	<-blocked
	select {
	case <-unblocked:
		t.Fatal("same bucket DB lock should block")
	case <-time.After(20 * time.Millisecond):
	}

	crossRelease, err := component.Acquire(context.Background(), LockTypeDB, crossBucketKey)
	if err != nil {
		t.Fatalf("cross bucket acquire error = %v", err)
	}
	crossRelease()

	firstRelease()
	select {
	case <-unblocked:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("same bucket waiter should resume after release")
	}
}

func TestCoroutineLockComponentLocationLockOrder(t *testing.T) {
	component := &CoroutineLockComponent{}
	component.Awake()

	order := make(chan int, 2)
	firstRelease, err := component.Acquire(context.Background(), LockTypeLocation, 777)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}

	go func() {
		release, acquireErr := component.Acquire(context.Background(), LockTypeLocation, 777)
		if acquireErr == nil {
			order <- 2
			release()
		}
	}()

	order <- 1
	time.Sleep(20 * time.Millisecond)
	firstRelease()

	first := <-order
	second := <-order
	if first != 1 || second != 2 {
		t.Fatalf("order = [%d %d], want [1 2]", first, second)
	}
}
