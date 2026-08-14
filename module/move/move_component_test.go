package move

import (
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

type fakeUnit struct {
	ecs.Entity
	position etmath.Vector3
	rotation etmath.Quaternion
}

func newFakeUnit() *fakeUnit {
	return &fakeUnit{
		Entity:   *ecs.NewEntity(),
		rotation: etmath.QuaternionIdentity,
	}
}

func (u *fakeUnit) Position() etmath.Vector3           { return u.position }
func (u *fakeUnit) SetPosition(pos etmath.Vector3)     { u.position = pos }
func (u *fakeUnit) Rotation() etmath.Quaternion       { return u.rotation }
func (u *fakeUnit) SetRotation(rot etmath.Quaternion) { u.rotation = rot }

func TestMoveToAsyncAndUpdate(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	unit := newFakeUnit()
	scene.AddChildWithID(1, &unit.Entity)

	component := NewMoveComponent()
	unit.AddComponent(component)
	component.Bind(unit)

	var now int64
	component.nowFunc = func() time.Time { return time.UnixMilli(now) }

	if err := component.beginMove([]etmath.Vector3{
		{X: 0, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 0},
	}, 5, 500); err != nil {
		t.Fatalf("beginMove error = %v", err)
	}
	done := component.moveDone

	now = 1000
	component.Update()
	if got := unit.Position().X; got < 4.9 || got > 5.1 {
		t.Fatalf("position x = %v, want about 5", got)
	}

	now = 2000
	component.Update()
	if err := <-done; err != nil {
		t.Fatalf("MoveToAsync error = %v", err)
	}
	if !component.IsArrived() {
		t.Fatal("move should be finished")
	}
	if unit.Position() != (etmath.Vector3{X: 10, Y: 0, Z: 0}) {
		t.Fatalf("final position = %v, want (10,0,0)", unit.Position())
	}
}

func TestChangeSpeedStopAndFlashTo(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	unit := newFakeUnit()
	scene.AddChildWithID(1, &unit.Entity)

	component := NewMoveComponent()
	unit.AddComponent(component)
	component.Bind(unit)

	var now int64
	component.nowFunc = func() time.Time { return time.UnixMilli(now) }

	if err := component.beginMove([]etmath.Vector3{
		{X: 0, Y: 0, Z: 0},
		{X: 10, Y: 0, Z: 0},
	}, 5, 0); err != nil {
		t.Fatalf("beginMove error = %v", err)
	}
	done := component.moveDone

	now = 500
	component.Update()
	if !component.ChangeSpeed(10) {
		t.Fatal("ChangeSpeed should succeed while moving")
	}
	if component.NeedTime >= 1500 {
		t.Fatalf("NeedTime = %d, want shorter after speeding up", component.NeedTime)
	}

	component.Stop(false)
	if err := <-done; err != ErrMoveCanceled {
		t.Fatalf("stop result = %v, want %v", err, ErrMoveCanceled)
	}

	if !component.FlashTo(etmath.Vector3{X: 3, Y: 0, Z: 4}) {
		t.Fatal("FlashTo should succeed")
	}
	if unit.Position() != (etmath.Vector3{X: 3, Y: 0, Z: 4}) {
		t.Fatalf("flash position = %v, want (3,0,4)", unit.Position())
	}
}

func TestMoveToAsyncValidation(t *testing.T) {
	component := NewMoveComponent()
	if err := component.MoveToAsync(nil, 1, 0); err != ErrInvalidPath {
		t.Fatalf("nil path error = %v, want %v", err, ErrInvalidPath)
	}
	if err := component.MoveToAsync([]etmath.Vector3{{}, {}}, 0, 0); err != ErrInvalidSpeed {
		t.Fatalf("invalid speed error = %v, want %v", err, ErrInvalidSpeed)
	}
}

func TestMoveDestroyCancelsPendingWaiter(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	unit := newFakeUnit()
	scene.AddChildWithID(1, &unit.Entity)

	component := NewMoveComponent()
	unit.AddComponent(component)
	component.Bind(unit)
	if err := component.beginMove([]etmath.Vector3{{}, {X: 10}}, 1, 0); err != nil {
		t.Fatalf("beginMove error = %v", err)
	}
	done := component.moveDone

	component.OnDestroy()

	select {
	case err := <-done:
		if err != ErrMoveCanceled {
			t.Fatalf("destroy result = %v, want %v", err, ErrMoveCanceled)
		}
	case <-time.After(time.Second):
		t.Fatal("destroy should release pending move waiter")
	}
}

func TestMoveDoesNotReopenAfterDestroy(t *testing.T) {
	component := NewMoveComponent()
	component.OnDestroy()

	if _, err := component.StartMove([]etmath.Vector3{{}, {X: 1}}, 1, 0); err != ErrMoveCanceled {
		t.Fatalf("StartMove after destroy = %v, want %v", err, ErrMoveCanceled)
	}
	if component.FlashTo(etmath.Vector3{X: 1}) {
		t.Fatal("FlashTo after destroy should fail")
	}
}
