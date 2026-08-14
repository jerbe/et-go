package crontab

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

type stubHandler struct {
	calls int32
	err   error
	panic bool
	scene *ecs.Scene
}

func (h *stubHandler) Handle(scene *ecs.Scene, task *CrontabTask) error {
	if h.panic {
		panic("boom")
	}
	atomic.AddInt32(&h.calls, 1)
	h.scene = scene
	return h.err
}

func TestCrontabComponentBasics(t *testing.T) {
	handler := &stubHandler{}
	RegisterCrontabHandler(1, handler)
	defer RegisterCrontabHandler(1, nil)

	scene := ecs.NewScene(ecs.SceneTypeMain, 1, "main")
	comp := &CrontabComponent{}
	scene.AddComponent(comp)
	comp.SetNowFunc(func() time.Time {
		return time.Date(2026, 3, 21, 10, 5, 0, 0, time.UTC)
	})
	if err := comp.AddTask(&CrontabTask{Name: "task", CronExpression: "* * * * *", InvokeType: 1}); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	if err := comp.AddTask(&CrontabTask{Name: "task", CronExpression: "* * * * *", InvokeType: 1}); !errors.Is(err, ErrTaskAlreadyExists) {
		t.Fatalf("expected ErrTaskAlreadyExists, got %v", err)
	}

	comp.Update()
	if atomic.LoadInt32(&handler.calls) != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if handler.scene != scene {
		t.Fatalf("handler scene mismatch")
	}
	task, ok := comp.GetTask("task")
	if !ok || task.LastRunTime == nil {
		t.Fatalf("task should have LastRunTime set")
	}

	if err := comp.RemoveTask("task"); err != nil {
		t.Fatalf("RemoveTask failed: %v", err)
	}
	if err := comp.RemoveTask("task"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestCrontabComponentAwakeDoesNotResetMinute(t *testing.T) {
	comp := &CrontabComponent{}
	comp.Awake()
	comp.lastMinute = 27
	comp.Awake()
	if comp.lastMinute != 27 {
		t.Fatalf("lastMinute after repeated Awake = %d, want 27", comp.lastMinute)
	}
}

func TestCrontabComponentHandlerPanic(t *testing.T) {
	handler := &stubHandler{panic: true}
	RegisterCrontabHandler(2, handler)
	defer RegisterCrontabHandler(2, nil)

	scene := ecs.NewScene(ecs.SceneTypeMain, 1, "main")
	comp := &CrontabComponent{}
	scene.AddComponent(comp)
	comp.SetNowFunc(func() time.Time {
		return time.Date(2026, 3, 21, 11, 15, 0, 0, time.UTC)
	})
	if err := comp.AddTask(&CrontabTask{Name: "panic", CronExpression: "* * * * *", InvokeType: 2}); err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}
	comp.Update()
	if atomic.LoadInt32(&handler.calls) != 0 {
		t.Fatalf("panic handler should not increment calls")
	}
}

func TestCrontabComponentOnDestroyResetsRunning(t *testing.T) {
	comp := &CrontabComponent{}
	comp.Awake()
	task := &CrontabTask{Name: "x", CronExpression: "* * * * *", InvokeType: 1, IsRunning: true}
	if err := comp.AddTask(task); err != nil {
		t.Fatalf("AddTask err = %v", err)
	}
	comp.OnDestroy()
	if task.IsRunning {
		t.Fatal("task should be reset on destroy")
	}
	if len(comp.tasks) != 0 {
		t.Fatal("tasks should be cleared")
	}
	comp.Awake()
	if err := comp.AddTask(&CrontabTask{Name: "reopen", CronExpression: "* * * * *"}); !errors.Is(err, ErrComponentClosed) {
		t.Fatalf("AddTask after repeated Awake = %v, want %v", err, ErrComponentClosed)
	}
}
