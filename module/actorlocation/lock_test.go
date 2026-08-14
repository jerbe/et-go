package actorlocation

import (
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
)

func TestLockUnlockUpdatesLocation(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, coroutinelock.New())
	oldActor := actor.ActorID{ProcessID: 1, FiberID: 10, InstanceID: 100}
	newActor := actor.ActorID{ProcessID: 1, FiberID: 11, InstanceID: 101}

	if err := lot.Add(1, oldActor); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if err := lot.Lock(1, oldActor, 0); err != nil {
		t.Fatalf("Lock error = %v", err)
	}

	unlocked := make(chan struct{})
	go func() {
		lot.Unlock(1, oldActor, newActor)
		close(unlocked)
	}()

	select {
	case <-unlocked:
	case <-time.After(time.Second):
		t.Fatal("Unlock timeout")
	}

	got, err := lot.Get(1)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if got != newActor {
		t.Fatalf("location after unlock = %+v, want %+v", got, newActor)
	}
}

func TestLockBlocksGetAndTimeoutUnlock(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, coroutinelock.New())
	current := actor.ActorID{ProcessID: 1, FiberID: 10, InstanceID: 100}

	if err := lot.Add(2, current); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if err := lot.Lock(2, current, 50); err != nil {
		t.Fatalf("Lock error = %v", err)
	}

	done := make(chan actor.ActorID, 1)
	go func() {
		actorID, _ := lot.Get(2)
		done <- actorID
	}()

	select {
	case <-done:
		t.Fatal("Get should be blocked while lock is held")
	case <-time.After(20 * time.Millisecond):
	}

	select {
	case got := <-done:
		if got != current {
			t.Fatalf("Get after timeout = %+v, want %+v", got, current)
		}
	case <-time.After(time.Second):
		t.Fatal("Get should resume after lock timeout")
	}
}

func TestLockWrongOldActorDoesNotUnlock(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, coroutinelock.New())
	current := actor.ActorID{ProcessID: 1, FiberID: 10, InstanceID: 100}

	if err := lot.Add(3, current); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if err := lot.Lock(3, current, 0); err != nil {
		t.Fatalf("Lock error = %v", err)
	}

	lot.Unlock(3, actor.ActorID{ProcessID: 9, FiberID: 9, InstanceID: 9}, current)

	blocked := make(chan struct{})
	go func() {
		_, _ = lot.Get(3)
		close(blocked)
	}()

	select {
	case <-blocked:
		t.Fatal("Get should remain blocked when unlock old actor mismatches")
	case <-time.After(30 * time.Millisecond):
	}

	lot.Unlock(3, current, current)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("Get should continue after correct unlock")
	}
}

func TestLockInfoDoesNotReopenTimerAfterDispose(t *testing.T) {
	info := NewLockInfo(actor.ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1}, func() {})
	info.Dispose()

	timer := time.NewTimer(time.Hour)
	info.SetTimer(timer)
	if timer.Stop() {
		t.Fatal("timer should be stopped when assigned to disposed lock info")
	}
}
