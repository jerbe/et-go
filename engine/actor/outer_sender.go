package actor

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network/codec"
)

// PacketSession 抽象跨进程发包能力。
type PacketSession interface {
	Send(packet *codec.Packet) error
}

// ProcessOuterSender 负责跨进程消息发送。
type ProcessOuterSender struct {
	ecs.BaseComponent

	mu        sync.RWMutex
	processID int
	sessions  map[int]PacketSession
	pending   map[uint32]int
	rpcMgr    *RpcManager
	closed    bool
	fibers    interface {
		Get(id int64) (*fiber.Fiber, bool)
	}
}

// NewProcessOuterSender 创建跨进程发送器。
func NewProcessOuterSender(rpcMgr *RpcManager) *ProcessOuterSender {
	return &ProcessOuterSender{
		sessions: make(map[int]PacketSession),
		pending:  make(map[uint32]int),
		rpcMgr:   rpcMgr,
	}
}

// Type 返回组件类型名。
func (s *ProcessOuterSender) Type() string { return "ProcessOuterSender" }

// Awake 初始化默认状态。
func (s *ProcessOuterSender) Awake() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.sessions == nil {
		s.sessions = make(map[int]PacketSession)
	}
	if s.pending == nil {
		s.pending = make(map[uint32]int)
	}
}

// SetFiberManager 注入跨进程接收端使用的本地 Fiber Manager。
func (s *ProcessOuterSender) SetFiberManager(manager interface {
	Get(id int64) (*fiber.Fiber, bool)
}) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.fibers = manager
}

// OnDestroy 释放资源。
func (s *ProcessOuterSender) OnDestroy() {
	if s == nil {
		return
	}
	s.mu.Lock()
	processID := s.processID
	s.closed = true
	pending := s.takePendingLocked(0, true)
	s.sessions = nil
	s.pending = nil
	s.fibers = nil
	s.mu.Unlock()
	UnregisterProcessOuterSender(processID, s)
	s.failPending(pending)
}

// AddSession 注册进程会话。
func (s *ProcessOuterSender) AddSession(processID int, session PacketSession) {
	if s == nil || session == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if s.sessions == nil {
		s.sessions = make(map[int]PacketSession)
	}
	if s.pending == nil {
		s.pending = make(map[uint32]int)
	}
	var pending []uint32
	if _, exists := s.sessions[processID]; exists {
		pending = s.takePendingLocked(processID, false)
	}
	s.sessions[processID] = session
	s.mu.Unlock()
	s.failPending(pending)
}

// RemoveSession 删除进程会话。
func (s *ProcessOuterSender) RemoveSession(processID int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.sessions == nil {
		s.mu.Unlock()
		return
	}
	delete(s.sessions, processID)
	pending := s.takePendingLocked(processID, false)
	s.mu.Unlock()
	s.failPending(pending)
}

// Send 跨进程单向发送。
func (s *ProcessOuterSender) Send(actorID ActorID, msgID uint16, payload []byte) error {
	if s == nil {
		return ErrActorNotFound
	}
	if !actorID.IsValid() {
		return ErrActorNotFound
	}

	session, ok := s.getSession(actorID.ProcessID)
	if !ok {
		return ErrActorNotFound
	}

	envelopePayload, err := encodeActorEnvelope(actorID, payload)
	if err != nil {
		return err
	}
	if err := session.Send(&codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   msgID,
		Payload: envelopePayload,
	}); err != nil {
		return err
	}
	return nil
}

