package timer

import (
	"container/heap"
	"sync"
	"time"
)

// Callback 定时器回调函数
type Callback func()

// entry 定时器条目
type entry struct {
	deadline time.Time
	callback Callback
	id       int64
	index    int // heap index
}

// timerHeap 最小堆实现
type timerHeap []*entry

func (h timerHeap) Len() int           { return len(h) }
func (h timerHeap) Less(i, j int) bool { return h[i].deadline.Before(h[j].deadline) }
func (h timerHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *timerHeap) Push(x any)        { e := x.(*entry); e.index = len(*h); *h = append(*h, e) }
func (h *timerHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*h = old[:n-1]
	return e
}

// Manager 定时器管理器。
// 对应 ET 框架的 TimerComponent。
type Manager struct {
	mu    sync.Mutex
	heap  timerHeap
	idGen int64
}

// NewManager 创建定时器管理器
func NewManager() *Manager {
	m := &Manager{}
	heap.Init(&m.heap)
	return m
}

// AddTimer 添加一次性定时器，返回定时器 ID
func (m *Manager) AddTimer(delay time.Duration, cb Callback) int64 {
	if m == nil || cb == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idGen++
	e := &entry{
		deadline: time.Now().Add(delay),
		callback: cb,
		id:       m.idGen,
	}
	heap.Push(&m.heap, e)
	return e.id
}

// Tick 检查并执行到期的定时器。应在主循环中调用。
func (m *Manager) Tick() {
	now := time.Now()
	m.mu.Lock()
	var expired []Callback
	for m.heap.Len() > 0 && !m.heap[0].deadline.After(now) {
		e := heap.Pop(&m.heap).(*entry)
		expired = append(expired, e.callback)
	}
	m.mu.Unlock()

	for _, cb := range expired {
		cb()
	}
}
