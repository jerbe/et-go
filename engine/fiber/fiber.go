package fiber

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// fiberIDGen 全局 Fiber ID 生成器
var fiberIDGen atomic.Int64

const (
	// DefaultFrameInterval 默认帧间隔（约 30 FPS）。
	DefaultFrameInterval = 33 * time.Millisecond
)

// Fiber 是轻量级执行上下文，对应 ET 框架的 Fiber 概念。
// 在 Go 中用 goroutine + channel 实现，每个 Fiber 拥有独立的 Scene 和消息队列。
type Fiber struct {
	id        int64
	zone      int
	processID int
	sceneType ecs.SceneType
	root      *ecs.Scene
	manager   *Manager

	ctx    context.Context
	cancel context.CancelFunc

	// 消息邮箱
	mailbox chan Message
	tasks   chan fiberTask

	// 更新注册
	updateSystems     []ecs.UpdateSystem
	lateUpdateSystems []ecs.LateUpdateSystem
	mu                sync.RWMutex

	frameInterval      time.Duration
	frameFinishTasks   []func()
	taskMu             sync.Mutex
	frameFinishWaiters []chan struct{}
	waiterMu           sync.Mutex
	frameWaitersClosed bool
	disposeOnce        sync.Once
	doneOnce           sync.Once

	stateMu       sync.RWMutex
	mailboxClosed bool
	stopping      bool
	done          chan struct{}
	started       atomic.Bool
}

// Message 是 Fiber 邮箱中传递的消息
type Message struct {
	From    int64 // 发送方 ActorID
	To      int64 // 接收方 ActorID
	MsgID   uint16
	Payload []byte
	RpcID   uint32 // 用于 RPC 请求/响应匹配
	Reply   chan<- MessageResponse
}

// MessageResponse 是 Fiber 消息处理结果。
type MessageResponse struct {
	Payload []byte
	Err     error
}

type fiberTask struct {
	ctx    context.Context
	run    func() ([]byte, error)
	result chan<- MessageResponse
}

// New 创建新的 Fiber
func New(parentCtx context.Context, sceneType ecs.SceneType, zone int, processID int) *Fiber {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	id := fiberIDGen.Add(1)

	f := &Fiber{
		id:            id,
		zone:          zone,
		processID:     processID,
		sceneType:     sceneType,
		ctx:           ctx,
		cancel:        cancel,
		mailbox:       make(chan Message, 1024),
		tasks:         make(chan fiberTask, 1024),
		done:          make(chan struct{}),
		frameInterval: DefaultFrameInterval,
	}
	f.root = ecs.NewScene(sceneType, zone, sceneType.String())
	f.root.SetFiber(f)
	return f
}

// ID 返回 Fiber ID
func (f *Fiber) ID() int64 { return f.id }

// Zone 返回分区 ID
func (f *Fiber) Zone() int { return f.zone }

// ProcessID 返回进程 ID
func (f *Fiber) ProcessID() int { return f.processID }

// SceneType 返回场景类型
func (f *Fiber) SceneType() ecs.SceneType { return f.sceneType }

// Root 返回根 Scene
func (f *Fiber) Root() *ecs.Scene { return f.root }

// Context 返回 Fiber 生命周期上下文。
func (f *Fiber) Context() context.Context {
	if f == nil || f.ctx == nil {
		return context.Background()
	}
	return f.ctx
}

// Manager 返回创建该 Fiber 的 Manager。
func (f *Fiber) Manager() *Manager {
	if f == nil {
		return nil
	}
	return f.manager
}

// SetFrameInterval 设置帧间隔，非正值将回退到默认值。
func (f *Fiber) SetFrameInterval(interval time.Duration) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if interval <= 0 {
		f.frameInterval = DefaultFrameInterval
		return
	}
	f.frameInterval = interval
}

// Send 向 Fiber 邮箱发送消息（非阻塞）。
// 队列关闭或已满时返回错误，不再静默丢弃消息。
func (f *Fiber) Send(msg Message) error {
	if f == nil {
		return ErrFiberClosed
	}
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	if f.mailboxClosed || f.ctx.Err() != nil {
		return ErrFiberClosed
	}
	select {
	case f.mailbox <- msg:
		return nil
	default:
		return ErrMailboxFull
	}
}