// Call 跨进程 RPC 调用。
func (s *ProcessOuterSender) Call(ctx context.Context, actorID ActorID, msgID uint16, payload []byte) ([]byte, error) {
	if s == nil {
		return nil, ErrActorNotFound
	}
	if !actorID.IsValid() {
		return nil, ErrActorNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	session, ok := s.getSession(actorID.ProcessID)
	if !ok {
		return nil, ErrActorNotFound
	}
	if s.rpcMgr == nil {
		return nil, ErrRpcManagerMissing
	}

	timeout := defaultRPCTimeout
	if s.rpcMgr.timeout > 0 {
		timeout = s.rpcMgr.timeout
	}
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	rpcID, responseChannel := s.rpcMgr.RegisterWithTimeout(timeout)
	if !s.trackPending(rpcID, actorID.ProcessID) {
		s.rpcMgr.Remove(rpcID)
		return nil, ErrProcessSessionClosed
	}

	envelopePayload, err := encodeActorEnvelope(actorID, payload)
	if err != nil {
		s.rpcMgr.Remove(rpcID)
		s.removePending(rpcID)
		return nil, err
	}
	if err := session.Send(&codec.Packet{
		Type:    codec.PacketTypeRequest,
		MsgID:   msgID,
		RpcID:   rpcID,
		Payload: envelopePayload,
	}); err != nil {
		s.rpcMgr.Remove(rpcID)
		s.removePending(rpcID)
		return nil, err
	}

	select {
	case response := <-responseChannel:
		// RpcManager 的超时回调不会经过 ResolveRPC；这里统一清理
		// pending，避免超时 RPC 长期占用进程会话状态。
		s.removePending(rpcID)
		return response.Payload, response.Err
	case <-ctx.Done():
		s.rpcMgr.Remove(rpcID)
		s.removePending(rpcID)
		return nil, ctx.Err()
	}
}

// HandlePacket 处理从进程间连接收到的包。
//
// Message/Request 的 payload 必须包含 Actor envelope；Response 的 payload
// 直接作为 RPC 结果交给 RpcManager。Request 会通过目标 Fiber 的 Call
// 进入目标 Fiber 队列，保证与同进程消息拥有相同的串行边界。
func (s *ProcessOuterSender) HandlePacket(ctx context.Context, packet *codec.Packet) (*codec.Packet, error) {
	if s == nil {
		return nil, ErrActorNotFound
	}
	if packet == nil {
		return nil, ErrInvalidPacket
	}
	if packet.Type == codec.PacketTypeResponse {
		if packet.RpcID == 0 {
			return nil, ErrInvalidPacket
		}
		s.ResolveRPC(packet.RpcID, packet.Payload, nil)
		return nil, nil
	}
	if packet.Type != codec.PacketTypeMessage && packet.Type != codec.PacketTypeRequest {
		return nil, ErrInvalidPacket
	}
	s.mu.RLock()
	closed := s.closed
	fibers := s.fibers
	s.mu.RUnlock()
	if closed {
		return nil, ErrActorNotFound
	}
	if fibers == nil {
		return nil, ErrFiberManagerMissing
	}
	target, payload, err := DecodeActorEnvelope(packet.Payload)
	if err != nil {
		return nil, err
	}
	if !target.IsValid() {
		return nil, ErrActorNotFound
	}
	targetFiber, ok := fibers.Get(target.FiberID)
	if !ok || targetFiber == nil {
		return nil, ErrActorNotFound
	}
	message := fiber.Message{
		To:      target.InstanceID,
		MsgID:   packet.MsgID,
		Payload: append([]byte(nil), payload...),
		RpcID:   packet.RpcID,
	}
	if packet.Type == codec.PacketTypeMessage {
		return nil, targetFiber.Send(message)
	}
	if packet.RpcID == 0 {
		return nil, ErrInvalidPacket
	}
	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	responsePayload, err := targetFiber.Call(callCtx, func() ([]byte, error) {
		return DispatchFiberMessage(targetFiber.Root(), message)
	})
	if err != nil {
		return nil, err
	}
	return &codec.Packet{
		Type:    codec.PacketTypeResponse,
		MsgID:   packet.MsgID,
		RpcID:   packet.RpcID,
		Payload: responsePayload,
	}, nil
}

// HandleSessionPacket 将 Session 收到的包接入跨进程接收端。
func (s *ProcessOuterSender) HandleSessionPacket(ctx context.Context, session PacketSession, packet *codec.Packet) error {
	if session == nil {
		return ErrActorNotFound
	}
	response, err := s.HandlePacket(ctx, packet)
	if err != nil {
		return err
	}
	if response != nil {
		return session.Send(response)
	}
	return nil
}

// ResolveRPC 用于处理收到的 RPC 响应。
func (s *ProcessOuterSender) ResolveRPC(rpcID uint32, payload []byte, err error) {
	if s.rpcMgr == nil {
		return
	}
	s.removePending(rpcID)
	s.rpcMgr.Resolve(rpcID, RpcResponse{
		Payload: payload,
		Err:     err,
	})
}

func (s *ProcessOuterSender) trackPending(rpcID uint32, processID int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		return false
	}
	if _, ok := s.sessions[processID]; !ok {
		return false
	}
	if s.pending == nil {
		s.pending = make(map[uint32]int)
	}
	s.pending[rpcID] = processID
	return true
}

func (s *ProcessOuterSender) removePending(rpcID uint32) {
	s.mu.Lock()
	if s.pending != nil {
		delete(s.pending, rpcID)
	}
	s.mu.Unlock()
}

func (s *ProcessOuterSender) takePendingLocked(processID int, all bool) []uint32 {
	if s.pending == nil {
		return nil
	}
	ids := make([]uint32, 0)
	for rpcID, targetProcessID := range s.pending {
		if !all && targetProcessID != processID {
			continue
		}
		delete(s.pending, rpcID)
		ids = append(ids, rpcID)
	}
	return ids
}

func (s *ProcessOuterSender) failPending(rpcIDs []uint32) {
	if s == nil || s.rpcMgr == nil {
		return
	}
	for _, rpcID := range rpcIDs {
		s.rpcMgr.Resolve(rpcID, RpcResponse{Err: ErrProcessSessionClosed})
	}
}

func (s *ProcessOuterSender) getSession(processID int) (PacketSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.sessions == nil {
		return nil, false
	}
	session, ok := s.sessions[processID]
	return session, ok
}

func encodeActorEnvelope(actorID ActorID, payload []byte) ([]byte, error) {
	if !actorID.IsValid() {
		return nil, ErrActorNotFound
	}

	headerSize := 4 + 8 + 8
	buffer := make([]byte, headerSize+len(payload))
	binary.BigEndian.PutUint32(buffer[0:4], uint32(actorID.ProcessID))
	binary.BigEndian.PutUint64(buffer[4:12], uint64(actorID.FiberID))
	binary.BigEndian.PutUint64(buffer[12:20], uint64(actorID.InstanceID))
	copy(buffer[headerSize:], payload)
	return buffer, nil
}

// DecodeActorEnvelope 解包跨进程载荷。
func DecodeActorEnvelope(payload []byte) (ActorID, []byte, error) {
	if len(payload) < 20 {
		return ActorID{}, nil, errors.New("actor: invalid outer envelope")
	}
	actorID := ActorID{
		ProcessID:  int(binary.BigEndian.Uint32(payload[0:4])),
		FiberID:    int64(binary.BigEndian.Uint64(payload[4:12])),
		InstanceID: int64(binary.BigEndian.Uint64(payload[12:20])),
	}
	return actorID, payload[20:], nil
}
