package actor

import (
	"context"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

// ProcessInnerSender 负责同进程 Actor 消息投递。
type ProcessInnerSender struct {
	ecs.BaseComponent

	processID   int
	fiberGetter interface {
		Get(id int64) (*fiber.Fiber, bool)
	}
	rpcMgr *RpcManager
}

// NewProcessInnerSender 创建同进程发送器。
func NewProcessInnerSender(processID int, getter interface {
	Get(id int64) (*fiber.Fiber, bool)
}, rpcMgr *RpcManager) *ProcessInnerSender {
	return &ProcessInnerSender{
		processID:   processID,
		fiberGetter: getter,
		rpcMgr:      rpcMgr,
	}
}

// Type 返回组件类型名。
func (s *ProcessInnerSender) Type() string { return "ProcessInnerSender" }

// Awake 初始化默认 RPC 管理器。
func (s *ProcessInnerSender) Awake() {
	if s == nil {
		return
	}
	if s.rpcMgr == nil {
		s.rpcMgr = NewRpcManager()
	}
	if s.fiberGetter == nil {
		s.fiberGetter = fiber.DefaultManager()
	}
}

// SetFiberManager 设置 Fiber 查询器。
func (s *ProcessInnerSender) SetFiberManager(getter interface {
	Get(id int64) (*fiber.Fiber, bool)
}) {
	if s == nil {
		return
	}
	s.fiberGetter = getter
}

// SetRPCManager 设置 RPC 管理器。
func (s *ProcessInnerSender) SetRPCManager(mgr *RpcManager) {
	if s == nil {
		return
	}
	s.rpcMgr = mgr
}

// Send 发送同进程单向消息。
func (s *ProcessInnerSender) Send(actorID ActorID, msgID uint16, payload []byte) error {
	if s == nil {
		return ErrFiberManagerMissing
	}
	if !actorID.IsValid() {
		return ErrActorNotFound
	}
	if s.fiberGetter == nil {
		return ErrFiberManagerMissing
	}
	targetFiber, ok := s.fiberGetter.Get(actorID.FiberID)
	if !ok || targetFiber == nil {
		return ErrActorNotFound
	}
	if err := targetFiber.Send(fiber.Message{
		From:    int64(s.processID),
		To:      actorID.InstanceID,
		MsgID:   msgID,
		Payload: append([]byte(nil), payload...),
	}); err != nil {
		return err
	}
	return nil
}

// Call 发送同进程 RPC 并等待响应。
func (s *ProcessInnerSender) Call(ctx context.Context, actorID ActorID, msgID uint16, payload []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrFiberManagerMissing
	}
	if !actorID.IsValid() {
		return nil, ErrActorNotFound
	}
	if s.fiberGetter == nil {
		return nil, ErrFiberManagerMissing
	}
	targetFiber, ok := s.fiberGetter.Get(actorID.FiberID)
	if !ok || targetFiber == nil {
		return nil, ErrActorNotFound
	}
	if s.rpcMgr == nil {
		s.rpcMgr = NewRpcManager()
	}

	timeout := defaultRPCTimeout
	if s.rpcMgr.timeout > 0 {
		timeout = s.rpcMgr.timeout
	}
	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(callCtx, timeout)
	defer cancel()

	return targetFiber.Call(timeoutCtx, func() ([]byte, error) {
		return DispatchFiberMessage(targetFiber.Root(), fiber.Message{
			From:    int64(s.processID),
			To:      actorID.InstanceID,
			MsgID:   msgID,
			Payload: append([]byte(nil), payload...),
			RpcID:   1,
		})
	})
}
