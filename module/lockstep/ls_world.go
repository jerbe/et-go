package lockstep

import (
	"errors"
	"math"
	"sort"
	"sync"

	etmath "github.com/jerbe/et-go/engine/math"
)

const (
	// trueSyncFixedFractionBits 是原 ET TrueSync FP 的 Q31.32 小数位数。
	trueSyncFixedFractionBits = 32
	trueSyncFixedScale        = float64(uint64(1) << trueSyncFixedFractionBits)
	// lockstepInputMoveNumerator/Denominator 对应原 C#：
	// TSVector2 * 6 * 50 / 1000。
	lockstepInputMoveNumerator   = 6 * 50
	lockstepInputMoveDenominator = 1000
)

var (
	// ErrLSWorldMissing 表示 Room 没有绑定确定性世界。
	ErrLSWorldMissing = errors.New("lockstep: ls world missing")
	// ErrLSWorldUnitInvalid 表示世界单位身份非法。
	ErrLSWorldUnitInvalid = errors.New("lockstep: ls world unit invalid")
	// ErrLSWorldPlayerMissing 表示输入引用了不存在的玩家。
	ErrLSWorldPlayerMissing = errors.New("lockstep: ls world player missing")
	// ErrLSWorldStateInvalid 表示世界快照状态非法。
	ErrLSWorldStateInvalid = errors.New("lockstep: ls world state invalid")
	// ErrLSWorldInputInvalid 表示输入无法被确定性世界接受。
	ErrLSWorldInputInvalid = errors.New("lockstep: ls world input invalid")
)

// LSUnit 是 Go 侧对应原 ET LSUnit 的最小确定性状态。
//
// Position/Rotation 使用 Go math 类型承载；输入仍按原 TrueSync Q31.32
// 原始整数解码。真正的跨语言 TrueSync 定点序列化协议仍由外部客户端
// 兼容层负责，不能在这里把 float32 误宣称为原始 FP wire format。
type LSUnit struct {
	ID       int64
	PlayerID int64
	Position etmath.Vector3
	Rotation etmath.Quaternion
	Input    LSInput
}

// LSWorld 是 Room 内的确定性战斗世界。
//
// 业务状态只应在 Room Fiber 内修改；锁只用于让快照测试和诊断读取不会
// 与显式 API 交错，不代表可以绕过 Room Fiber 直接修改单位。
type LSWorld struct {
	mu sync.RWMutex

	SceneType   int
	Frame       int
	IDGenerator int64
	units       map[int64]*LSUnit
}

// LSWorldState 是可版本化的世界状态，不包含运行时锁和 map。
type LSWorldState struct {
	SceneType   int           `json:"scene_type"`
	Frame       int           `json:"frame"`
	IDGenerator int64         `json:"id_generator"`
	Units       []LSUnitState `json:"units"`
}

// LSUnitState 是单个 LSUnit 的可恢复状态。
type LSUnitState struct {
	ID       int64             `json:"id"`
	PlayerID int64             `json:"player_id"`
	Position etmath.Vector3     `json:"position"`
	Rotation etmath.Quaternion `json:"rotation"`
	Input    LSInput           `json:"input"`
}

// NewLSWorld 创建一个空的确定性世界。
func NewLSWorld(sceneType int) *LSWorld {
	return &LSWorld{
		SceneType: sceneType,
		units:     make(map[int64]*LSUnit),
	}
}

// InitializePlayers 用默认起始状态创建玩家单位。
func (w *LSWorld) InitializePlayers(playerIDs []int64) error {
	infos := make([]*LockStepUnitInfo, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		infos = append(infos, &LockStepUnitInfo{
			PlayerId: playerID,
			Position: etmath.Vector3{X: 20, Y: 0, Z: -10},
			Rotation: etmath.QuaternionIdentity,
		})
	}
	return w.InitializeUnitInfos(infos)
}