// Call 将函数排入目标 Fiber 的执行队列，并等待其在 Fiber goroutine 中完成。
// 调用方不得在 run 中阻塞等待同一个 Fiber 的其他任务，否则会形成自等待。
func (f *Fiber) Call(ctx context.Context, run func() ([]byte, error)) ([]byte, error) {
	if f == nil || run == nil {
		return nil, ErrFiberClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make(chan MessageResponse, 1)
	f.stateMu.RLock()
	if f.mailboxClosed || f.ctx.Err() != nil {
		f.stateMu.RUnlock()
		return nil, ErrFiberClosed
	}
	select {
	case f.tasks <- fiberTask{ctx: ctx, run: run, result: result}:
		f.stateMu.RUnlock()
	case <-ctx.Done():
		f.stateMu.RUnlock()
		return nil, ctx.Err()
	default:
		f.stateMu.RUnlock()
		return nil, ErrTaskQueueFull
	}

	select {
	case response := <-result:
		return response.Payload, response.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// RegisterUpdate 注册帧更新系统
func (f *Fiber) RegisterUpdate(sys ecs.UpdateSystem) {
	if f == nil || sys == nil {
		return
	}
	target := systemPointer(sys)
	f.mu.Lock()
	defer f.mu.Unlock()
	if target != 0 {
		for _, current := range f.updateSystems {
			if systemPointer(current) == target {
				return
			}
		}
	}
	f.updateSystems = append(f.updateSystems, sys)
}

// RegisterLateUpdate 注册延迟更新系统
func (f *Fiber) RegisterLateUpdate(sys ecs.LateUpdateSystem) {
	if f == nil || sys == nil {
		return
	}
	target := systemPointer(sys)
	f.mu.Lock()
	defer f.mu.Unlock()
	if target != 0 {
		for _, current := range f.lateUpdateSystems {
			if systemPointer(current) == target {
				return
			}
		}
	}
	f.lateUpdateSystems = append(f.lateUpdateSystems, sys)
}

// UnregisterUpdate 注销帧更新系统。
func (f *Fiber) UnregisterUpdate(sys ecs.UpdateSystem) {
	if f == nil || sys == nil {
		return
	}
	target := systemPointer(sys)
	if target == 0 {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for index, current := range f.updateSystems {
		if systemPointer(current) != target {
			continue
		}
		f.updateSystems = append(f.updateSystems[:index], f.updateSystems[index+1:]...)
		return
	}
}

// UnregisterLateUpdate 注销延迟更新系统。
func (f *Fiber) UnregisterLateUpdate(sys ecs.LateUpdateSystem) {
	if f == nil || sys == nil {
		return
	}
	target := systemPointer(sys)
	if target == 0 {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	for index, current := range f.lateUpdateSystems {
		if systemPointer(current) != target {
			continue
		}
		f.lateUpdateSystems = append(f.lateUpdateSystems[:index], f.lateUpdateSystems[index+1:]...)
		return
	}
}

// AddFrameFinishTask 添加帧结束任务。
//
// Fiber 停止后拒绝新任务，避免调用方收到 accepted 但任务永远不会执行。
func (f *Fiber) AddFrameFinishTask(task func()) error {
	if f == nil {
		return ErrFiberClosed
	}
	if task == nil {
		return ErrFrameTaskRequired
	}
	f.taskMu.Lock()
	defer f.taskMu.Unlock()
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	if f.stopping || f.mailboxClosed || f.ctx.Err() != nil {
		return ErrFiberClosed
	}
	f.frameFinishTasks = append(f.frameFinishTasks, task)
	return nil
}

// WaitFrameFinish 返回帧结束信号，关闭 channel 表示当前帧完成。
func (f *Fiber) WaitFrameFinish() <-chan struct{} {
	waiter := make(chan struct{})
	if f == nil {
		close(waiter)
		return waiter
	}
	f.waiterMu.Lock()
	f.stateMu.RLock()
	stopping := f.stopping || f.mailboxClosed || f.ctx.Err() != nil
	f.stateMu.RUnlock()
	if f.frameWaitersClosed || !f.started.Load() || stopping {
		close(waiter)
	} else {
		f.frameFinishWaiters = append(f.frameFinishWaiters, waiter)
	}
	f.waiterMu.Unlock()
	return waiter
}

// Run 启动 Fiber 主循环（在独立 goroutine 中运行）
func (f *Fiber) Run(handler MessageHandler) {
	if f == nil {
		return
	}
	f.stateMu.RLock()
	stopping := f.stopping || f.mailboxClosed || f.ctx.Err() != nil
	f.stateMu.RUnlock()
	if stopping {
		return
	}
	if !f.started.CompareAndSwap(false, true) {
		return
	}
	go f.loop(handler)
}

// MessageHandler 消息处理回调
type MessageHandler func(f *Fiber, msg Message)

// loop Fiber 主循环
func (f *Fiber) loop(handler MessageHandler) {
	ticker := time.NewTicker(f.getFrameInterval())
	defer ticker.Stop()
	defer f.closeDone()
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("Fiber 主循环 panic", "fiber_id", f.ID(), "panic", recovered)
		}
		f.dispose(handler)
	}()

	for {
		select {
		case <-f.ctx.Done():
			return
		case msg, ok := <-f.mailbox:
			if ok && handler != nil {
				safeInvoke("Fiber 消息处理器", func() {
					handler(f, msg)
				})
			}
		case task, ok := <-f.tasks:
			if !ok {
				f.failTasks()
				return
			}
			if task.run == nil {
				task.result <- MessageResponse{Err: ErrFiberClosed}
				continue
			}
			if task.ctx != nil {
				select {
				case <-task.ctx.Done():
					task.result <- MessageResponse{Err: task.ctx.Err()}
					continue
				default:
				}
			}
			payload, err := executeTask(task.run)
			task.result <- MessageResponse{Payload: payload, Err: err}
		case <-ticker.C:
			f.drainMailbox(handler)
			f.runUpdateSystems()
			f.runLateUpdateSystems()
			f.runFrameFinishTasks()
		}
	}
}

func (f *Fiber) closeDone() {
	if f == nil {
		return
	}
	f.doneOnce.Do(func() {
		close(f.done)
	})
}

func executeTask(run func() ([]byte, error)) (payload []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrFiberTaskPanic, recovered)
			payload = nil
		}
	}()
	return run()
}

