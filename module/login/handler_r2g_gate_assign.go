package login

import "github.com/jerbe/et-go/engine/ecs"

// HandleR2GGateAssign 处理 Gate Token 分配请求。
func HandleR2GGateAssign(scene *ecs.Scene, req *R2GGateAssign) (*G2RGateAssign, error) {
	if req == nil {
		return nil, ErrInvalidLoginRequest
	}
	if req.AccountId <= 0 {
		return nil, ErrAccountIDRequired
	}
	if scene == nil || scene.ID() <= 0 {
		return nil, ErrInvalidLoginRequest
	}
	component, ok := scene.GetComponent("GateSessionKeyComponent")
	if !ok || component == nil {
		return nil, ErrConnectGateKey
	}
	keys, ok := component.(*GateSessionKeyComponent)
	if !ok {
		return nil, ErrConnectGateKey
	}
	token, err := generateGateToken(req.AccountId)
	if err != nil {
		return nil, err
	}
	if err := keys.Add(token, req.AccountId); err != nil {
		return nil, err
	}
	return &G2RGateAssign{
		RpcId:  req.RpcId,
		GateId: scene.ID(),
		Token:  token,
	}, nil
}
