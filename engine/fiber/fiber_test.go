package fiber

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestNewInjectsFiberIntoScene(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)

	if f.Root().Fiber() != f {
		t.Fatal("scene fiber should be injected on Fiber.New")
	}
}

type testUpdateSystem struct {
	onUpdate func()
}

func (s *testUpdateSystem) Update() {
	if s.onUpdate != nil {
		s.onUpdate()
	}
}

type testLateUpdateSystem struct {
	onLateUpdate func()
}

func (s *testLateUpdateSystem) LateUpdate() {
	if s.onLateUpdate != nil {
		s.onLateUpdate()
	}
}

func TestFiberSpec_MainLoopOrder(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	defer f.Stop()

	var mu sync.Mutex
	order := make([]string, 0, 4)
	appendOrder := func(item string) {
		mu.Lock()
		order = append(order, item)
		mu.Unlock()
	}

	frameDone := make(chan struct{})
	var frameOnce sync.Once

	f.RegisterUpdate(&testUpdateSystem{
		onUpdate: func() { appendOrder("update") },
	})
	f.RegisterLateUpdate(&testLateUpdateSystem{
		onLateUpdate: func() { appendOrder("late_update") },
	})
	f.AddFrameFinishTask(func() {
		appendOrder("frame_finish")
		frameOnce.Do(func() {
			close(frameDone)
		})
	})

	f.Run(func(_ *Fiber, _ Message) {
		appendOrder("message")
	})
	f.Send(Message{MsgID: 1001})

	select {
	case <-frameDone:
	case <-time.After(2 * time.Second):
		t.Fatal("wait frame finish timeout")
	}

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()

	want := []string{"message", "update", "late_update", "frame_finish"}
	if len(got) < len(want) {
		t.Fatalf("order length = %d, want >= %d, got=%v", len(got), len(want), got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("order[%d] = %q, want %q (full=%v)", index, got[index], want[index], got)
		}
	}
}

func TestFiberSpec_UnregisterSystems(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	defer f.Stop()

	var updateCount atomic.Int64
	var lateCount atomic.Int64

	updateSys := &testUpdateSystem{
		onUpdate: func() { updateCount.Add(1) },
	}
	lateSys := &testLateUpdateSystem{
		onLateUpdate: func() { lateCount.Add(1) },
	}

	f.RegisterUpdate(updateSys)
	f.RegisterLateUpdate(lateSys)
	f.Run(nil)

	waitForCondition(t, time.Second, func() bool {
		return updateCount.Load() > 0 && lateCount.Load() > 0
	})

	f.UnregisterUpdate(updateSys)
	f.UnregisterLateUpdate(lateSys)

	updateBefore := updateCount.Load()
	lateBefore := lateCount.Load()
	time.Sleep(150 * time.Millisecond)

	updateAfter := updateCount.Load()
	lateAfter := lateCount.Load()
	if updateAfter != updateBefore {
		t.Fatalf("update count changed after unregister: before=%d after=%d", updateBefore, updateAfter)
	}
	if lateAfter != lateBefore {
		t.Fatalf("late update count changed after unregister: before=%d after=%d", lateBefore, lateAfter)
	}
}

func TestFiberSpec_GracefulStopRunsPendingMessageAndFrameTask(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)

	messageDone := make(chan struct{})
	var messageOnce sync.Once
	taskDone := make(chan struct{})
	var taskOnce sync.Once

	f.AddFrameFinishTask(func() {
		taskOnce.Do(func() { close(taskDone) })
	})
	f.Run(func(_ *Fiber, _ Message) {
		messageOnce.Do(func() { close(messageDone) })
	})

	f.Send(Message{MsgID: 2001})
	f.Stop()

	select {
	case <-messageDone:
	case <-time.After(2 * time.Second):
		t.Fatal("message should be handled before fiber stop")
	}
	select {
	case <-taskDone:
	case <-time.After(2 * time.Second):
		t.Fatal("frame finish task should run before fiber stop")
	}
	if !f.Wait(time.Second) {
		t.Fatal("fiber should stop within timeout")
	}
	if err := f.Send(Message{MsgID: 2002}); err != ErrFiberClosed {
		t.Fatalf("Send after Stop error = %v, want %v", err, ErrFiberClosed)
	}
	if _, err := f.Call(context.Background(), func() ([]byte, error) {
		return nil, nil
	}); err != ErrFiberClosed {
		t.Fatalf("Call after Stop error = %v, want %v", err, ErrFiberClosed)
	}
}

func TestFiberSpec_WaitFrameFinish(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	defer f.Stop()

	f.Run(nil)
	waiterA := f.WaitFrameFinish()
	waiterB := f.WaitFrameFinish()

	select {
	case <-waiterA:
	case <-time.After(2 * time.Second):
		t.Fatal("waiterA should close at frame finish")
	}
	select {
	case <-waiterB:
	case <-time.After(2 * time.Second):
		t.Fatal("waiterB should close at frame finish")
	}
}

