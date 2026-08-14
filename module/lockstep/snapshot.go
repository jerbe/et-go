package lockstep

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jerbe/et-go/engine/actor"
)

const lockstepSnapshotVersion = 2

var (
	// ErrSnapshotRoomMissing 表示快照缺少 Room 状态。
	ErrSnapshotRoomMissing = errors.New("lockstep: snapshot room missing")
	// ErrSnapshotServerMissing 表示快照缺少 RoomServer 状态。
	ErrSnapshotServerMissing = errors.New("lockstep: snapshot room server missing")
	// ErrSnapshotChecksumMismatch 表示快照校验和不匹配。
	ErrSnapshotChecksumMismatch = errors.New("lockstep: snapshot checksum mismatch")
	// ErrSnapshotVersionUnsupported 表示快照版本不受支持。
	ErrSnapshotVersionUnsupported = errors.New("lockstep: snapshot version unsupported")
)

type lockstepSnapshotEnvelope struct {
	Version int                   `json:"version"`
	SHA256  string                `json:"sha256"`
	State   lockstepSnapshotState `json:"state"`
}

type lockstepSnapshotState struct {
	StartTime       int64                    `json:"start_time"`
	AuthorityFrame  int                      `json:"authority_frame"`
	PredictionFrame int                      `json:"prediction_frame"`
	Players         []lockstepSnapshotPlayer `json:"players"`
	Frames          []lockstepSnapshotFrame  `json:"frames"`
	World           LSWorldState             `json:"world"`
}

type lockstepSnapshotPlayer struct {
	PlayerID    int64         `json:"player_id"`
	Online      bool          `json:"online"`
	Progress    int           `json:"progress"`
	GateActorID actor.ActorID `json:"gate_actor_id"`
}

type lockstepSnapshotFrame struct {
	Frame  int                     `json:"frame"`
	Inputs []lockstepSnapshotInput `json:"inputs"`
}

type lockstepSnapshotInput struct {
	PlayerID int64    `json:"player_id"`
	Input    *LSInput `json:"input"`
}

// MarshalRoomSnapshot 序列化当前 Go Lockstep Room 的完整可恢复状态。
//
// 快照不是固定占位字节：它包含版本、状态、帧输入和 SHA-256 校验和。
// 当前 serializer 同时保存 Room、FrameBuffer、RoomPlayer 和 Go LSWorld
// 状态。它是 Go 内部可恢复格式，不等于原 C# MemoryPack/TrueSync 的线协议；
// 客户端恢复仍必须使用双方共同约定的外部编码。
func MarshalRoomSnapshot(room *LockstepRoom, server *RoomServerComponent) ([]byte, error) {
	if room == nil || room.FrameBuffer == nil {
		return nil, ErrSnapshotRoomMissing
	}
	if server == nil {
		return nil, ErrSnapshotServerMissing
	}
	if room.World == nil {
		return nil, ErrLSWorldMissing
	}
	world, err := room.World.SnapshotState()
	if err != nil {
		return nil, fmt.Errorf("lockstep: snapshot ls world: %w", err)
	}
	if world.Frame != room.AuthorityFrame {
		return nil, fmt.Errorf("%w: room authority frame %d != world frame %d",
			ErrSnapshotStateInvalid, room.AuthorityFrame, world.Frame)
	}

	state := lockstepSnapshotState{
		StartTime:       room.StartTime,
		AuthorityFrame:  room.AuthorityFrame,
		PredictionFrame: room.PredictionFrame,
		Players:         snapshotPlayers(server, room.PlayerIds),
		Frames:          snapshotFrames(room.FrameBuffer, room.AuthorityFrame),
		World:           world,
	}
	if _, err := validateSnapshotState(state); err != nil {
		return nil, err
	}
	stateBytes, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("lockstep: marshal snapshot state: %w", err)
	}
	sum := sha256.Sum256(stateBytes)
	envelope := lockstepSnapshotEnvelope{
		Version: lockstepSnapshotVersion,
		SHA256:  hex.EncodeToString(sum[:]),
		State:   state,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("lockstep: marshal snapshot envelope: %w", err)
	}
	return data, nil
}

