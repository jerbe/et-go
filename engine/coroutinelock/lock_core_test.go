package coroutinelock

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLockTypeIsolation(t *testing.T) {
	lock := New()

	loginRelease, err := lock.Acquire(context.Background(), LockTypeLogin, 100)
	if err != nil {
		t.Fatalf("first acquire error: %v", err)
	}

	dbDone := make(chan struct{}, 1)
	go func() {
		release, acquireErr := lock.Acquire(context.Background(), LockTypeDB, 100)
		if acquireErr == nil {
			release()
			close(dbDone)
		}
	}()

	select {
	case <-dbDone:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("different lockType should not block")
	}

	loginBlockedDone := make(chan struct{}, 1)
	go func() {
		release, acquireErr := lock.Acquire(context.Background(), LockTypeLogin, 100)
		if acquireErr == nil {
			release()
			close(loginBlockedDone)
		}
	}()

	select {
	case <-loginBlockedDone:
		t.Fatal("same lockType+key should block before release")
	case <-time.After(40 * time.Millisecond):
	}

	loginRelease()
	waitForChannelClose(t, loginBlockedDone, "login waiter done")
}

func TestLockAcquireByNameCompatibility(t *testing.T) {
	lock := New()

	firstRelease, err := lock.AcquireByName(context.Background(), "account:1")
	if err != nil {
		t.Fatalf("first acquire by name error: %v", err)
	}

	secondDone := make(chan struct{}, 1)
	go func() {
		release, acquireErr := lock.AcquireByName(context.Background(), "account:1")
		if acquireErr == nil {
			release()
			close(secondDone)
		}
	}()

	select {
	case <-secondDone:
		t.Fatal("same name key should block before release")
	case <-time.After(40 * time.Millisecond):
	}

	firstRelease()
	waitForChannelClose(t, secondDone, "name waiter done")
}

func TestLockAcquireTimeoutError(t *testing.T) {
	lock := New()
	firstRelease, err := lock.Acquire(context.Background(), LockTypeLocation, 8)
	if err != nil {
		t.Fatalf("first acquire error: %v", err)
	}
	defer firstRelease()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = lock.Acquire(ctx, LockTypeLocation, 8)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrLockTimeout) {
		t.Fatalf("error = %v, want contain ErrLockTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want contain DeadlineExceeded", err)
	}
}

func TestLockWarnTimeoutLogging(t *testing.T) {
	var buffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buffer, nil))
	lock := New(WithWarnTimeout(10*time.Millisecond), WithLogger(logger))

	release, err := lock.Acquire(context.Background(), LockTypeLogin, 42)
	if err != nil {
		t.Fatalf("acquire error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	release()

	output := buffer.String()
	if !strings.Contains(output, "协程锁持有时间过长") {
		t.Fatalf("warn log not found: %q", output)
	}
	if !strings.Contains(output, "lockType=9001") {
		t.Fatalf("warn log should contain lockType: %q", output)
	}
}
