package actorlocation

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
)

func TestLocationOneTypeAddGetRemove(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, coroutinelock.New())
	first := actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}
	second := actor.ActorID{ProcessID: 4, FiberID: 5, InstanceID: 6}

	if err := lot.Add(100, first); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	got, err := lot.Get(100)
	if err != nil || got != first {
		t.Fatalf("Get got=%+v err=%v", got, err)
	}

	if err := lot.Add(100, second); err != nil {
		t.Fatalf("Add override error = %v", err)
	}
	got, err = lot.Get(100)
	if err != nil || got != second {
		t.Fatalf("Get override got=%+v err=%v", got, err)
	}

	if err := lot.Remove(100); err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	got, err = lot.Get(100)
	if err != nil {
		t.Fatalf("Get after remove err = %v", err)
	}
	if got.IsValid() {
		t.Fatalf("Get after remove should return zero actor, got=%+v", got)
	}
}

func TestLocationOneTypeRejectsOperationsAfterClose(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, coroutinelock.New())
	lot.Close()
	actorID := actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}

	if err := lot.Add(1, actorID); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("Add after close = %v, want %v", err, ErrLocationClosed)
	}
	if _, err := lot.Get(1); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("Get after close = %v, want %v", err, ErrLocationClosed)
	}
	if _, err := lot.TryGet(1); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("TryGet after close = %v, want %v", err, ErrLocationClosed)
	}
	if err := lot.Lock(1, actorID, 0); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("Lock after close = %v, want %v", err, ErrLocationClosed)
	}
	if err := lot.Unlock(1, actorID, actorID); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("Unlock after close = %v, want %v", err, ErrLocationClosed)
	}
	if err := lot.Remove(1); !errors.Is(err, ErrLocationClosed) {
		t.Fatalf("Remove after close = %v, want %v", err, ErrLocationClosed)
	}
}

func TestLocationOneTypeRejectsZeroKeyReads(t *testing.T) {
	lot := NewLocationOneType(LocationTypeUnit, nil)

	if _, err := lot.Get(0); !errors.Is(err, ErrZeroLocationKey) {
		t.Fatalf("Get zero key error = %v, want %v", err, ErrZeroLocationKey)
	}
	if _, err := lot.TryGet(0); !errors.Is(err, ErrZeroLocationKey) {
		t.Fatalf("TryGet zero key error = %v, want %v", err, ErrZeroLocationKey)
	}
}