// VerifyRoomSnapshot 校验快照版本和状态校验和。
func VerifyRoomSnapshot(data []byte) error {
	if len(data) == 0 {
		return ErrSnapshotEmpty
	}
	var envelope lockstepSnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("lockstep: decode snapshot: %w", err)
	}
	if envelope.Version != lockstepSnapshotVersion {
		return fmt.Errorf("%w: %d", ErrSnapshotVersionUnsupported, envelope.Version)
	}
	stateBytes, err := json.Marshal(envelope.State)
	if err != nil {
		return fmt.Errorf("lockstep: marshal snapshot state for verification: %w", err)
	}
	sum := sha256.Sum256(stateBytes)
	if !equalStringFold(envelope.SHA256, hex.EncodeToString(sum[:])) {
		return ErrSnapshotChecksumMismatch
	}
	return nil
}

// RestoreRoomSnapshot 将 Go Room/FrameBuffer/RoomPlayer 状态恢复到现有对象。
//
// 函数恢复当前 Go 运行时明确建模的 Room、FrameBuffer、RoomPlayer 和
// LSWorld 状态。输入必须先通过版本和 SHA-256 校验，玩家、帧号、帧输入和
// 世界实体结构不合法时直接失败，不执行部分恢复。
func RestoreRoomSnapshot(data []byte, room *LockstepRoom, server *RoomServerComponent) error {
	if room == nil {
		return ErrSnapshotRoomMissing
	}
	if server == nil {
		return ErrSnapshotServerMissing
	}
	if err := VerifyRoomSnapshot(data); err != nil {
		return err
	}
	var envelope lockstepSnapshotEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("lockstep: decode snapshot for restore: %w", err)
	}
	state := envelope.State
	playerIDs, err := validateSnapshotState(state)
	if err != nil {
		return err
	}
	newWorld := NewLSWorld(state.World.SceneType)
	if err := newWorld.RestoreState(state.World); err != nil {
		return fmt.Errorf("%w: restore ls world: %v", ErrSnapshotStateInvalid, err)
	}

	newFrameBuffer := NewFrameBuffer()
	maxFrame := 0
	for _, frame := range state.Frames {
		if frame.Frame > maxFrame {
			maxFrame = frame.Frame
		}
		inputs := NewOneFrameInputs()
		for _, input := range frame.Inputs {
			if input.Input == nil {
				inputs.Inputs[input.PlayerID] = nil
				continue
			}
			value := *input.Input
			inputs.Inputs[input.PlayerID] = &value
		}
		newFrameBuffer.SetFrameInputs(frame.Frame, inputs)
	}

	if err := server.restoreSnapshotPlayers(room, playerIDs, state.Players); err != nil {
		return err
	}
	room.World = newWorld
	room.StartTime = state.StartTime
	room.AuthorityFrame = state.AuthorityFrame
	room.PredictionFrame = state.PredictionFrame
	room.FrameBuffer = newFrameBuffer
	room.Replay = NewReplay()
	for frame := 1; frame <= maxFrame; frame++ {
		if inputs, ok := newFrameBuffer.GetFrameInputs(frame); ok {
			room.Replay.AddFrameInput(inputs)
		}
	}
	return nil
}

