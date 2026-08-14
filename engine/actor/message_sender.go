package actor

import (
	"context"

	"github.com/jerbe/et-go/engine/ecs"
)

// MessageSender 是统一消息发送入口。
type MessageSender struct {
	ecs.BaseComponent

	processID   int
	innerSender *ProcessInnerSender
	outerSender *ProcessOuterSender
}

// NewMessageSender 创建 MessageSender。
func NewMessageSender(processID int, inner *ProcessInnerSender, outer *ProcessOuterSender) *MessageSender {
	return &MessageSender{
		processID:   processID,
		innerSender: inner,
		outerSender: outer,
	}
}

// Type 返回组件类型名。
func (m *MessageSender) Type() string { return "MessageSender" }

// SetProcessID 设置当前进程 ID。
func (m *MessageSender) SetProcessID(processID int) {
	if m == nil {
		return
	}
	m.processID = processID
}

// SetInnerSender 设置同进程发送器。
func (m *MessageSender) SetInnerSender(sender *ProcessInnerSender) {
	if m == nil {
		return
	}
	m.innerSender = sender
}

// SetOuterSender 设置跨进程发送器。
func (m *MessageSender) SetOuterSender(sender *ProcessOuterSender) {
	if m == nil {
		return
	}
	m.outerSender = sender
}

// Send 发送单向消息。
func (m *MessageSender) Send(actorID ActorID, msgID uint16, payload []byte) error {
	if m == nil || !actorID.IsValid() {
		return ErrActorNotFound
	}
	if actorID.ProcessID == m.processID {
		if m.innerSender == nil {
			return ErrActorNotFound
		}
		return m.innerSender.Send(actorID, msgID, payload)
	}
	outerSender := m.outerSender
	if outerSender == nil {
		outerSender = ResolveProcessOuterSender(m.processID)
	}
	if outerSender == nil {
		return ErrActorNotFound
	}
	return outerSender.Send(actorID, msgID, payload)
}

// Call 发送 RPC 请求并等待响应。
func (m *MessageSender) Call(ctx context.Context, actorID ActorID, msgID uint16, payload []byte) ([]byte, error) {
	if m == nil || !actorID.IsValid() {
		return nil, ErrActorNotFound
	}
	if actorID.ProcessID == m.processID {
		if m.innerSender == nil {
			return nil, ErrActorNotFound
		}
		return m.innerSender.Call(ctx, actorID, msgID, payload)
	}
	outerSender := m.outerSender
	if outerSender == nil {
		outerSender = ResolveProcessOuterSender(m.processID)
	}
	if outerSender == nil {
		return nil, ErrActorNotFound
	}
	return outerSender.Call(ctx, actorID, msgID, payload)
}
