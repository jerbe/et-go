package fiber

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	etlog "github.com/jerbe/et-go/internal/log"
)

func newTestManager() *Manager {
	return NewManager(context.Background(), ecs.NewWorld(), etlog.New("error"))
}

func TestManagerSpec_FiberInitRouting(t *testing.T) {
	manager := newTestManager()
	sceneType := ecs.SceneType(990101)
	initCalled := make(chan *Fiber, 1)

	RegisterFiberInit(sceneType, func(f *Fiber) error {
		initCalled <- f
		return nil
	})

	fiber := manager.Create(sceneType, 1, 1, nil)
	if fiber == nil {
		t.Fatal("fiber should not be nil when init succeeds")
	}
	defer manager.Remove(fiber.ID())

	select {
	case got := <-initCalled:
		if got != fiber {
			t.Fatal("fiber init should receive the created fiber")
		}
		if got.Root().Fiber() != got {
			t.Fatal("fiber init should observe scene fiber injection")
		}
	case <-time.After(time.Second):
		t.Fatal("fiber init handler should be called")
	}
}

func TestManagerSpec_CreateConfiguredSetsSceneIdentityBeforeInit(t *testing.T) {
	manager := newTestManager()
	sceneType := ecs.SceneType(990103)
	observed := make(chan struct {
		id   int64
		name string
	}, 1)
	RegisterFiberInit(sceneType, func(f *Fiber) error {
		observed <- struct {
			id   int64
			name string
		}{id: f.Root().ID(), name: f.Root().Name()}
		return nil
	})
	defer RegisterFiberInit(sceneType, nil)

	created := manager.CreateConfigured(sceneType, 1, 1, 1234, "ConfiguredScene", nil)
	if created == nil {
		t.Fatal("configured fiber should be created")
	}
	defer manager.Remove(created.ID())

	select {
	case got := <-observed:
		if got.id != 1234 || got.name != "ConfiguredScene" {
			t.Fatalf("init observed scene identity = %+v, want id=1234 name=ConfiguredScene", got)
		}
	case <-time.After(time.Second):
		t.Fatal("fiber init did not run")
	}
}

func TestManagerSpec_CreateWithSetupRunsBeforeLoop(t *testing.T) {
	manager := newTestManager()
	defer manager.StopAll()

	sceneType := ecs.SceneType(990104)
	RegisterFiberInit(sceneType, func(f *Fiber) error {
		f.Root().AddComponent(&setupMarkerComponent{})
		return nil
	})
	defer RegisterFiberInit(sceneType, nil)

	setupStarted := false
	created := manager.CreateWithSetup(sceneType, 1, 1, func(f *Fiber) error {
		if f.started.Load() {
			t.Fatal("setup must run before Fiber loop starts")
		}
		setupStarted = true
		component, ok := f.Root().GetComponent("SetupMarker")
		if !ok || component == nil {
			return errors.New("fiber init component missing")
		}
		return nil
	}, nil)
	if created == nil {
		t.Fatal("fiber should be created")
	}
	if !setupStarted {
		t.Fatal("setup should run synchronously")
	}
}

func TestManagerSpec_FiberInitFailure(t *testing.T) {
	manager := newTestManager()
	sceneType := ecs.SceneType(990102)

	RegisterFiberInit(sceneType, func(_ *Fiber) error {
		return errors.New("init failed")
	})

	fiber := manager.Create(sceneType, 1, 1, nil)
	if fiber != nil {
		t.Fatal("fiber should be nil when init fails")
	}
	if manager.Count() != 0 {
		t.Fatalf("manager count = %d, want 0", manager.Count())
	}
}

type setupMarkerComponent struct {
	ecs.BaseComponent
}

func (c *setupMarkerComponent) Type() string { return "SetupMarker" }

func TestManagerSpec_ConcurrentCreateGetRemove(t *testing.T) {
	manager := newTestManager()
	const workers = 60

	var createWG sync.WaitGroup
	ids := make(chan int64, workers)
	for index := 0; index < workers; index++ {
		createWG.Add(1)
		go func(zone int) {
			defer createWG.Done()
			fiber := manager.Create(ecs.SceneTypeMain, zone+1, 1, nil)
			if fiber != nil {
				ids <- fiber.ID()
			}
		}(index)
	}
	createWG.Wait()
	close(ids)

	idList := make([]int64, 0, workers)
	for id := range ids {
		idList = append(idList, id)
	}
	if len(idList) == 0 {
		t.Fatal("expected non-empty fiber ids")
	}

	var opWG sync.WaitGroup
	for _, id := range idList {
		opWG.Add(1)
		go func(id int64) {
			defer opWG.Done()
			if _, ok := manager.Get(id); ok {
				manager.Remove(id)
			}
		}(id)
	}
	opWG.Wait()

	manager.StopAll()
	waitForCondition(t, time.Second, func() bool { return manager.Count() == 0 })
}

func TestManagerSpec_StopAllGraceful(t *testing.T) {
	manager := newTestManager()
	messageDone := make(chan struct{})
	frameTaskDone := make(chan struct{})
	var messageOnce sync.Once
	var frameTaskOnce sync.Once

	fiber := manager.Create(ecs.SceneTypeMain, 1, 1, func(_ *Fiber, _ Message) {
		messageOnce.Do(func() { close(messageDone) })
	})
	if fiber == nil {
		t.Fatal("fiber should not be nil")
	}

	fiber.AddFrameFinishTask(func() {
		frameTaskOnce.Do(func() { close(frameTaskDone) })
	})
	fiber.Send(Message{MsgID: 3001})

	manager.StopAll()

	select {
	case <-messageDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pending message should be handled before stop all returns")
	}
	select {
	case <-frameTaskDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pending frame finish task should run before stop all returns")
	}

	waitForCondition(t, time.Second, func() bool { return manager.Count() == 0 })
	if DefaultManager() == manager {
		t.Fatal("stopped manager should not remain the default manager")
	}
}

func TestManagerSpec_RejectsCreateAfterStopAll(t *testing.T) {
	manager := newTestManager()
	manager.StopAll()

	if created := manager.Create(ecs.SceneTypeMain, 1, 1, nil); created != nil {
		t.Fatal("Create after StopAll should return nil")
	}
}
