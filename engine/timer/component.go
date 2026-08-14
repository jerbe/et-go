package timer

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// TimerComponent 是一个轻量的 ECS 计时器组件。
type TimerComponent struct {
	ecs.BaseComponent

	mu     sync.Mutex
	timers map[int64]*timerHandle
	nextID atomic.Int64
	closed bool
}

type timerHandle struct {
	cb             Callback
	timer          *time.Timer
	ticker         *time.Ticker
	stopCh         chan struct{}
	stopOnce       sync.Once
	completion     chan struct{}
	completionOnce sync.Once
	repeat         bool
}

// Type 返回组件名称。
func (c *TimerComponent) Type() string { return "TimerComponent" }

// Awake 初始化内部状态。
func (c *TimerComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.timers == nil {
		c.timers = make(map[int64]*timerHandle)
	}
}

// OnDestroy 停止并清理所有定时器。
func (c *TimerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	handles := c.timers
	c.timers = nil
	c.mu.Unlock()
	for _, handle := range handles {
		c.stopHandle(handle)
	}
}

// AddTimer 添加一次性定时器，返回定时器 ID。
func (c *TimerComponent) AddTimer(delay time.Duration, cb Callback) int64 {
	if c == nil || cb == nil {
		return 0
	}
	return c.addTimer(delay, cb, nil)
}

func (c *TimerComponent) addTimer(delay time.Duration, cb Callback, completion chan struct{}) int64 {
	if c == nil {
		return 0
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0
	}
	id := c.nextID.Add(1)
	handle := &timerHandle{
		cb:         cb,
		stopCh:     make(chan struct{}),
		completion: completion,
	}
	c.timers[id] = handle
	handle.timer = time.AfterFunc(delay, func() {
		c.invokeTimer(id)
	})
	c.mu.Unlock()
	return id
}

// AddRepeatingTimer 添加周期性定时器。
func (c *TimerComponent) AddRepeatingTimer(interval time.Duration, cb Callback) int64 {
	if c == nil || interval <= 0 || cb == nil {
		return 0
	}
	c.Awake()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0
	}
	id := c.nextID.Add(1)
	handle := &timerHandle{
		cb:     cb,
		ticker: time.NewTicker(interval),
		stopCh: make(chan struct{}),
		repeat: true,
	}
	c.timers[id] = handle
	c.mu.Unlock()
	go func() {
		for {
			select {
			case <-handle.ticker.C:
				if handle.cb != nil {
					c.invoke(handle.cb)
				}
			case <-handle.stopCh:
				return
			}
		}
	}()
	return id
}

// RemoveTimer 停止指定定时器。
func (c *TimerComponent) RemoveTimer(id int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	handle, ok := c.timers[id]
	if ok {
		delete(c.timers, id)
	}
	c.mu.Unlock()
	if ok {
		c.stopHandle(handle)
	}
}

// WaitAsync 返回在 delay 之后关闭的 channel。
func (c *TimerComponent) WaitAsync(delay time.Duration) <-chan struct{} {
	done := make(chan struct{})
	if c == nil {
		close(done)
		return done
	}
	if c.addTimer(delay, nil, done) == 0 {
		close(done)
	}
	return done
}

func (c *TimerComponent) invokeTimer(id int64) {
	c.mu.Lock()
	handle, ok := c.timers[id]
	if ok {
		delete(c.timers, id)
	}
	c.mu.Unlock()
	if ok && handle.cb != nil {
		c.invoke(handle.cb)
	}
	c.stopHandle(handle)
}

func (c *TimerComponent) invoke(cb Callback) {
	if c == nil || cb == nil {
		return
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return
	}
	entity := c.GetEntity()
	if entity == nil || entity.Scene() == nil || entity.Scene().Fiber() == nil {
		// 独立 ECS 单测没有 Fiber 所有权；生产 Scene 由下面的 Call 串行执行。
		cb()
		return
	}
	fiberRef, ok := entity.Scene().Fiber().(interface {
		Call(context.Context, func() ([]byte, error)) ([]byte, error)
	})
	if !ok || fiberRef == nil {
		slog.Error("timer callback dispatch failed: scene fiber does not support Call")
		return
	}
	callbackContext := context.Background()
	if provider, ok := entity.Scene().Fiber().(interface{ Context() context.Context }); ok {
		if ctx := provider.Context(); ctx != nil {
			callbackContext = ctx
		}
	}
	if _, err := fiberRef.Call(callbackContext, func() ([]byte, error) {
		cb()
		return nil, nil
	}); err != nil {
		slog.Error("timer callback dispatch failed", "err", err)
	}
}

func (c *TimerComponent) stopHandle(handle *timerHandle) {
	if handle == nil {
		return
	}
	if handle.timer != nil {
		handle.timer.Stop()
	}
	if handle.ticker != nil {
		handle.ticker.Stop()
	}
	handle.stopOnce.Do(func() {
		close(handle.stopCh)
	})
	if handle.completion != nil {
		handle.completionOnce.Do(func() {
			close(handle.completion)
		})
	}
}
