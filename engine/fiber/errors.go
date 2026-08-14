package fiber

import "errors"

var (
	// ErrFiberClosed 表示目标 Fiber 已经停止接收消息。
	ErrFiberClosed = errors.New("fiber: closed")
	// ErrMailboxFull 表示消息队列已满，调用方必须处理背压或失败。
	ErrMailboxFull = errors.New("fiber: mailbox full")
	// ErrTaskQueueFull 表示 Fiber 任务队列已满。
	ErrTaskQueueFull = errors.New("fiber: task queue full")
	// ErrFrameTaskRequired 表示帧结束任务为空。
	ErrFrameTaskRequired = errors.New("fiber: frame-finish task required")
	// ErrFiberTaskPanic 表示 Fiber 任务执行时发生 panic。
	ErrFiberTaskPanic = errors.New("fiber: task panicked")
)