// InitializeUnitInfos 用 Room Start 状态重建世界单位。
func (w *LSWorld) InitializeUnitInfos(infos []*LockStepUnitInfo) error {
	if w == nil {
		return ErrLSWorldMissing
	}
	if len(infos) == 0 {
		return ErrRoomPlayersInvalid
	}
	next := make(map[int64]*LSUnit, len(infos))
	var nextID int64
	for _, info := range infos {
		if info == nil || info.PlayerId <= 0 {
			return ErrLSWorldUnitInvalid
		}
		if _, exists := next[info.PlayerId]; exists {
			return ErrRoomPlayersInvalid
		}
		if !validFloat3(info.Position) || !validQuaternion(info.Rotation) {
			return ErrLSWorldUnitInvalid
		}
		if info.PlayerId > nextID {
			nextID = info.PlayerId
		}
		next[info.PlayerId] = &LSUnit{
			ID:       info.PlayerId,
			PlayerID: info.PlayerId,
			Position: info.Position,
			Rotation: info.Rotation,
		}
	}
	w.mu.Lock()
	w.units = next
	w.IDGenerator = nextID
	w.Frame = 0
	w.mu.Unlock()
	return nil
}

// AddPlayer 在不重置现有世界帧的情况下添加一个默认起始单位。
func (w *LSWorld) AddPlayer(playerID int64) error {
	if w == nil {
		return ErrLSWorldMissing
	}
	if playerID <= 0 {
		return ErrLSWorldUnitInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.units[playerID]; exists {
		return ErrRoomPlayersInvalid
	}
	w.units[playerID] = &LSUnit{
		ID:       playerID,
		PlayerID: playerID,
		Position: etmath.Vector3{X: 20, Y: 0, Z: -10},
		Rotation: etmath.QuaternionIdentity,
	}
	if playerID > w.IDGenerator {
		w.IDGenerator = playerID
	}
	return nil
}

// SyncPlayers 让世界单位集合与房间玩家集合一致，并保留已有单位状态。
//
// 新增玩家使用原 Room 初始化约定的起始状态；已有玩家的 Position、
// Rotation、Input 和世界帧不会被重置。
func (w *LSWorld) SyncPlayers(playerIDs []int64) error {
	if w == nil {
		return ErrLSWorldMissing
	}
	if err := validateRoomPlayers(playerIDs); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	next := make(map[int64]*LSUnit, len(playerIDs))
	for _, playerID := range playerIDs {
		if unit, exists := w.units[playerID]; exists && unit != nil {
			clone := *unit
			next[playerID] = &clone
			continue
		}
		next[playerID] = &LSUnit{
			ID:       playerID,
			PlayerID: playerID,
			Position: etmath.Vector3{X: 20, Y: 0, Z: -10},
			Rotation: etmath.QuaternionIdentity,
		}
	}
	w.units = next
	for _, playerID := range playerIDs {
		if playerID > w.IDGenerator {
			w.IDGenerator = playerID
		}
	}
	return nil
}

// Unit 返回指定玩家单位的副本。
func (w *LSWorld) Unit(playerID int64) (*LSUnit, bool) {
	if w == nil || playerID <= 0 {
		return nil, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	unit, ok := w.units[playerID]
	if !ok || unit == nil {
		return nil, false
	}
	clone := *unit
	return &clone, true
}

// Units 返回按 PlayerID 排序的单位副本。
func (w *LSWorld) Units() []*LSUnit {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	ids := make([]int64, 0, len(w.units))
	for playerID := range w.units {
		ids = append(ids, playerID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	result := make([]*LSUnit, 0, len(ids))
	for _, playerID := range ids {
		unit := w.units[playerID]
		if unit == nil {
			continue
		}
		clone := *unit
		result = append(result, &clone)
	}
	return result
}

// ApplyFrameInputs 将一帧输入写入对应 LSUnit。
func (w *LSWorld) ApplyFrameInputs(inputs *OneFrameInputs) error {
	if w == nil {
		return ErrLSWorldMissing
	}
	if inputs == nil {
		return ErrLSWorldInputInvalid
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for playerID := range inputs.Inputs {
		unit, ok := w.units[playerID]
		if !ok || unit == nil {
			return ErrLSWorldPlayerMissing
		}
	}
	for playerID, input := range inputs.Inputs {
		unit := w.units[playerID]
		if input == nil {
			unit.Input = LSInput{}
			continue
		}
		unit.Input = *input
	}
	return nil
}

// Update 按原 C# LSInputComponentSystem 规则推进一帧世界。
func (w *LSWorld) Update() error {
	if w == nil {
		return ErrLSWorldMissing
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	ids := make([]int64, 0, len(w.units))
	for playerID := range w.units {
		ids = append(ids, playerID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, playerID := range ids {
		unit := w.units[playerID]
		if unit == nil || unit.PlayerID <= 0 || unit.ID <= 0 ||
			unit.ID != unit.PlayerID || !validFloat3(unit.Position) ||
			!validQuaternion(unit.Rotation) {
			return ErrLSWorldUnitInvalid
		}
	}
	for _, playerID := range ids {
		unit := w.units[playerID]
		deltaX, deltaZ := fixedInputDelta(unit.Input.V.X), fixedInputDelta(unit.Input.V.Y)
		if deltaX == 0 && deltaZ == 0 {
			continue
		}
		delta := etmath.Vector3{X: deltaX, Z: deltaZ}
		unit.Position = unit.Position.Add(delta)
		unit.Rotation = etmath.LookRotation(delta)
	}
	w.Frame++
	return nil
}

// SnapshotState 返回确定性排序后的世界状态。
func (w *LSWorld) SnapshotState() (LSWorldState, error) {
	if w == nil {
		return LSWorldState{}, ErrLSWorldMissing
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	ids := make([]int64, 0, len(w.units))
	for playerID := range w.units {
		ids = append(ids, playerID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	state := LSWorldState{
		SceneType:   w.SceneType,
		Frame:       w.Frame,
		IDGenerator: w.IDGenerator,
		Units:       make([]LSUnitState, 0, len(ids)),
	}
	for _, playerID := range ids {
		unit := w.units[playerID]
		if unit == nil {
			return LSWorldState{}, ErrLSWorldUnitInvalid
		}
		state.Units = append(state.Units, LSUnitState{
			ID:       unit.ID,
			PlayerID: unit.PlayerID,
			Position: unit.Position,
			Rotation: unit.Rotation,
			Input:    unit.Input,
		})
	}
	return state, nil
}

// RestoreState 原子恢复世界状态。
func (w *LSWorld) RestoreState(state LSWorldState) error {
	if w == nil {
		return ErrLSWorldMissing
	}
	if state.Frame < 0 || state.IDGenerator < 0 || len(state.Units) == 0 {
		return ErrLSWorldStateInvalid
	}
	next := make(map[int64]*LSUnit, len(state.Units))
	var maxID int64
	for _, item := range state.Units {
		if item.ID <= 0 || item.PlayerID <= 0 {
			return ErrLSWorldUnitInvalid
		}
		if _, exists := next[item.PlayerID]; exists {
			return ErrLSWorldStateInvalid
		}
		if !validFloat3(item.Position) || !validQuaternion(item.Rotation) {
			return ErrLSWorldStateInvalid
		}
		if item.ID > maxID {
			maxID = item.ID
		}
		next[item.PlayerID] = &LSUnit{
			ID:       item.ID,
			PlayerID: item.PlayerID,
			Position: item.Position,
			Rotation: item.Rotation,
			Input:    item.Input,
		}
	}
	if state.IDGenerator < maxID {
		return ErrLSWorldStateInvalid
	}
	w.mu.Lock()
	w.SceneType = state.SceneType
	w.Frame = state.Frame
	w.IDGenerator = state.IDGenerator
	w.units = next
	w.mu.Unlock()
	return nil
}

func fixedInputDelta(raw int64) float32 {
	value := float64(raw) / trueSyncFixedScale
	value *= float64(lockstepInputMoveNumerator) / float64(lockstepInputMoveDenominator)
	return float32(value)
}

func validFloat3(value etmath.Vector3) bool {
	return finiteFloat32(value.X) && finiteFloat32(value.Y) && finiteFloat32(value.Z)
}

func validQuaternion(value etmath.Quaternion) bool {
	if !finiteFloat32(value.X) || !finiteFloat32(value.Y) ||
		!finiteFloat32(value.Z) || !finiteFloat32(value.W) {
		return false
	}
	return value != (etmath.Quaternion{})
}

func finiteFloat32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
