package lockstep

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// SnapshotProvider 将当前 Go 锁步房间和 LSWorld 序列化为可恢复快照。
//
// 生产路径必须注入真实 serializer；默认 Room Fiber 使用
// MarshalRoomSnapshot。没有 provider 时不能写入固定占位字节。
type SnapshotProvider func(room *LockstepRoom) ([]byte, error)

type LSServerUpdater struct {
	ecs.BaseComponent
	frameBuffer *FrameBuffer
	frame       int
	room        *LockstepRoom
	nowFunc     func() time.Time

	snapshotMu       sync.RWMutex
	snapshotProvider SnapshotProvider
	snapshotErr      error
	updateErr        error
}

func NewLSServerUpdater(fb *FrameBuffer) *LSServerUpdater {
	return &LSServerUpdater{
		frameBuffer: fb,
	}
}

func (u *LSServerUpdater) Type() string { return "LSServerUpdater" }

func (u *LSServerUpdater) BindRoom(room *LockstepRoom) {
	u.room = room
	if room == nil {
		return
	}
	u.frame = room.AuthorityFrame
	if room.FrameBuffer != nil {
		u.frameBuffer = room.FrameBuffer
	}
}

func (u *LSServerUpdater) SetNowFunc(fn func() time.Time) {
	u.nowFunc = fn
}

// SetSnapshotProvider 注入真实世界状态快照序列化器。
func (u *LSServerUpdater) SetSnapshotProvider(provider SnapshotProvider) {
	u.snapshotMu.Lock()
	u.snapshotProvider = provider
	u.snapshotErr = nil
	u.snapshotMu.Unlock()
}

// LastSnapshotError 返回最近一次生成快照的错误。
func (u *LSServerUpdater) LastSnapshotError() error {
	u.snapshotMu.RLock()
	defer u.snapshotMu.RUnlock()
	return u.snapshotErr
}

// LastUpdateError 返回最近一次世界推进错误。
func (u *LSServerUpdater) LastUpdateError() error {
	u.snapshotMu.RLock()
	defer u.snapshotMu.RUnlock()
	return u.updateErr
}

// CaptureSnapshot 保存当前权威帧的恢复快照。
//
// 房间启动时 AuthorityFrame 为 0，此时也必须允许生成快照，否则启动
// 后到第一个权威帧推进前发生哈希异常时没有可回退的状态。
func (u *LSServerUpdater) CaptureSnapshot() error {
	if u == nil || u.frameBuffer == nil {
		err := ErrFrameBufferMissing
		if u != nil {
			u.setSnapshotError(err)
		}
		return err
	}
	if u.room == nil {
		u.setSnapshotError(ErrRoomDefinitionMissing)
		return ErrRoomDefinitionMissing
	}
	if u.room.World == nil {
		u.setSnapshotError(ErrLSWorldMissing)
		return ErrLSWorldMissing
	}
	if u.room.Replay == nil {
		u.setSnapshotError(ErrReplayMissing)
		return ErrReplayMissing
	}
	frame := u.room.AuthorityFrame
	if frame < 0 || u.frame != frame {
		u.setSnapshotError(ErrLockstepStateOutOfSync)
		return ErrLockstepStateOutOfSync
	}

	u.snapshotMu.RLock()
	provider := u.snapshotProvider
	u.snapshotMu.RUnlock()
	if provider == nil {
		u.setSnapshotError(ErrSnapshotProviderMissing)
		return ErrSnapshotProviderMissing
	}
	snapshot, err := provider(u.room)
	if err != nil {
		u.setSnapshotError(err)
		return err
	}
	if len(snapshot) == 0 {
		u.setSnapshotError(ErrSnapshotEmpty)
		return ErrSnapshotEmpty
	}
	u.frameBuffer.SetSnapshot(frame, snapshot)
	u.room.Replay.AddSnapshot(frame, snapshot)
	u.setSnapshotError(nil)
	return nil
}

func (u *LSServerUpdater) now() time.Time {
	if u.nowFunc != nil {
		return u.nowFunc()
	}
	return time.Now()
}

