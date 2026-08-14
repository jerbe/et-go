package actor

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// MailBoxType 邮箱类型。
type MailBoxType int

const (
	// MailBoxTypeUnOrderedMessage 无序消息邮箱（Core*1000+1）。
	MailBoxTypeUnOrderedMessage MailBoxType = 1001
	// MailBoxTypeOrderedMessage 有序消息邮箱（ActorLocation*1000+1）。
	MailBoxTypeOrderedMessage MailBoxType = 3001
	// MailBoxTypeGateSession Gate 会话邮箱（Login*1000+1）。
	MailBoxTypeGateSession MailBoxType = 9001

	// MailBoxTypeUnordered 兼容旧命名。
	MailBoxTypeUnordered MailBoxType = MailBoxTypeUnOrderedMessage
)

// Handler 表示消息处理器函数。
type Handler func(actorID ActorID, msgID uint16, payload []byte) ([]byte, error)

// Dispatcher 表示按邮箱类型处理消息的分发器。
type Dispatcher interface {
	Handle(entity *ecs.Entity, actorID ActorID, msgID uint16, payload []byte) ([]byte, error)
}

var (
	dispatcherMu sync.RWMutex
	dispatchers  = make(map[MailBoxType]Dispatcher)
)

// MailBox 是 Actor 消息组件。
type MailBox struct {
	ecs.BaseComponent

	actorID  ActorID
	typ      MailBoxType
	handlers map[uint16]Handler
	mu       sync.RWMutex
	closed   bool
}

// NewMailBox 创建 MailBox。
func NewMailBox(actorID ActorID, typ MailBoxType) *MailBox {
	mb := &MailBox{
		actorID:  actorID,
		typ:      typ,
		handlers: make(map[uint16]Handler),
	}
	return mb
}

// Type 返回组件名称。
func (mb *MailBox) Type() string { return "MailBox" }

// Awake 初始化内部状态。
func (mb *MailBox) Awake() {
	if mb == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	if mb.handlers == nil {
		mb.handlers = make(map[uint16]Handler)
	}
}

// OnDestroy 清理内部状态。
func (mb *MailBox) OnDestroy() {
	if mb == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.closed = true
	mb.handlers = nil
}

// SetActorID 设置 ActorID。
func (mb *MailBox) SetActorID(actorID ActorID) {
	if mb == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	mb.actorID = actorID
}

// ActorID 返回邮箱对应的 ActorID。
func (mb *MailBox) ActorID() ActorID {
	if mb == nil {
		return ActorID{}
	}
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.actorID
}

// SetMailBoxType 设置邮箱类型。
func (mb *MailBox) SetMailBoxType(typ MailBoxType) {
	if mb == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	mb.typ = typ
}

// MailBoxType 返回邮箱类型。
func (mb *MailBox) MailBoxType() MailBoxType {
	if mb == nil {
		return 0
	}
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return mb.typ
}

// RegisterMailBoxDispatcher 注册指定邮箱类型的默认分发器。
func RegisterMailBoxDispatcher(typ MailBoxType, dispatcher Dispatcher) {
	dispatcherMu.Lock()
	defer dispatcherMu.Unlock()
	if dispatcher == nil {
		delete(dispatchers, typ)
		return
	}
	dispatchers[typ] = dispatcher
}

// RegisterHandler 注册消息处理器。
func (mb *MailBox) RegisterHandler(msgID uint16, handler Handler) {
	if mb == nil || handler == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	if mb.handlers == nil {
		mb.handlers = make(map[uint16]Handler)
	}
	mb.handlers[msgID] = handler
}

// UnregisterHandler 注销消息处理器。
func (mb *MailBox) UnregisterHandler(msgID uint16) {
	if mb == nil {
		return
	}
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	if mb.handlers == nil {
		return
	}
	delete(mb.handlers, msgID)
}

// Dispatch 按 MsgID 分发消息。
func (mb *MailBox) Dispatch(msgID uint16, payload []byte) ([]byte, error) {
	if mb == nil {
		return nil, ErrActorNotFound
	}
	mb.mu.RLock()
	if mb.closed {
		mb.mu.RUnlock()
		return nil, ErrActorNotFound
	}
	handler, ok := mb.handlers[msgID]
	actorID := mb.actorID
	typ := mb.typ
	mb.mu.RUnlock()

	if !ok {
		dispatcherMu.RLock()
		dispatcher := dispatchers[typ]
		dispatcherMu.RUnlock()
		if dispatcher == nil {
			return nil, ErrHandlerNotFound
		}
		return dispatcher.Handle(mb.GetEntity(), actorID, msgID, payload)
	}
	return handler(actorID, msgID, payload)
}
