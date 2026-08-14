package gamelogin

import (
	"errors"

	etproto "github.com/jerbe/et-go/proto"
	gproto "google.golang.org/protobuf/proto"
)

var ErrMessageNil = errors.New("gamelogin: message is nil")

func MarshalG2GameLogin(msg *G2GameLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.G2Game_Login{
		RpcId:     msg.RpcId,
		AccountId: msg.AccountId,
	})
}

func UnmarshalG2GameLogin(data []byte) (*G2GameLogin, error) {
	wire := &etproto.G2Game_Login{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2GameLogin{
		RpcId:     wire.GetRpcId(),
		AccountId: wire.GetAccountId(),
	}, nil
}

func MarshalGame2GLogin(msg *Game2GLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.Game2G_Login{
		RpcId:     msg.RpcId,
		Error:     msg.Error,
		Message:   msg.Message,
		AccountId: msg.AccountId,
		ZoneId:    msg.ZoneId,
		PlayerId:  msg.PlayerId,
	})
}

func UnmarshalGame2GLogin(data []byte) (*Game2GLogin, error) {
	wire := &etproto.Game2G_Login{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Game2GLogin{
		RpcId:     wire.GetRpcId(),
		Error:     wire.GetError(),
		Message:   wire.GetMessage(),
		AccountId: wire.GetAccountId(),
		ZoneId:    wire.GetZoneId(),
		PlayerId:  wire.GetPlayerId(),
	}, nil
}
