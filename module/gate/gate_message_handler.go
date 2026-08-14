package gate

import (
	"context"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/actorlocation"
)

// SessionPacketHandler 定义 Gate 本地包处理器。
type SessionPacketHandler func(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error)

var (
	sessionHandlerMu  sync.RWMutex
	sessionHandlers   = make(map[uint16]SessionPacketHandler)
	locationMessages  = make(map[uint16]struct{})
	locationRequests  = make(map[uint16]struct{})
	locationResponses = make(map[uint16]uint16)
)

// RegisterSessionPacketHandler 注册本地 Session 包处理器。
func RegisterSessionPacketHandler(msgID uint16, handler SessionPacketHandler) {
	sessionHandlerMu.Lock()
	defer sessionHandlerMu.Unlock()
	if handler == nil {
		delete(sessionHandlers, msgID)
		return
	}
	sessionHandlers[msgID] = handler
}

// RegisterLocationMessage 注册仅转发不等待响应的定位消息。
func RegisterLocationMessage(msgID uint16) {
	sessionHandlerMu.Lock()
	locationMessages[msgID] = struct{}{}
	sessionHandlerMu.Unlock()
}

// RegisterLocationRequest 注册需要等待响应的定位请求。
func RegisterLocationRequest(msgID uint16) {
	RegisterLocationRequestWithResponse(msgID, 0)
}

// RegisterLocationRequestWithResponse 注册定位 RPC 以及客户端响应编号。
// responseMsgID 为 0 时响应沿用请求编号，适用于只有内部 RPC 编号的兼容消息。
func RegisterLocationRequestWithResponse(msgID, responseMsgID uint16) {
	sessionHandlerMu.Lock()
	defer sessionHandlerMu.Unlock()
	locationRequests[msgID] = struct{}{}
	if responseMsgID == 0 {
		delete(locationResponses, msgID)
		return
	}
	locationResponses[msgID] = responseMsgID
}

// GateMessageHandler 负责把客户端包路由到本地或定位目标。
type GateMessageHandler struct{}

// Handle 处理客户端入站消息。
func (h *GateMessageHandler) Handle(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
	if packet == nil {
		return nil, codec.ErrInvalidPacket
	}
	if scene == nil {
		return nil, ErrSceneMissing
	}

	sessionHandlerMu.RLock()
	handler := sessionHandlers[packet.MsgID]
	_, isLocationMessage := locationMessages[packet.MsgID]
	_, isLocationRequest := locationRequests[packet.MsgID]
	responseMsgID := locationResponses[packet.MsgID]
	sessionHandlerMu.RUnlock()

	if handler != nil {
		return handler(scene, session, packet)
	}

	if !isLocationMessage && !isLocationRequest {
		return nil, ErrMessageHandlerMissing
	}

	playerID, unitID, err := sessionBinding(session)
	if err != nil {
		return nil, err
	}

	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return nil, ErrLocationSenderMissing
	}
	senderComponent, ok := component.(*actorlocation.MessageLocationSenderComponent)
	if !ok || senderComponent == nil {
		return nil, ErrLocationSenderMissing
	}
	sender := senderComponent.Get(int(actorlocation.LocationTypeUnit))
	if sender == nil {
		return nil, ErrLocationSenderMissing
	}

	targetKey := unitID
	if targetKey == 0 {
		targetKey = playerID
	}

	if isLocationMessage {
		return nil, sender.Send(targetKey, packet.MsgID, packet.Payload)
	}

	callContext := context.Background()
	if session != nil {
		callContext = session.Context()
	}
	payload, err := sender.Call(callContext, targetKey, packet.MsgID, packet.Payload)
	if err != nil {
		return nil, err
	}
	if responseMsgID == 0 {
		responseMsgID = packet.MsgID
	}
	return &codec.Packet{
		Type:    codec.PacketTypeResponse,
		MsgID:   responseMsgID,
		RpcID:   packet.RpcID,
		Payload: payload,
	}, nil
}

func sessionBinding(session *network.Session) (int64, int64, error) {
	if session == nil || session.Entity() == nil {
		return 0, 0, ErrNotLoggedIn
	}
	component, ok := session.Entity().GetComponent("SessionPlayerComponent")
	if !ok || component == nil {
		return 0, 0, ErrNotLoggedIn
	}
	playerComponent, ok := component.(interface {
		GetPlayerID() int64
		GetUnitID() int64
	})
	if !ok {
		return 0, 0, ErrNotLoggedIn
	}
	return playerComponent.GetPlayerID(), playerComponent.GetUnitID(), nil
}
