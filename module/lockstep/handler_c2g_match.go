package lockstep

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/gate"
)

func init() {
	gate.RegisterSessionPacketHandler(MsgC2GMatch, func(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
		req, err := unmarshalC2GMatch(packet.Payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleC2GMatch(scene, session, req)
		if err != nil {
			return nil, err
		}
		payload, err := marshalG2CMatch(resp)
		if err != nil {
			return nil, err
		}
		return &codec.Packet{
			Type:    codec.PacketTypeResponse,
			MsgID:   MsgG2CMatch,
			RpcID:   packet.RpcID,
			Payload: payload,
		}, nil
	})
}

// HandleC2GMatch 处理客户端发起的匹配请求。
func HandleC2GMatch(scene *ecs.Scene, session *network.Session, req *C2GMatch) (*G2CMatch, error) {
	if req == nil {
		return nil, ErrMatchRequestInvalid
	}
	playerID := req.PlayerId
	if session != nil && session.Entity() != nil {
		if component, ok := session.Entity().GetComponent("SessionPlayerComponent"); ok && component != nil {
			if sessionPlayer, ok := component.(interface{ GetUnitID() int64 }); ok {
				boundPlayerID := sessionPlayer.GetUnitID()
				if boundPlayerID <= 0 {
					return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchRequestInvalid.Error()}, nil
				}
				if playerID != 0 && playerID != boundPlayerID {
					return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchRequestInvalid.Error()}, nil
				}
				playerID = boundPlayerID
			}
		}
	}
	if playerID <= 0 {
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchRequestInvalid.Error()}, nil
	}
	var matchScene *ecs.Scene
	var matchErr error
	if scene != nil && scene.Zone() > 0 {
		matchScene, matchErr = ResolveMatchSceneForZone(scene.Zone())
	} else {
		// 独立单元测试或未挂载 Gate Scene 时没有 Zone 上下文，只允许
		// 全局注册表本身唯一；多个候选仍然返回歧义错误。
		matchScene, matchErr = ResolveMatchScene()
	}
	if matchScene == nil {
		if matchErr == nil {
			matchErr = ErrMatchSceneAmbiguous
		}
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: matchErr.Error()}, nil
	}
	return HandleMatch(matchScene, &G2MatchMatch{
		RpcId:    req.RpcId,
		PlayerId: playerID,
	}), nil
}
