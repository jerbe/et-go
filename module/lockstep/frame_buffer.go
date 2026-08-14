package lockstep

import (
	"errors"
	"sync"
)

var ErrFrameOutOfRange = errors.New("lockstep: frame out of range")

type TSVector2 struct {
	X int64
	Y int64
}

type LSInput struct {
	V      TSVector2
	Button int32
}

type OneFrameInputs struct {
	Inputs map[int64]*LSInput
}

func NewOneFrameInputs() *OneFrameInputs {
	return &OneFrameInputs{Inputs: make(map[int64]*LSInput)}
}

func (f *OneFrameInputs) IsEmpty() bool {
	if f == nil {
		return true
	}
	return len(f.Inputs) == 0
}

type FrameBuffer struct {
	mu        sync.RWMutex
	frames    []*OneFrameInputs
	maxFrame  int
	hashs     []int64
	hashSet   []bool
	snapshots map[int][]byte
}

func NewFrameBuffer() *FrameBuffer {
	return &FrameBuffer{
		frames:    make([]*OneFrameInputs, 0),
		hashs:     make([]int64, 0),
		hashSet:   make([]bool, 0),
		snapshots: make(map[int][]byte),
	}
}

func (fb *FrameBuffer) SetFrameInputs(frame int, inputs *OneFrameInputs) {
	if fb == nil || frame <= 0 {
		return
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.ensureFrameCapacity(frame)
	fb.frames[frame-1] = cloneFrameInputs(inputs)
	if frame > fb.maxFrame {
		fb.maxFrame = frame
	}
}

func (fb *FrameBuffer) GetFrameInputs(frame int) (*OneFrameInputs, bool) {
	if fb == nil || frame <= 0 {
		return nil, false
	}
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	if frame-1 >= len(fb.frames) || fb.frames[frame-1] == nil {
		return nil, false
	}
	return cloneFrameInputs(fb.frames[frame-1]), true
}

func (fb *FrameBuffer) SetHash(frame int, hash int64) {
	if fb == nil || frame <= 0 {
		return
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.ensureFrameCapacity(frame)
	fb.hashs[frame-1] = hash
	fb.hashSet[frame-1] = true
}

func (fb *FrameBuffer) GetHash(frame int) (int64, bool) {
	if fb == nil || frame <= 0 {
		return 0, false
	}
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	if frame-1 >= len(fb.hashs) || !fb.hashSet[frame-1] {
		return 0, false
	}
	return fb.hashs[frame-1], true
}

// CheckAndSetHash 原子地检查帧哈希并记录首次收到的值。
//
// Room Fiber 使用无序邮箱时，单独调用 GetHash 和 SetHash 会让两个并发
// 请求都观察到“尚未记录”，从而丢失真正的 mismatch。
func (fb *FrameBuffer) CheckAndSetHash(frame int, hash int64) (previous int64, exists bool, mismatch bool) {
	if fb == nil || frame <= 0 {
		return 0, false, false
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.ensureFrameCapacity(frame)
	if fb.hashSet[frame-1] {
		previous = fb.hashs[frame-1]
		return previous, true, previous != hash
	}
	fb.hashs[frame-1] = hash
	fb.hashSet[frame-1] = true
	return 0, false, false
}

func (fb *FrameBuffer) SetSnapshot(frame int, data []byte) {
	if fb == nil || frame < 0 {
		return
	}
	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.snapshots == nil {
		fb.snapshots = make(map[int][]byte)
	}
	fb.snapshots[frame] = append([]byte(nil), data...)
}

func (fb *FrameBuffer) GetNearestSnapshot(frame int) (int, []byte, bool) {
	if fb == nil || frame < 0 {
		return 0, nil, false
	}
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	if len(fb.snapshots) == 0 {
		return 0, nil, false
	}
	best := -1
	for f := range fb.snapshots {
		if f <= frame && f > best {
			best = f
		}
	}
	if best < 0 {
		return 0, nil, false
	}
	data := append([]byte(nil), fb.snapshots[best]...)
	return best, data, true
}

func (fb *FrameBuffer) MaxFrame() int {
	if fb == nil {
		return 0
	}
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return fb.maxFrame
}

func (fb *FrameBuffer) ensureFrameCapacity(frame int) {
	for len(fb.frames) < frame {
		fb.frames = append(fb.frames, nil)
	}
	for len(fb.hashs) < frame {
		fb.hashs = append(fb.hashs, 0)
		fb.hashSet = append(fb.hashSet, false)
	}
}