func TestFiberSpec_WaitFrameFinishClosesWhenFiberStops(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	f.Run(nil)
	waiter := f.WaitFrameFinish()
	f.Stop()

	select {
	case <-waiter:
	case <-time.After(time.Second):
		t.Fatal("waiter should close when fiber stops")
	}
}

func TestFiberSpec_WaitFrameFinishClosesBeforeFiberStarts(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	waiter := f.WaitFrameFinish()

	select {
	case <-waiter:
	case <-time.After(time.Second):
		t.Fatal("waiter should close for an unstarted fiber")
	}
}

func TestFiberSpec_StopBeforeRunDisposesAndCannotRestart(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)

	f.Stop()

	if !f.Root().IsDisposed() {
		t.Fatal("stopping an unstarted fiber should dispose its root scene")
	}
	if err := f.Send(Message{MsgID: 1}); err != ErrFiberClosed {
		t.Fatalf("Send after stopping an unstarted fiber error = %v, want %v", err, ErrFiberClosed)
	}
	if !f.Wait(time.Second) {
		t.Fatal("stopping an unstarted fiber should close its done signal")
	}

	f.Run(nil)
	if f.started.Load() {
		t.Fatal("a stopped fiber must not be restarted")
	}
}

func TestFiberSpec_AddFrameFinishTaskRejectsAfterStop(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	f.Run(nil)
	f.Stop()

	if err := f.AddFrameFinishTask(func() {}); err != ErrFiberClosed {
		t.Fatalf("AddFrameFinishTask after stop error = %v, want %v", err, ErrFiberClosed)
	}
}

func TestFiberSpec_AddFrameFinishTaskRejectsNil(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	if err := f.AddFrameFinishTask(nil); err != ErrFrameTaskRequired {
		t.Fatalf("AddFrameFinishTask nil error = %v, want %v", err, ErrFrameTaskRequired)
	}
}

func TestFiberSpec_AddFrameFinishTaskRejectsNilFiber(t *testing.T) {
	var f *Fiber
	if err := f.AddFrameFinishTask(func() {}); err != ErrFiberClosed {
		t.Fatalf("AddFrameFinishTask on nil fiber error = %v, want %v", err, ErrFiberClosed)
	}
}

func TestFiberSpec_CallConvertsTaskPanicToError(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	f.Run(nil)
	defer f.Stop()

	_, err := f.Call(context.Background(), func() ([]byte, error) {
		panic("task failure")
	})
	if !errors.Is(err, ErrFiberTaskPanic) {
		t.Fatalf("Call panic error = %v, want %v", err, ErrFiberTaskPanic)
	}
}

func TestFiberSpec_CanceledCallDoesNotExecuteTask(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	f.Run(nil)
	defer f.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var executed atomic.Bool
	_, err := f.Call(ctx, func() ([]byte, error) {
		executed.Store(true)
		return nil, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call error = %v, want context canceled", err)
	}
	if executed.Load() {
		t.Fatal("a canceled call should not execute its task")
	}
}

func TestFiberSpec_MessageHandlerPanicDoesNotBreakLoop(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	defer f.Stop()

	handled := make(chan struct{})
	f.Run(func(_ *Fiber, msg Message) {
		if msg.MsgID == 1 {
			panic("handler failure")
		}
		if msg.MsgID == 2 {
			close(handled)
		}
	})

	if err := f.Send(Message{MsgID: 1}); err != nil {
		t.Fatalf("send first message error = %v", err)
	}
	if err := f.Send(Message{MsgID: 2}); err != nil {
		t.Fatalf("send second message error = %v", err)
	}

	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("fiber should continue after message handler panic")
	}
}

func TestFiberSpec_RegisterUpdateIsIdempotent(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	system := &testUpdateSystem{}
	f.RegisterUpdate(system)
	f.RegisterUpdate(system)

	f.mu.RLock()
	count := len(f.updateSystems)
	f.mu.RUnlock()
	if count != 1 {
		t.Fatalf("update system count = %d, want 1", count)
	}
}

func TestFiberSpec_RegisterLateUpdateIsIdempotent(t *testing.T) {
	f := New(context.Background(), ecs.SceneTypeMain, 1, 1)
	system := &testLateUpdateSystem{}
	f.RegisterLateUpdate(system)
	f.RegisterLateUpdate(system)

	f.mu.RLock()
	count := len(f.lateUpdateSystems)
	f.mu.RUnlock()
	if count != 1 {
		t.Fatalf("late update system count = %d, want 1", count)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("wait condition timeout")
}