func (f *Fiber) getFrameInterval() time.Duration {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.frameInterval <= 0 {
		return DefaultFrameInterval
	}
	return f.frameInterval
}

func (f *Fiber) runUpdateSystems() {
	f.mu.RLock()
	systems := append([]ecs.UpdateSystem(nil), f.updateSystems...)
	f.mu.RUnlock()

	for _, system := range systems {
		if system != nil {
			safeInvoke("Fiber Update 系统", system.Update)
		}
	}
}

func (f *Fiber) runLateUpdateSystems() {
	f.mu.RLock()
	systems := append([]ecs.LateUpdateSystem(nil), f.lateUpdateSystems...)
	f.mu.RUnlock()

	for _, system := range systems {
		if system != nil {
			safeInvoke("Fiber LateUpdate 系统", system.LateUpdate)
		}
	}
}

func (f *Fiber) drainMailbox(handler MessageHandler) {
	for {
		select {
		case msg, ok := <-f.mailbox:
			if !ok {
				return
			}
			if handler != nil {
				safeInvoke("Fiber 消息处理器", func() {
					handler(f, msg)
				})
			}
		default:
			return
		}
	}
}

// Stop 停止 Fiber
func (f *Fiber) Stop() {
	if f == nil {
		return
	}
	f.RequestStop()
	if !f.started.Load() {
		f.dispose(nil)
		f.closeDone()
		return
	}
	if !f.Wait(5 * time.Second) {
		slog.Warn("Fiber 停止等待超时", "fiber_id", f.ID())
	}
}

// RequestStop 请求停止 Fiber，但不等待 loop 结束。
//
// 该方法用于 Fiber 自身的生命周期回收路径；在当前 Fiber goroutine
// 内调用时不能同步 Wait，否则会等待自己退出。
func (f *Fiber) RequestStop() {
	if f == nil || f.cancel == nil {
		return
	}
	f.stateMu.Lock()
	f.stopping = true
	f.stateMu.Unlock()
	f.cancel()
}

// Wait 等待 Fiber 停止，返回是否在超时前结束。
func (f *Fiber) Wait(timeout time.Duration) bool {
	if f == nil {
		return true
	}
	if timeout <= 0 {
		if !f.started.Load() {
			return true
		}
		<-f.done
		return true
	}
	select {
	case <-f.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (f *Fiber) runFrameFinishTasks() {
	f.taskMu.Lock()
	tasks := f.frameFinishTasks
	f.frameFinishTasks = nil
	f.taskMu.Unlock()

	f.waiterMu.Lock()
	waiters := f.frameFinishWaiters
	f.frameFinishWaiters = nil
	f.waiterMu.Unlock()

	for _, task := range tasks {
		if task != nil {
			safeInvoke("Fiber 帧结束任务", task)
		}
	}

	for _, waiter := range waiters {
		if waiter != nil {
			close(waiter)
		}
	}
}

// dispose 清理 Fiber 资源
func (f *Fiber) dispose(handler MessageHandler) {
	if f == nil {
		return
	}
	f.disposeOnce.Do(func() {
		f.drainMailbox(handler)
		f.runFrameFinishTasks()
		f.closeFrameFinishWaiters()

		if f.root != nil {
			f.root.Dispose()
		}

		f.stateMu.Lock()
		if !f.mailboxClosed {
			close(f.mailbox)
			f.mailboxClosed = true
		}
		f.stateMu.Unlock()
		f.failTasks()
	})
}

func safeInvoke(name string, fn func()) {
	if fn == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error(name+" panic", "panic", recovered)
		}
	}()
	fn()
}

func (f *Fiber) failTasks() {
	for {
		select {
		case task := <-f.tasks:
			if task.result != nil {
				task.result <- MessageResponse{Err: ErrFiberClosed}
			}
		default:
			return
		}
	}
}

func (f *Fiber) closeFrameFinishWaiters() {
	f.waiterMu.Lock()
	if f.frameWaitersClosed {
		f.waiterMu.Unlock()
		return
	}
	f.frameWaitersClosed = true
	waiters := f.frameFinishWaiters
	f.frameFinishWaiters = nil
	f.waiterMu.Unlock()

	for _, waiter := range waiters {
		if waiter != nil {
			close(waiter)
		}
	}
}

func systemPointer(value any) uintptr {
	reflectValue := reflect.ValueOf(value)
	if !reflectValue.IsValid() {
		return 0
	}
	switch reflectValue.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Chan, reflect.Map, reflect.Slice, reflect.UnsafePointer:
		return reflectValue.Pointer()
	default:
		return 0
	}
}
