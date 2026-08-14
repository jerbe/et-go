package login

import (
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/gate"
)

func init() {
	gate.RegisterSessionPacketHandler(MsgC2GPing, func(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
		req, err := unmarshalC2GPing(packet.Payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleC2GPing(session, req)
		if err != nil {
			return nil, err
		}
		payload, err := marshalG2CPing(resp)
		if err != nil {
			return nil, err
		}
		return &codec.Packet{
			Type:    codec.PacketTypeResponse,
			MsgID:   MsgG2CPing,
			RpcID:   packet.RpcID,
			Payload: payload,
		}, nil
	})
}

// HandleC2GPing 处理心跳请求。
func HandleC2GPing(session *network.Session, req *C2GPing) (*G2CPing, error) {
	if req == nil {
		return nil, ErrInvalidLoginRequest
	}
	if session != nil {
		session.TouchRecv()
	}
	return &G2CPing{
		RpcId: req.RpcId,
		Time:  time.Now().UnixMilli(),
	}, nil
}