func (u *LSServerUpdater) AddInput(frame int, playerId int64, input *LSInput) error {
	if u == nil || u.frameBuffer == nil {
		return ErrFrameBufferMissing
	}
	if u.room == nil {
		return ErrRoomDefinitionMissing
	}
	if u.room.World == nil {
		return ErrLSWorldMissing
	}
	if frame <= 0 {
		return ErrFrameInvalid
	}
	if playerId <= 0 {
		return ErrPlayerInvalid
	}
	if _, ok := u.room.World.Unit(playerId); !ok {
		return ErrLSWorldPlayerMissing
	}
	frameInputs, ok := u.frameBuffer.GetFrameInputs(frame)
	if !ok || frameInputs == nil {
		frameInputs = NewOneFrameInputs()
	}
	if input == nil {
		input = &LSInput{}
	}
	value := *input
	frameInputs.Inputs[playerId] = &value
	u.frameBuffer.SetFrameInputs(frame, frameInputs)
	return nil
}

func (u *LSServerUpdater) Update() {
	if u.frameBuffer == nil || u.room == nil || u.room.StartTime == 0 {
		return
	}
	if u.room.World == nil {
		u.setUpdateError(ErrLSWorldMissing)
		return
	}
	if u.room.AuthorityFrame != u.frame || u.room.World.Frame != u.frame {
		u.setUpdateError(ErrLockstepStateOutOfSync)
		return
	}
	if u.room.Replay == nil {
		u.setSnapshotError(ErrReplayMissing)
		return
	}
	targetFrame := u.frame + 1
	elapsed := u.now().UnixMilli() - u.room.StartTime
	if elapsed < 0 {
		return
	}
	calculated := int(elapsed / UpdateIntervalMillis)
	if calculated <= u.frame {
		return
	}
	u.setUpdateError(nil)
	targetFrame = calculated
	for u.frame < targetFrame {
		frame := u.frame + 1
		frameInputs := u.ensureFrame(frame)
		if err := u.room.World.ApplyFrameInputs(frameInputs); err != nil {
			u.setUpdateError(err)
			return
		}
		if err := u.room.World.Update(); err != nil {
			u.setUpdateError(err)
			return
		}
		u.frame = frame
		u.room.AuthorityFrame = frame
		u.room.Replay.AddFrameInput(frameInputs)
		if entity := u.GetEntity(); entity != nil {
			if err := broadcastToGate(entity.Scene(), u.room.PlayerIds, MsgOneFrameInputs, frameInputs); err != nil {
				slog.Error("lockstep frame input broadcast failed", "frame", frame, "err", err)
			}
		}
		// Keep a recovery point at the first authoritative frame as well as
		// the regular long-interval checkpoints. Without the initial
		// checkpoint, a hash mismatch during the first prediction window has
		// no snapshot that can be sent to the client.
		if frame == 1 || frame%SaveLSWorldFrameCount == 0 {
			if err := u.CaptureSnapshot(); err != nil {
				continue
			}
		}
	}
}

func (u *LSServerUpdater) setSnapshotError(err error) {
	u.snapshotMu.Lock()
	u.snapshotErr = err
	u.snapshotMu.Unlock()
}

func (u *LSServerUpdater) setUpdateError(err error) {
	u.snapshotMu.Lock()
	u.updateErr = err
	u.snapshotMu.Unlock()
}

func (u *LSServerUpdater) ensureFrame(frame int) *OneFrameInputs {
	frameInputs, ok := u.frameBuffer.GetFrameInputs(frame)
	if !ok || frameInputs == nil {
		frameInputs = NewOneFrameInputs()
	}
	var prevInputs *OneFrameInputs
	if frame > 1 {
		prevInputs, _ = u.frameBuffer.GetFrameInputs(frame - 1)
	}
	for _, playerID := range u.room.PlayerIds {
		if _, ok := frameInputs.Inputs[playerID]; ok {
			continue
		}
		if prevInputs != nil {
			if prevInput, ok := prevInputs.Inputs[playerID]; ok && prevInput != nil {
				value := *prevInput
				frameInputs.Inputs[playerID] = &value
				continue
			}
		}
		frameInputs.Inputs[playerID] = &LSInput{}
	}
	u.frameBuffer.SetFrameInputs(frame, frameInputs)
	return frameInputs
}

func (u *LSServerUpdater) Frame() int { return u.frame }
