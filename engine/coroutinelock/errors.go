package coroutinelock

import "errors"

var (
	// ErrLockTimeout 表示协程锁等待超时。
	ErrLockTimeout = errors.New("coroutinelock: lock timeout")
	// ErrLockCanceled 表示协程锁等待被取消。
	ErrLockCanceled = errors.New("coroutinelock: lock canceled")
	// ErrComponentNotAwake 表示协程锁组件尚未初始化。
	ErrComponentNotAwake = errors.New("coroutinelock: component not awake")
	// ErrLockManagerMissing 表示锁管理器未注入。
	ErrLockManagerMissing = errors.New("coroutinelock: lock manager missing")
	// ErrLockContextRequired 表示等待锁必须提供 context。
	ErrLockContextRequired = errors.New("coroutinelock: context required")
)
