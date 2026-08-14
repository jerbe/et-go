package timer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestTimerComponentOnce(t *testing.T) {
	component := &TimerComponent{}
	done := make(chan struct{})

	component.AddTimer(10*time.Millisecond, func() {
		close(done)
	})

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timer did not fire")
	}
}

func TestTimerComponentRepeatingAndRemove(t *testing.T) {
	component := &TimerComponent{}
	var count atomic.Int32
	id := component.AddRepeatingTimer(10*time.Millisecond, func() {
		count.Add(1)
	})

	time.Sleep(60 * time.Millisecond)
	component.RemoveTimer(id)

	got := count.Load()
	if got < 2 {
		t.Fatalf("repeat count = %d, want >=2", got)
	}
	prev := got
	time.Sleep(40 * time.Millisecond)
	if got2 := count.Load(); got2 > prev+1 {
		t.Fatalf("repeat after remove count increased to %d", got2)
	}
}

func TestTimerWaitAsync(t *testing.T) {
	component := &TimerComponent{}
	ch := component.WaitAsync(10 * time.Millisecond)

	select {
	case <-ch:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("WaitAsync channel not closed")
	}
}

func TestTimerWaitAsyncClosesOnDestroy(t *testing.T) {
	component := &TimerComponent{}
	ch := component.WaitAsync(time.Hour)
	component.OnDestroy()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("WaitAsync channel should close when timer component is destroyed")
	}
}

func TestTimerDoesNotReopenAfterDestroy(t *testing.T) {
	component := &TimerComponent{}
	component.OnDestroy()

	if id := component.AddTimer(time.Millisecond, func() {}); id != 0 {
		t.Fatalf("AddTimer after destroy = %d, want 0", id)
	}
	if id := component.AddRepeatingTimer(time.Millisecond, func() {}); id != 0 {
		t.Fatalf("AddRepeatingTimer after destroy = %d, want 0", id)
	}
}

func TestTimerStopHandleConcurrent(t *testing.T) {
	handle := &timerHandle{
		stopCh: make(chan struct{}),
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			(&TimerComponent{}).stopHandle(handle)
		}()
	}
	wg.Wait()

	select {
	case <-handle.stopCh:
	default:
		t.Fatal("stop channel should be closed")
	}
}

func TestTimerCallbackUsesFiberLifecycle(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()
	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	current := manager.Create(ecs.SceneTypeMain, 1, 1, nil)
	if current == nil {
		t.Fatal("fiber should be created")
	}
	component := &TimerComponent{}
	current.Root().AddComponent(component)

	done := make(chan struct{})
	if id := component.AddTimer(10*time.Millisecond, func() {
		close(done)
	}); id == 0 {
		t.Fatal("timer id should be non-zero")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fiber timer callback did not run")
	}
}
