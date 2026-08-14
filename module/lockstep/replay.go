package lockstep

import (
	"sync"

	"go.mongodb.org/mongo-driver/bson"
)

type Replay struct {
	mu        sync.RWMutex
	unitInfos [][]byte
	inputs    []*OneFrameInputs
	snapshots map[int][]byte
}

func (r *Replay) UnitInfos() [][]byte {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneByteSlices(r.unitInfos)
}

func (r *Replay) SetUnitInfos(unitInfos [][]byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unitInfos = cloneByteSlices(unitInfos)
}

func NewReplay() *Replay {
	return &Replay{
		snapshots: make(map[int][]byte),
	}
}

func (r *Replay) AddFrameInput(inputs *OneFrameInputs) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs = append(r.inputs, cloneFrameInputs(inputs))
}

func (r *Replay) AddSnapshot(frame int, data []byte) {
	if r == nil || frame < 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snapshots == nil {
		r.snapshots = make(map[int][]byte)
	}
	r.snapshots[frame] = append([]byte(nil), data...)
}

func (r *Replay) GetFrameInputsRange(from, to int) []*OneFrameInputs {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if from < 0 {
		from = 0
	}
	if to > len(r.inputs) {
		to = len(r.inputs)
	}
	if from >= to {
		return nil
	}
	result := make([]*OneFrameInputs, 0, to-from)
	for _, input := range r.inputs[from:to] {
		result = append(result, cloneFrameInputs(input))
	}
	return result
}

func (r *Replay) Serialize() ([]byte, error) {
	if r == nil {
		return nil, ErrReplayMissing
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return bson.Marshal(struct {
		UnitInfos [][]byte          `bson:"unitInfos"`
		Inputs    []*OneFrameInputs `bson:"inputs"`
		Snapshots map[int][]byte    `bson:"snapshots"`
	}{
		UnitInfos: cloneByteSlices(r.unitInfos),
		Inputs:    cloneFrameInputsList(r.inputs),
		Snapshots: r.snapshotsLocked(),
	})
}

func (r *Replay) Deserialize(data []byte) error {
	if r == nil {
		return ErrReplayMissing
	}
	var payload struct {
		UnitInfos [][]byte          `bson:"unitInfos"`
		Inputs    []*OneFrameInputs `bson:"inputs"`
		Snapshots map[int][]byte    `bson:"snapshots"`
	}
	if err := bson.Unmarshal(data, &payload); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.unitInfos = cloneByteSlices(payload.UnitInfos)
	r.inputs = cloneFrameInputsList(payload.Inputs)
	r.snapshots = make(map[int][]byte, len(payload.Snapshots))
	for frame, snapshot := range payload.Snapshots {
		r.snapshots[frame] = append([]byte(nil), snapshot...)
	}
	return nil
}

func (r *Replay) Snapshots() map[int][]byte {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshotsLocked()
}

func (r *Replay) snapshotsLocked() map[int][]byte {
	copy := make(map[int][]byte, len(r.snapshots))
	for k, v := range r.snapshots {
		copy[k] = append([]byte(nil), v...)
	}
	return copy
}

func cloneByteSlices(values [][]byte) [][]byte {
	if values == nil {
		return nil
	}
	result := make([][]byte, len(values))
	for index, value := range values {
		result[index] = append([]byte(nil), value...)
	}
	return result
}

func cloneFrameInputsList(values []*OneFrameInputs) []*OneFrameInputs {
	if values == nil {
		return nil
	}
	result := make([]*OneFrameInputs, len(values))
	for index, value := range values {
		result[index] = cloneFrameInputs(value)
	}
	return result
}

func cloneFrameInputs(input *OneFrameInputs) *OneFrameInputs {
	if input == nil {
		return nil
	}
	clone := NewOneFrameInputs()
	for playerID, lsInput := range input.Inputs {
		if lsInput == nil {
			clone.Inputs[playerID] = nil
			continue
		}
		value := *lsInput
		clone.Inputs[playerID] = &value
	}
	return clone
}
