package login

import (
	etproto "github.com/jerbe/et-go/proto"
	gproto "google.golang.org/protobuf/proto"
)

func marshalC2RLogin(msg *C2RLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.C2R_Login{
		RpcId:       msg.RpcId,
		AccessToken: msg.AccessToken,
		ZoneId:      msg.ZoneId,
	})
}

func unmarshalC2RLogin(data []byte) (*C2RLogin, error) {
	wire := &etproto.C2R_Login{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2RLogin{
		RpcId:       wire.GetRpcId(),
		AccessToken: wire.GetAccessToken(),
		ZoneId:      wire.GetZoneId(),
	}, nil
}

func marshalR2CLogin(msg *R2CLogin) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.R2C_Login{
		RpcId:   msg.RpcId,
		Error:   msg.Error,
		Message: msg.Message,
		Address: msg.Address,
		GateId:  msg.GateId,
		Token:   msg.Token,
	})
}

func unmarshalR2CLogin(data []byte) (*R2CLogin, error) {
	wire := &etproto.R2C_Login{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &R2CLogin{
		RpcId:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
		Address: wire.GetAddress(),
		GateId:  wire.GetGateId(),
		Token:   wire.GetToken(),
	}, nil
}

func marshalR2GGateAssign(msg *R2GGateAssign) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.R2G_GateAssign{
		RpcId:     msg.RpcId,
		AccountId: msg.AccountId,
	})
}

func unmarshalR2GGateAssign(data []byte) (*R2GGateAssign, error) {
	wire := &etproto.R2G_GateAssign{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &R2GGateAssign{
		RpcId:     wire.GetRpcId(),
		AccountId: wire.GetAccountId(),
	}, nil
}

func marshalG2RGateAssign(msg *G2RGateAssign) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.G2R_GateAssign{
		RpcId:   msg.RpcId,
		Error:   msg.Error,
		Message: msg.Message,
		GateId:  msg.GateId,
		Token:   msg.Token,
	})
}

func unmarshalG2RGateAssign(data []byte) (*G2RGateAssign, error) {
	wire := &etproto.G2R_GateAssign{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2RGateAssign{
		RpcId:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
		GateId:  wire.GetGateId(),
		Token:   wire.GetToken(),
	}, nil
}

func marshalC2GLoginGate(msg *C2GLoginGate) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.C2G_LoginGate{
		RpcId:  msg.RpcId,
		Token:  msg.Token,
		GateId: msg.GateId,
	})
}

func unmarshalC2GLoginGate(data []byte) (*C2GLoginGate, error) {
	wire := &etproto.C2G_LoginGate{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2GLoginGate{
		RpcId:  wire.GetRpcId(),
		Token:  wire.GetToken(),
		GateId: wire.GetGateId(),
	}, nil
}

func marshalG2CLoginGate(msg *G2CLoginGate) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.G2C_LoginGate{
		RpcId:          msg.RpcId,
		Error:          msg.Error,
		Message:        msg.Message,
		PlayerId:       msg.PlayerId,
		CharacterCount: msg.CharacterCount,
	})
}

func unmarshalG2CLoginGate(data []byte) (*G2CLoginGate, error) {
	wire := &etproto.G2C_LoginGate{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2CLoginGate{
		RpcId:          wire.GetRpcId(),
		Error:          wire.GetError(),
		Message:        wire.GetMessage(),
		PlayerId:       wire.GetPlayerId(),
		CharacterCount: wire.GetCharacterCount(),
	}, nil
}

func marshalC2GPing(msg *C2GPing) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.C2G_Ping{RpcId: msg.RpcId})
}

func unmarshalC2GPing(data []byte) (*C2GPing, error) {
	wire := &etproto.C2G_Ping{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2GPing{RpcId: wire.GetRpcId()}, nil
}

func marshalG2CPing(msg *G2CPing) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.G2C_Ping{
		RpcId:   msg.RpcId,
		Error:   msg.Error,
		Message: msg.Message,
		Time:    msg.Time,
	})
}

func unmarshalG2CPing(data []byte) (*G2CPing, error) {
	wire := &etproto.G2C_Ping{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2CPing{
		RpcId:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
		Time:    wire.GetTime(),
	}, nil
}

func marshalG2MSessionDisconnect(msg *G2MSessionDisconnect) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&etproto.G2M_SessionDisconnect{RpcId: msg.RpcId})
}

func unmarshalG2MSessionDisconnect(data []byte) (*G2MSessionDisconnect, error) {
	wire := &etproto.G2M_SessionDisconnect{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2MSessionDisconnect{RpcId: wire.GetRpcId()}, nil
}