func validateSnapshotState(state lockstepSnapshotState) ([]int64, error) {
	if state.AuthorityFrame < 0 || state.PredictionFrame < 0 ||
		state.PredictionFrame < state.AuthorityFrame {
		return nil, fmt.Errorf("%w: invalid frame counters", ErrSnapshotStateInvalid)
	}
	if len(state.Players) == 0 {
		return nil, fmt.Errorf("%w: no players", ErrSnapshotStateInvalid)
	}
	playerIDs := make([]int64, 0, len(state.Players))
	seenPlayers := make(map[int64]struct{}, len(state.Players))
	for _, player := range state.Players {
		if player.PlayerID <= 0 {
			return nil, fmt.Errorf("%w: invalid player id", ErrSnapshotStateInvalid)
		}
		if _, exists := seenPlayers[player.PlayerID]; exists {
			return nil, fmt.Errorf("%w: duplicate player id %d", ErrSnapshotStateInvalid, player.PlayerID)
		}
		if player.Progress < 0 || player.Progress > 100 {
			return nil, fmt.Errorf("%w: invalid player progress", ErrSnapshotStateInvalid)
		}
		seenPlayers[player.PlayerID] = struct{}{}
		playerIDs = append(playerIDs, player.PlayerID)
	}
	sort.Slice(playerIDs, func(i, j int) bool { return playerIDs[i] < playerIDs[j] })

	previousFrame := 0
	for _, frame := range state.Frames {
		expectedFrame := previousFrame + 1
		if frame.Frame <= 0 || frame.Frame != expectedFrame {
			return nil, fmt.Errorf("%w: frames are not contiguous", ErrSnapshotStateInvalid)
		}
		previousFrame = frame.Frame
		seenInputs := make(map[int64]struct{}, len(frame.Inputs))
		for _, input := range frame.Inputs {
			if _, exists := seenPlayers[input.PlayerID]; !exists {
				return nil, fmt.Errorf("%w: frame input player %d missing", ErrSnapshotStateInvalid, input.PlayerID)
			}
			if _, exists := seenInputs[input.PlayerID]; exists {
				return nil, fmt.Errorf("%w: duplicate frame input player %d", ErrSnapshotStateInvalid, input.PlayerID)
			}
			seenInputs[input.PlayerID] = struct{}{}
		}
	}
	if previousFrame > state.AuthorityFrame {
		return nil, fmt.Errorf("%w: frame exceeds authority frame", ErrSnapshotStateInvalid)
	}
	if previousFrame != state.AuthorityFrame {
		return nil, fmt.Errorf("%w: frame list does not reach authority frame", ErrSnapshotStateInvalid)
	}
	if state.World.SceneType <= 0 || state.World.Frame != state.AuthorityFrame {
		return nil, fmt.Errorf("%w: invalid ls world frame or scene", ErrSnapshotStateInvalid)
	}
	if len(state.World.Units) != len(playerIDs) {
		return nil, fmt.Errorf("%w: ls world unit count mismatch", ErrSnapshotStateInvalid)
	}
	seenUnits := make(map[int64]struct{}, len(state.World.Units))
	for _, unit := range state.World.Units {
		if unit.ID <= 0 || unit.PlayerID <= 0 || unit.ID != unit.PlayerID {
			return nil, fmt.Errorf("%w: invalid ls world unit identity", ErrSnapshotStateInvalid)
		}
		if _, exists := seenPlayers[unit.PlayerID]; !exists {
			return nil, fmt.Errorf("%w: ls world unit player %d missing", ErrSnapshotStateInvalid, unit.PlayerID)
		}
		if _, exists := seenUnits[unit.PlayerID]; exists {
			return nil, fmt.Errorf("%w: duplicate ls world unit player %d", ErrSnapshotStateInvalid, unit.PlayerID)
		}
		seenUnits[unit.PlayerID] = struct{}{}
	}
	return playerIDs, nil
}

func snapshotPlayers(server *RoomServerComponent, playerIDs []int64) []lockstepSnapshotPlayer {
	ids := append([]int64(nil), playerIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	players := make([]lockstepSnapshotPlayer, 0, len(ids))
	for _, playerID := range ids {
		player := lockstepSnapshotPlayer{PlayerID: playerID}
		if server != nil {
			if current := server.Player(playerID); current != nil {
				player.Online = current.IsOnline
				player.Progress = current.Progress
				player.GateActorID = current.GateActorID
			}
		}
		players = append(players, player)
	}
	return players
}

func snapshotFrames(frameBuffer *FrameBuffer, maxFrame int) []lockstepSnapshotFrame {
	if maxFrame <= 0 {
		return nil
	}
	frames := make([]lockstepSnapshotFrame, 0, maxFrame)
	for frame := 1; frame <= maxFrame; frame++ {
		inputs, ok := frameBuffer.GetFrameInputs(frame)
		frameState := lockstepSnapshotFrame{Frame: frame}
		if ok && inputs != nil {
			playerIDs := make([]int64, 0, len(inputs.Inputs))
			for playerID := range inputs.Inputs {
				playerIDs = append(playerIDs, playerID)
			}
			sort.Slice(playerIDs, func(i, j int) bool { return playerIDs[i] < playerIDs[j] })
			frameState.Inputs = make([]lockstepSnapshotInput, 0, len(playerIDs))
			for _, playerID := range playerIDs {
				frameState.Inputs = append(frameState.Inputs, lockstepSnapshotInput{
					PlayerID: playerID,
					Input:    cloneLSInput(inputs.Inputs[playerID]),
				})
			}
		}
		frames = append(frames, frameState)
	}
	return frames
}

func cloneLSInput(input *LSInput) *LSInput {
	if input == nil {
		return nil
	}
	clone := *input
	return &clone
}

func equalStringFold(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		a, b := left[index], right[index]
		if a >= 'A' && a <= 'F' {
			a += 'a' - 'A'
		}
		if b >= 'A' && b <= 'F' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}
