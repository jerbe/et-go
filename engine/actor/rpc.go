package actor

import (
	"sync"
	"time"
)

const defaultRPCTimeout = 40 * time.Second

// RpcResponse 是 RPC 返回结果。
type RpcResponse struct {
	Payload []byte
	Err     error
}

// RpcOption 定义 RPC 管理器配置。
type RpcOption func(mgr *RpcManager)

// WithRPCTimeout 配置默认 RPC 超时时间。
func WithRPCTimeout(timeout time.Duration) RpcOption {
	return func(mgr *RpcManager) {
		if timeout > 0 {
			mgr.timeout = timeout
		}
	}
}

// RpcManager 负责 RPC 回调注册和匹配。
type RpcManager struct {
	mu        sync.Mutex
	nextID    uint32
	callbacks map[uint32]chan RpcResponse
	timers    map[uint32]*time.Timer
	timeout   time.Duration
}

// NewRpcManager 创建 RPC 管理器。
func NewRpcManager(opts ...RpcOption) *RpcManager {
	mgr := &RpcManager{
		callbacks: make(map[uint32]chan RpcResponse),
		timers:    make(map[uint32]*time.Timer),
		timeout:   defaultRPCTimeout,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(mgr)
		}
	}
	return mgr
}

// Register 注册 RPC 并返回 rpcID 与响应通道。
func (r *RpcManager) Register() (uint32, <-chan RpcResponse) {
	return r.RegisterWithTimeout(r.timeout)
}

// RegisterWithTimeout 注册 RPC 并指定超时。
func (r *RpcManager) RegisterWithTimeout(timeout time.Duration) (uint32, <-chan RpcResponse) {
	if timeout <= 0 {
		timeout = r.timeout
	}

	r.mu.Lock()
	r.nextID++
	if r.nextID == 0 {
		r.nextID++
	}
	rpcID := r.nextID
	channel := make(chan RpcResponse, 1)
	r.callbacks[rpcID] = channel
	timer := time.AfterFunc(timeout, func() {
		r.Resolve(rpcID, RpcResponse{Err: ErrTimeout})
	})
	r.timers[rpcID] = timer
	r.mu.Unlock()

	return rpcID, channel
}

// Resolve 解析并通知 RPC 回调。
func (r *RpcManager) Resolve(rpcID uint32, response RpcResponse) {
	r.mu.Lock()
	channel, ok := r.callbacks[rpcID]
	if ok {
		delete(r.callbacks, rpcID)
	}
	timer, hasTimer := r.timers[rpcID]
	if hasTimer {
		delete(r.timers, rpcID)
	}
	r.mu.Unlock()

	if hasTimer {
		timer.Stop()
	}
	if !ok {
		return
	}
	select {
	case channel <- response:
	default:
	}
}

// Remove 移除等待中的 RPC。
func (r *RpcManager) Remove(rpcID uint32) {
	r.mu.Lock()
	_, ok := r.callbacks[rpcID]
	if ok {
		delete(r.callbacks, rpcID)
	}
	timer, hasTimer := r.timers[rpcID]
	if hasTimer {
		delete(r.timers, rpcID)
	}
	r.mu.Unlock()

	if hasTimer {
		timer.Stop()
	}
}
