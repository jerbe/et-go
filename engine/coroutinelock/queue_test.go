package coroutinelock

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLockQueueFIFO(t *testing.T) {
	queue := newLockQueue()
	firstRelease, err := queue.acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire error: %v", err)
	}

	orderCh := make(chan int, 2)
	errCh := make(chan error, 2)
	startWaiter := func(waiterID int) {
		go func() {
			release, acquireErr := queue.acquire(context.Background())
			if acquireErr != nil {
				errCh <- acquireErr
				return
			}
			orderCh <- waiterID
			release()
		}()
	}

	startWaiter(2)
	waitForCondition(t, time.Second, func() bool { return queue.waiterCount() == 1 })
	startWaiter(3)
	waitForCondition(t, time.Second, func() bool { return queue.waiterCount() == 2 })

	firstRelease()

	first := waitOrder(t, orderCh, "first waiter")
	second := waitOrder(t, orderCh, "second waiter")
	if first != 2 || second != 3 {
		t.Fatalf("wake order = [%d %d], want [2 3]", first, second)
	}

	select {
	case acquireErr := <-errCh:
		t.Fatalf("unexpected waiter error: %v", acquireErr)
	default:
	}

	waitForCondition(t, time.Second, queue.isEmpty)
}

func TestLockQueueContextCancelDoesNotBlockNextWaiter(t *testing.T) {
	queue := newLockQueue()
	firstRelease, err := queue.acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire error: %v", err)
	}

	timeoutErrCh := make(chan error, 1)
	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		t.Cleanup(cancel)
		go func() {
			release, acquireErr := queue.acquire(ctx)
			if acquireErr == nil {
				release()
			}
			timeoutErrCh <- acquireErr
		}()
	}()

	waitForCondition(t, time.Second, func() bool { return queue.waiterCount() == 1 })

	secondDone := make(chan struct{}, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		release, acquireErr := queue.acquire(ctx)
		if acquireErr == nil {
			release()
			close(secondDone)
		}
	}()

	waitForCondition(t, time.Second, func() bool { return queue.waiterCount() == 2 })

	timeoutErr := waitError(t, timeoutErrCh, "timeout waiter")
	if !errors.Is(timeoutErr, context.DeadlineExceeded) {
		t.Fatalf("timeout waiter error = %v, want deadline exceeded", timeoutErr)
	}

	firstRelease()
	waitForChannelClose(t, secondDone, "second waiter done")
	waitForCondition(t, time.Second, queue.isEmpty)
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition wait timeout")
}

func waitOrder(t *testing.T, ch <-chan int, label string) int {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatalf("%s timeout", label)
		return 0
	}
}

func waitError(t *testing.T, ch <-chan error, label string) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(time.Second):
		t.Fatalf("%s timeout", label)
		return nil
	}
}

func waitForChannelClose(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("%s timeout", label)
	}
}
