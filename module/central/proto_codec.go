package central

import (
	etproto "github.com/jerbe/et-go/proto"
	gproto "google.golang.org/protobuf/proto"
)

func marshalR2CentralAccountLogin(msg *R2CentralAccountLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.R2Central_AccountLogin{
		RpcId:    msg.RpcId,
		Username: msg.Username,
		Password: msg.Password,
	})
}

// MarshalR2CentralAccountLogin 导出编码函数供跨模块调用。
func MarshalR2CentralAccountLogin(msg *R2CentralAccountLogin) ([]byte, error) {
	return marshalR2CentralAccountLogin(msg)
}

func unmarshalR2CentralAccountLogin(data []byte) (*R2CentralAccountLogin, error) {
	wire := &etproto.R2Central_AccountLogin{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &R2CentralAccountLogin{
		RpcId:    wire.GetRpcId(),
		Username: wire.GetUsername(),
		Password: wire.GetPassword(),
	}, nil
}

func marshalCentral2RAccountLogin(msg *Central2RAccountLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.Central2R_AccountLogin{
		RpcId:       msg.RpcId,
		Error:       msg.Error,
		Message:     msg.Message,
		AccessToken: msg.AccessToken,
	})
}

// MarshalCentral2RAccountLogin 导出编码函数供跨模块调用。
func MarshalCentral2RAccountLogin(msg *Central2RAccountLogin) ([]byte, error) {
	return marshalCentral2RAccountLogin(msg)
}

func unmarshalCentral2RAccountLogin(data []byte) (*Central2RAccountLogin, error) {
	wire := &etproto.Central2R_AccountLogin{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Central2RAccountLogin{
		RpcId:       wire.GetRpcId(),
		Error:       wire.GetError(),
		Message:     wire.GetMessage(),
		AccessToken: wire.GetAccessToken(),
	}, nil
}

// UnmarshalCentral2RAccountLogin 导出解码函数供跨模块调用。
func UnmarshalCentral2RAccountLogin(data []byte) (*Central2RAccountLogin, error) {
	return unmarshalCentral2RAccountLogin(data)
}
