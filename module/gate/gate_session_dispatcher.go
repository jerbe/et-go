package gate

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network/codec"
)

// GateSessionDispatcher 负责将 Actor 消息转发到客户端连接。
type GateSessionDispatcher struct{}

func init() {
	actor.RegisterMailBoxDispatcher(actor.MailBoxTypeGateSession, &GateSessionDispatcher{})
}

// Handle 将消息包装为网络包并转发到客户端。
func (d *GateSessionDispatcher) Handle(entity *ecs.Entity, _ actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	if entity == nil {
		return nil, ErrGateSessionMissing
	}
	component, ok := entity.GetComponent("GateSessionComponent")
	if !ok || component == nil {
		return nil, ErrGateSessionMissing
	}
	gateSession, ok := component.(*GateSessionComponent)
	if !ok || gateSession.Session == nil {
		return nil, ErrSessionNil
	}
	if handler := gateSessionActorMessageHandler(msgID); handler != nil {
		responseMsgID, responsePayload, handled, err := handler(entity, msgID, payload)
		if err != nil {
			return nil, err
		}
		if handled {
			if responseMsgID == 0 {
				return nil, nil
			}
			msgID = responseMsgID
			payload = responsePayload
		}
	}
	if err := gateSession.Session.Send(&codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   msgID,
		Payload: append([]byte(nil), payload...),
	}); err != nil {
		return nil, err
	}
	return nil, nil
}
