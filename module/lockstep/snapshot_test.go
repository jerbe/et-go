package lockstep

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
)

func TestMarshalRoomSnapshotIsVersionedAndVerifiable(t *testing.T) {
	room := NewLockstepRoom([]int64{2, 1})
	room.StartTime = 100
	setSnapshotTestFrame(room, 1)
	room.PredictionFrame = 2
	room.FrameBuffer.SetFrameInputs(1, &OneFrameInputs{
		Inputs: map[int64]*LSInput{
			2: {Button: 2},
			1: {Button: 1},
		},
	})
	server := NewRoomServerComponent(room)
	server.Awake()

	first, err := MarshalRoomSnapshot(room, server)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}
	second, err := MarshalRoomSnapshot(room, server)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot second error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("snapshot should be deterministic")
	}
	if err := VerifyRoomSnapshot(first); err != nil {
		t.Fatalf("VerifyRoomSnapshot error = %v", err)
	}
}

func TestVerifyRoomSnapshotRejectsCorruption(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	data, err := MarshalRoomSnapshot(room, NewRoomServerComponent(room))
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}
	data[len(data)-2] ^= 1
	if err := VerifyRoomSnapshot(data); err == nil {
		t.Fatal("corrupted snapshot should fail verification")
	}
}

func TestMarshalRoomSnapshotRequiresServer(t *testing.T) {
	_, err := MarshalRoomSnapshot(NewLockstepRoom([]int64{1}), nil)
	if !errors.Is(err, ErrSnapshotServerMissing) {
		t.Fatalf("MarshalRoomSnapshot error = %v, want %v", err, ErrSnapshotServerMissing)
	}
}

func TestRestoreRoomSnapshotRestoresGoRoomState(t *testing.T) {
	source := NewLockstepRoom([]int64{2, 1})
	source.StartTime = 100
	setSnapshotTestFrame(source, 2)
	source.PredictionFrame = 3
	source.FrameBuffer.SetFrameInputs(1, &OneFrameInputs{
		Inputs: map[int64]*LSInput{1: {Button: 7}},
	})
	source.FrameBuffer.SetFrameInputs(2, &OneFrameInputs{
		Inputs: map[int64]*LSInput{2: {Button: 8}},
	})
	sourceServer := NewRoomServerComponent(source)
	sourceServer.Awake()
	sourceServer.RestorePlayerState(1, false, 80, actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3})

	data, err := MarshalRoomSnapshot(source, sourceServer)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}

	target := NewLockstepRoom(nil)
	targetServer := NewRoomServerComponent(target)
	if err := RestoreRoomSnapshot(data, target, targetServer); err != nil {
		t.Fatalf("RestoreRoomSnapshot error = %v", err)
	}
	if target.StartTime != source.StartTime ||
		target.AuthorityFrame != source.AuthorityFrame ||
		target.PredictionFrame != source.PredictionFrame {
		t.Fatalf("restored room = start=%d authority=%d prediction=%d", target.StartTime, target.AuthorityFrame, target.PredictionFrame)
	}
	inputs, ok := target.FrameBuffer.GetFrameInputs(1)
	if !ok || inputs.Inputs[1] == nil || inputs.Inputs[1].Button != 7 {
		t.Fatalf("restored frame 1 inputs = %+v", inputs)
	}
	player := targetServer.Player(1)
	if player == nil || player.IsOnline || player.Progress != 80 ||
		player.GateActorID != (actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}) {
		t.Fatalf("restored player = %+v", player)
	}
}

func TestRoomSnapshotPreservesEmptyFramesForReplayAlignment(t *testing.T) {
	source := NewLockstepRoom([]int64{1})
	setSnapshotTestFrame(source, 3)
	source.PredictionFrame = 3
	source.FrameBuffer.SetFrameInputs(1, &OneFrameInputs{
		Inputs: map[int64]*LSInput{1: {Button: 7}},
	})
	source.FrameBuffer.SetFrameInputs(3, &OneFrameInputs{
		Inputs: map[int64]*LSInput{1: {Button: 9}},
	})
	sourceServer := NewRoomServerComponent(source)
	sourceServer.Awake()

	data, err := MarshalRoomSnapshot(source, sourceServer)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}

	target := NewLockstepRoom(nil)
	targetServer := NewRoomServerComponent(target)
	if err := RestoreRoomSnapshot(data, target, targetServer); err != nil {
		t.Fatalf("RestoreRoomSnapshot error = %v", err)
	}
	if _, ok := target.FrameBuffer.GetFrameInputs(2); !ok {
		t.Fatal("empty frame 2 should remain represented in the restored buffer")
	}
	if inputs := target.Replay.GetFrameInputsRange(0, 3); len(inputs) != 3 {
		t.Fatalf("replay frame count = %d, want 3", len(inputs))
	}
}

func TestRestoreRoomSnapshotRejectsInvalidFrameState(t *testing.T) {
	room := NewLockstepRoom([]int64{1})
	setSnapshotTestFrame(room, 1)
	server := NewRoomServerComponent(room)
	server.Awake()
	data, err := MarshalRoomSnapshot(room, server)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}

	var envelope lockstepSnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode snapshot error = %v", err)
	}
	envelope.State.PredictionFrame = -1
	stateBytes, err := json.Marshal(envelope.State)
	if err != nil {
		t.Fatalf("marshal invalid state error = %v", err)
	}
	sum := sha256.Sum256(stateBytes)
	envelope.SHA256 = hex.EncodeToString(sum[:])
	data, err = json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal invalid snapshot error = %v", err)
	}
	if err := RestoreRoomSnapshot(data, NewLockstepRoom(nil), NewRoomServerComponent(NewLockstepRoom(nil))); !errors.Is(err, ErrSnapshotStateInvalid) {
		t.Fatalf("RestoreRoomSnapshot error = %v, want %v", err, ErrSnapshotStateInvalid)
	}
}

func TestRestoreRoomSnapshotDoesNotPartiallyMutateClosedServer(t *testing.T) {
	source := NewLockstepRoom([]int64{1})
	setSnapshotTestFrame(source, 1)
	sourceServer := NewRoomServerComponent(source)
	sourceServer.Awake()
	data, err := MarshalRoomSnapshot(source, sourceServer)
	if err != nil {
		t.Fatalf("MarshalRoomSnapshot error = %v", err)
	}

	target := NewLockstepRoom([]int64{99})
	target.StartTime = 77
	setSnapshotTestFrame(target, 9)
	targetServer := NewRoomServerComponent(target)
	targetServer.Awake()
	targetServer.OnDestroy()

	err = RestoreRoomSnapshot(data, target, targetServer)
	if !errors.Is(err, ErrSnapshotStateInvalid) {
		t.Fatalf("RestoreRoomSnapshot error = %v, want %v", err, ErrSnapshotStateInvalid)
	}
	if target.StartTime != 77 || target.AuthorityFrame != 9 {
		t.Fatalf("target room partially mutated: start=%d authority=%d", target.StartTime, target.AuthorityFrame)
	}
	if len(target.PlayerIds) != 1 || target.PlayerIds[0] != 99 {
		t.Fatalf("target players partially mutated: %v", target.PlayerIds)
	}
}

func setSnapshotTestFrame(room *LockstepRoom, frame int) {
	room.AuthorityFrame = frame
	room.PredictionFrame = frame
	room.World.Frame = frame
}
