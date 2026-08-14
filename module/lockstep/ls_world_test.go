package lockstep

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

func TestLSWorldMovementIsDeterministicAcrossPlayerOrder(t *testing.T) {
	first := NewLSWorld(int(ecs.SceneTypeLockStep))
	second := NewLSWorld(int(ecs.SceneTypeLockStep))
	if err := first.InitializePlayers([]int64{2, 1}); err != nil {
		t.Fatalf("initialize first world: %v", err)
	}
	if err := second.InitializePlayers([]int64{1, 2}); err != nil {
		t.Fatalf("initialize second world: %v", err)
	}
	inputs := &OneFrameInputs{
		Inputs: map[int64]*LSInput{
			1: {V: TSVector2{X: int64(uint64(1) << trueSyncFixedFractionBits), Y: -int64(uint64(1) << trueSyncFixedFractionBits)}},
			2: {V: TSVector2{X: -int64(uint64(1) << trueSyncFixedFractionBits), Y: int64(uint64(1) << trueSyncFixedFractionBits)}},
		},
	}
	for _, world := range []*LSWorld{first, second} {
		if err := world.ApplyFrameInputs(inputs); err != nil {
			t.Fatalf("apply inputs: %v", err)
		}
		if err := world.Update(); err != nil {
			t.Fatalf("update world: %v", err)
		}
	}
	if first.Frame != 1 || second.Frame != 1 {
		t.Fatalf("frames = %d/%d, want 1/1", first.Frame, second.Frame)
	}
	for _, playerID := range []int64{1, 2} {
		left, leftOK := first.Unit(playerID)
		right, rightOK := second.Unit(playerID)
		if !leftOK || !rightOK {
			t.Fatalf("player %d missing after update", playerID)
		}
		if left.Position != right.Position || etmath.QuaternionDistance(left.Rotation, right.Rotation) > 0.000001 {
			t.Fatalf("player %d differs: first=%+v second=%+v", playerID, left, right)
		}
	}

	playerOne, _ := first.Unit(1)
	if playerOne.Position.X <= 20.29 || playerOne.Position.X >= 20.31 ||
		playerOne.Position.Z >= -10.29 || playerOne.Position.Z <= -10.31 {
		t.Fatalf("player 1 position = %+v, want approximately (20.3, 0, -10.3)", playerOne.Position)
	}
	forward := etmath.QuaternionForward(playerOne.Rotation)
	if forward.X <= 0.69 || forward.X >= 0.72 || forward.Z >= -0.69 || forward.Z <= -0.72 {
		t.Fatalf("player 1 forward = %+v, want approximately (0.707, 0, -0.707)", forward)
	}
}

func TestLSWorldRejectsUnknownInputWithoutPartialMutation(t *testing.T) {
	world := NewLSWorld(int(ecs.SceneTypeLockStep))
	if err := world.InitializePlayers([]int64{1}); err != nil {
		t.Fatalf("initialize world: %v", err)
	}
	before, err := world.SnapshotState()
	if err != nil {
		t.Fatalf("snapshot before input: %v", err)
	}
	err = world.ApplyFrameInputs(&OneFrameInputs{
		Inputs: map[int64]*LSInput{
			1: {Button: 7},
			2: {Button: 8},
		},
	})
	if !errors.Is(err, ErrLSWorldPlayerMissing) {
		t.Fatalf("ApplyFrameInputs error = %v, want %v", err, ErrLSWorldPlayerMissing)
	}
	after, err := world.SnapshotState()
	if err != nil {
		t.Fatalf("snapshot after input: %v", err)
	}
	if len(after.Units) != 1 || after.Units[0].Input != before.Units[0].Input {
		t.Fatalf("world mutated after rejected input: before=%+v after=%+v", before, after)
	}
}

func TestLSWorldRestoreStateIsAtomic(t *testing.T) {
	world := NewLSWorld(int(ecs.SceneTypeLockStep))
	if err := world.InitializePlayers([]int64{1, 2}); err != nil {
		t.Fatalf("initialize world: %v", err)
	}
	before, err := world.SnapshotState()
	if err != nil {
		t.Fatalf("snapshot before restore: %v", err)
	}
	invalid := before
	invalid.Units = append(invalid.Units, invalid.Units[0])
	if err := world.RestoreState(invalid); !errors.Is(err, ErrLSWorldStateInvalid) {
		t.Fatalf("RestoreState error = %v, want %v", err, ErrLSWorldStateInvalid)
	}
	after, err := world.SnapshotState()
	if err != nil {
		t.Fatalf("snapshot after restore: %v", err)
	}
	if len(after.Units) != len(before.Units) || after.Frame != before.Frame ||
		after.IDGenerator != before.IDGenerator {
		t.Fatalf("world mutated after rejected restore: before=%+v after=%+v", before, after)
	}
}

func TestLSWorldUpdateRejectsInvalidUnitWithoutPartialMovement(t *testing.T) {
	world := NewLSWorld(int(ecs.SceneTypeLockStep))
	if err := world.InitializePlayers([]int64{1, 2}); err != nil {
		t.Fatalf("initialize world: %v", err)
	}
	before, _ := world.Unit(1)
	world.units[2].ID = 0
	err := world.ApplyFrameInputs(&OneFrameInputs{
		Inputs: map[int64]*LSInput{
			1: {V: TSVector2{X: int64(uint64(1) << trueSyncFixedFractionBits)}},
			2: {V: TSVector2{X: int64(uint64(1) << trueSyncFixedFractionBits)}},
		},
	})
	if err != nil {
		t.Fatalf("apply inputs: %v", err)
	}
	updateErr := world.Update()
	if !errors.Is(updateErr, ErrLSWorldUnitInvalid) {
		t.Fatalf("Update error = %v, want %v", updateErr, ErrLSWorldUnitInvalid)
	}
	after, _ := world.Unit(1)
	if after.Position != before.Position || world.Frame != 0 {
		t.Fatalf("world partially moved: before=%+v after=%+v frame=%d", before, after, world.Frame)
	}
}

func TestLSWorldSyncPlayersPreservesExistingState(t *testing.T) {
	world := NewLSWorld(int(ecs.SceneTypeLockStep))
	if err := world.InitializePlayers([]int64{1}); err != nil {
		t.Fatalf("initialize world: %v", err)
	}
	if err := world.ApplyFrameInputs(&OneFrameInputs{
		Inputs: map[int64]*LSInput{1: {Button: 9}},
	}); err != nil {
		t.Fatalf("apply input: %v", err)
	}
	if err := world.Update(); err != nil {
		t.Fatalf("update world: %v", err)
	}
	before, _ := world.Unit(1)
	if err := world.SyncPlayers([]int64{1, 2}); err != nil {
		t.Fatalf("sync players: %v", err)
	}
	after, _ := world.Unit(1)
	if after.Position != before.Position || after.Input != before.Input || world.Frame != 1 {
		t.Fatalf("existing player state was reset: before=%+v after=%+v frame=%d", before, after, world.Frame)
	}
	if _, ok := world.Unit(2); !ok {
		t.Fatal("new player unit missing")
	}
}
