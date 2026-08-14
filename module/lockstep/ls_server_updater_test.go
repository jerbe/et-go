package lockstep

import (
	"testing"
	"time"
)

func TestLSServerUpdater(t *testing.T) {
	room := NewLockstepRoom([]int64{1, 2})
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	start := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	room.StartTime = start.UnixMilli()
	updater.SetNowFunc(func() time.Time {
		return start.Add(2 * time.Duration(UpdateIntervalMillis) * time.Millisecond)
	})
	updater.AddInput(1, 1, &LSInput{})
	updater.Update()
	if room.FrameBuffer.MaxFrame() != 2 {
		t.Fatalf("frame = %d", room.FrameBuffer.MaxFrame())
	}
	if frame2, ok := room.FrameBuffer.GetFrameInputs(2); !ok || len(frame2.Inputs) != 2 {
		t.Fatal("frame inputs missing or not padded")
	}
	if room.AuthorityFrame != 2 {
		t.Fatalf("authority frame = %d", room.AuthorityFrame)
	}
	if len(room.Replay.GetFrameInputsRange(0, 2)) == 0 {
		t.Fatal("replay should record frames")
	}
}

func TestLSServerUpdaterAdvancesLSWorld(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	start := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	room.StartTime = start.UnixMilli()
	updater.SetNowFunc(func() time.Time {
		return start.Add(time.Duration(UpdateIntervalMillis) * time.Millisecond)
	})
	if err := updater.AddInput(1, 1, &LSInput{
		V: TSVector2{X: int64(uint64(1) << trueSyncFixedFractionBits)},
	}); err != nil {
		t.Fatalf("AddInput error = %v", err)
	}
	updater.Update()

	unit, ok := room.World.Unit(1)
	if !ok {
		t.Fatal("world unit missing")
	}
	if room.AuthorityFrame != 1 || room.World.Frame != 1 || updater.Frame() != 1 {
		t.Fatalf("frames = authority=%d world=%d updater=%d, want 1/1/1",
			room.AuthorityFrame, room.World.Frame, updater.Frame())
	}
	if unit.Position.X <= 20.29 || unit.Position.X >= 20.31 {
		t.Fatalf("unit position = %+v, want x approximately 20.3", unit.Position)
	}
	if err := updater.LastUpdateError(); err != nil {
		t.Fatalf("LastUpdateError = %v", err)
	}
}

func TestLSServerUpdaterSnapshot(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	updater.SetSnapshotProvider(func(*LockstepRoom) ([]byte, error) {
		return []byte("room-state"), nil
	})
	room.StartTime = time.Now().Add(-time.Duration(SaveLSWorldFrameCount*UpdateIntervalMillis) * time.Millisecond).UnixMilli()
	updater.Update()
	if _, _, ok := room.FrameBuffer.GetNearestSnapshot(SaveLSWorldFrameCount); !ok {
		t.Fatal("snapshot should be saved")
	}
}

func TestLSServerUpdaterStoresInitialRecoverySnapshot(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	updater.SetSnapshotProvider(func(*LockstepRoom) ([]byte, error) {
		return []byte("initial-room-state"), nil
	})
	start := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	room.StartTime = start.UnixMilli()
	updater.SetNowFunc(func() time.Time {
		return start.Add(UpdateIntervalMillis * time.Millisecond)
	})

	updater.Update()

	frame, snapshot, ok := room.FrameBuffer.GetNearestSnapshot(1)
	if !ok || frame != 1 || string(snapshot) != "initial-room-state" {
		t.Fatalf("initial snapshot = frame=%d snapshot=%q ok=%v", frame, snapshot, ok)
	}
}

func TestLSServerUpdaterDoesNotCreatePlaceholderSnapshot(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	updater := NewLSServerUpdater(room.FrameBuffer)
	updater.BindRoom(room)
	room.StartTime = time.Now().Add(-time.Duration(SaveLSWorldFrameCount*UpdateIntervalMillis) * time.Millisecond).UnixMilli()
	updater.Update()
	if _, _, ok := room.FrameBuffer.GetNearestSnapshot(SaveLSWorldFrameCount); ok {
		t.Fatal("snapshot must not be synthesized without a provider")
	}
	if updater.LastSnapshotError() != ErrSnapshotProviderMissing {
		t.Fatalf("snapshot error = %v", updater.LastSnapshotError())
	}
}
