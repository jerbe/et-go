package gate

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// GateSessionActorMessageHandler 处理发往 GateSession MailBox 的 Actor 消息。
//
// 返回值中的 handled=true 表示调用方已经接管该消息；responseMsgID 为 0
// 时不向客户端发送包，适用于只更新 Gate 会话状态的内部消息。
type GateSessionActorMessageHandler func(entity *ecs.Entity, msgID uint16, payload []byte) (responseMsgID uint16, responsePayload []byte, handled bool, err error)

var (
	gateSessionHandlerMu sync.RWMutex
	gateSessionHandlers  = make(map[uint16]GateSessionActorMessageHandler)
)

// RegisterGateSessionActorMessageHandler 注册 GateSession Actor 消息转换器。
func RegisterGateSessionActorMessageHandler(msgID uint16, handler GateSessionActorMessageHandler) {
	gateSessionHandlerMu.Lock()
	defer gateSessionHandlerMu.Unlock()
	if handler == nil {
		delete(gateSessionHandlers, msgID)
		return
	}
	gateSessionHandlers[msgID] = handler
}

func gateSessionActorMessageHandler(msgID uint16) GateSessionActorMessageHandler {
	gateSessionHandlerMu.RLock()
	defer gateSessionHandlerMu.RUnlock()
	return gateSessionHandlers[msgID]
}
