package actorlocation

import (
	"context"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestLocationFiberEndToEnd(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	locationFiber := manager.Create(ecs.SceneTypeLocation, 1, 1, nil)
	if locationFiber == nil {
		t.Fatal("location fiber create failed")
	}

	if _, ok := locationFiber.Root().GetComponent("LocationManagerComponent"); !ok {
		t.Fatal("location fiber should register LocationManagerComponent")
	}
	if _, ok := locationFiber.Root().GetComponent("CoroutineLockComponent"); !ok {
		t.Fatal("location fiber should register CoroutineLockComponent")
	}
	if _, ok := locationFiber.Root().GetComponent("MailBox"); !ok {
		t.Fatal("location fiber should register MailBox")
	}

	innerSender := actor.NewProcessInnerSender(1, manager, actor.NewRpcManager())
	innerSender.Awake()

	messageSender := actor.NewMessageSender(1, innerSender, nil)
	proxy := &LocationProxyComponent{}
	proxy.SetCaller(messageSender)
	proxy.SetLocationActor(actor.ActorID{
		ProcessID:  1,
		FiberID:    locationFiber.ID(),
		InstanceID: locationFiber.Root().InstanceID(),
	})

	oldActor := actor.ActorID{ProcessID: 1, FiberID: 200, InstanceID: 300}
	newActor := actor.ActorID{ProcessID: 1, FiberID: 201, InstanceID: 301}

	if err := proxy.Add(int(LocationTypeUnit), 1001, oldActor); err != nil {
		t.Fatalf("Add error = %v", err)
	}

	got, err := proxy.Get(int(LocationTypeUnit), 1001)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if got != oldActor {
		t.Fatalf("Get actor = %+v, want %+v", got, oldActor)
	}

	if err := proxy.Lock(int(LocationTypeUnit), 1001, oldActor, 0); err != nil {
		t.Fatalf("Lock error = %v", err)
	}

	blocked := make(chan actor.ActorID, 1)
	errs := make(chan error, 1)
	go func() {
		actorID, getErr := proxy.Get(int(LocationTypeUnit), 1001)
		if getErr != nil {
			errs <- getErr
			return
		}
		blocked <- actorID
	}()

	select {
	case <-blocked:
		t.Fatal("Get should block while lock is held")
	case err := <-errs:
		t.Fatalf("blocked get error = %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	if err := proxy.Unlock(int(LocationTypeUnit), 1001, oldActor, newActor); err != nil {
		t.Fatalf("Unlock error = %v", err)
	}

	select {
	case got := <-blocked:
		if got != newActor {
			t.Fatalf("unblocked Get actor = %+v, want %+v", got, newActor)
		}
	case err := <-errs:
		t.Fatalf("unblocked get error = %v", err)
	case <-time.After(time.Second):
		t.Fatal("Get should resume after unlock")
	}

	if err := proxy.Remove(int(LocationTypeUnit), 1001); err != nil {
		t.Fatalf("Remove error = %v", err)
	}

	got, err = proxy.Get(int(LocationTypeUnit), 1001)
	if err != nil {
		t.Fatalf("Get after remove error = %v", err)
	}
	if got.IsValid() {
		t.Fatalf("Get after remove should return zero actor, got=%+v", got)
	}
}
