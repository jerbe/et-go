package maprpc

import (
	"errors"

	mapinnerpb "github.com/jerbe/et-go/proto/mapinnerpb"
	gproto "google.golang.org/protobuf/proto"
)

var ErrMessageNil = errors.New("maprpc: message is nil")

const (
	// MsgG2MEnterMap 表示 Gate 请求 Map 初始化玩家。
	MsgG2MEnterMap uint16 = 22501
	// MsgM2GEnterMap 表示 Map 返回玩家初始化结果。
	MsgM2GEnterMap uint16 = 22502
)

// G2MEnterMap 表示 Gate 请求 Map 初始化玩家。
type G2MEnterMap struct {
	RpcID    uint32 `json:"rpc_id"`
	PlayerID int64  `json:"player_id"`
}

// M2GEnterMap 表示 Map 返回初始化结果。
type M2GEnterMap struct {
	RpcID   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message"`
}

// MarshalG2MEnterMap 将 Gate 进图请求编码为 protobuf。
func MarshalG2MEnterMap(msg *G2MEnterMap) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapinnerpb.G2M_EnterMap{
		RpcId:    msg.RpcID,
		PlayerId: msg.PlayerID,
	})
}

// UnmarshalG2MEnterMap 将 protobuf 载荷解码为 Gate 进图请求。
func UnmarshalG2MEnterMap(data []byte) (*G2MEnterMap, error) {
	wire := &mapinnerpb.G2M_EnterMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2MEnterMap{
		RpcID:    wire.GetRpcId(),
		PlayerID: wire.GetPlayerId(),
	}, nil
}

// MarshalM2GEnterMap 将 Map 进图响应编码为 protobuf。
func MarshalM2GEnterMap(msg *M2GEnterMap) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapinnerpb.M2G_EnterMap{
		RpcId:   msg.RpcID,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

// UnmarshalM2GEnterMap 将 protobuf 载荷解码为 Map 进图响应。
func UnmarshalM2GEnterMap(data []byte) (*M2GEnterMap, error) {
	wire := &mapinnerpb.M2G_EnterMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2GEnterMap{
		RpcID:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}
