package numeric

import (
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

type testWatcher struct {
	calls []NumericChange
}

func (w *testWatcher) Run(unit any, numType int, old, new_ int64) {
	w.calls = append(w.calls, NumericChange{
		Unit:     unit,
		Type:     numType,
		OldValue: old,
		NewValue: new_,
	})
}

type panicWatcher struct{}

func (panicWatcher) Run(any, int, int64, int64) {
	panic("watcher failure")
}

func TestNumericConstants(t *testing.T) {
	cases := map[string]int{
		"Speed":         Speed,
		"SpeedBase":     SpeedBase,
		"SpeedAdd":      SpeedAdd,
		"SpeedPct":      SpeedPct,
		"SpeedFinalAdd": SpeedFinalAdd,
		"SpeedFinalPct": SpeedFinalPct,
		"Hp":            Hp,
		"AOI":           AOI,
	}

	expected := map[string]int{
		"Speed":         1000,
		"SpeedBase":     10001,
		"SpeedAdd":      10002,
		"SpeedPct":      10003,
		"SpeedFinalAdd": 10004,
		"SpeedFinalPct": 10005,
		"Hp":            1001,
		"AOI":           1003,
	}

	for name, value := range cases {
		if value != expected[name] {
			t.Fatalf("%s = %d, want %d", name, value, expected[name])
		}
	}
	if NumericTypeMax != 10000 {
		t.Fatalf("NumericTypeMax = %d, want 10000", NumericTypeMax)
	}
}

func TestNumericSetFloatAndRecalculate(t *testing.T) {
	component := NewNumericComponent()
	component.SetFloat(Speed, 6.0)

	if got := component.Get(Speed); got != 60000 {
		t.Fatalf("Speed raw = %d, want 60000", got)
	}
	if got := component.GetAsFloat(Speed); got != 6.0 {
		t.Fatalf("Speed float = %v, want 6.0", got)
	}

	component.Set(SpeedBase, 60000)
	component.Set(SpeedPct, 50)
	if got := component.Get(Speed); got != 90000 {
		t.Fatalf("Speed recalculated = %d, want 90000", got)
	}
}

func TestNumericDoesNotReopenAfterDestroy(t *testing.T) {
	component := NewNumericComponent()
	component.OnDestroy()
	component.Set(Speed, 100)

	if got := component.Get(Speed); got != 0 {
		t.Fatalf("value after destroy = %d, want 0", got)
	}
	if _, err := component.Serialize(); err != ErrNumericClosed {
		t.Fatalf("Serialize after destroy = %v, want %v", err, ErrNumericClosed)
	}
	if err := component.Deserialize(nil); err != ErrNumericClosed {
		t.Fatalf("Deserialize after destroy = %v, want %v", err, ErrNumericClosed)
	}
}

func TestNumericWatcherAndEvent(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	entity := ecs.NewEntity()
	scene.AddChildWithID(100, entity)

	component := NewNumericComponent()
	entity.AddComponent(component)

	watcher := &testWatcher{}
	manager := NewWatcherManager()
	manager.Register(Speed, watcher)
	component.watcherManager = manager

	events := make(chan *NumericChange, 1)
	cancel := scene.EventBus().Subscribe(EventNumericChange, func(args any) {
		if change, ok := args.(*NumericChange); ok {
			events <- change
		}
	})
	defer cancel()

	component.SetFloat(Speed, 6.0)

	if len(watcher.calls) != 1 {
		t.Fatalf("watcher calls = %d, want 1", len(watcher.calls))
	}
	select {
	case evt := <-events:
		if evt.Type != Speed || evt.NewValue != 60000 {
			t.Fatalf("unexpected event: %+v", evt)
		}
	default:
		t.Fatal("numeric change event should be published")
	}
}

func TestNumericWatcherPanicDoesNotStopOtherWatchers(t *testing.T) {
	manager := NewWatcherManager()
	manager.Register(Speed, panicWatcher{})
	observer := &testWatcher{}
	manager.Register(Speed, observer)

	manager.Notify(nil, Speed, 1, 2)

	if len(observer.calls) != 1 {
		t.Fatalf("non-panicking watcher calls = %d, want 1", len(observer.calls))
	}
}

func TestNumericTransferRoundTrip(t *testing.T) {
	component := NewNumericComponent()
	component.Set(SpeedBase, 60000)
	component.Set(SpeedPct, 50)
	component.Set(AOI, 15000)

	data, err := component.Transfer()
	if err != nil {
		t.Fatalf("Transfer error = %v", err)
	}

	clone := NewNumericComponent()
	if err := clone.OnTransferIn(data); err != nil {
		t.Fatalf("OnTransferIn error = %v", err)
	}
	if clone.Get(Speed) != component.Get(Speed) {
		t.Fatalf("Speed = %d, want %d", clone.Get(Speed), component.Get(Speed))
	}
	if clone.Get(AOI) != 15000 {
		t.Fatalf("AOI = %d, want 15000", clone.Get(AOI))
	}
}
