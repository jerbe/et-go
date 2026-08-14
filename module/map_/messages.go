package map_

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/module/maprpc"
)

const (
	// MsgC2MTransferMap 表示客户端请求切图。
	MsgC2MTransferMap uint16 = 3021
	// MsgM2CTransferMap 表示客户端切图响应。
	MsgM2CTransferMap uint16 = 3022
	// MsgG2MEnterMap 表示 Gate 请求 Map 初始化玩家。
	MsgG2MEnterMap uint16 = maprpc.MsgG2MEnterMap
	// MsgM2GEnterMap 表示 Map 返回玩家初始化结果。
	MsgM2GEnterMap uint16 = maprpc.MsgM2GEnterMap
	// MsgM2MUnitTransferRequest 表示地图间转移请求。
	MsgM2MUnitTransferRequest uint16 = 23003
	// MsgM2MUnitTransferResponse 表示地图间转移响应。
	MsgM2MUnitTransferResponse uint16 = 23004
)

// C2MTransferMap 表示客户端切图请求。
type C2MTransferMap struct {
	RpcID uint32 `json:"rpc_id"`
}

// M2CTransferMap 表示客户端切图响应。
type M2CTransferMap struct {
	RpcID   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message"`
}

// G2MEnterMap 表示 Gate 请求 Map 初始化玩家。
type G2MEnterMap = maprpc.G2MEnterMap

// M2GEnterMap 表示 Map 返回初始化结果。
type M2GEnterMap = maprpc.M2GEnterMap

// M2MUnitTransferRequest 表示地图间单位转移请求。
type M2MUnitTransferRequest struct {
	RpcID      uint32        `json:"rpc_id" bson:"rpc_id"`
	OldActorID actor.ActorID `json:"old_actor_id" bson:"old_actor_id"`
	Unit       []byte        `json:"unit" bson:"unit"`
	Entitys    [][]byte      `json:"entitys" bson:"entitys"`
}

// M2MUnitTransferResponse 表示地图间单位转移响应。
type M2MUnitTransferResponse struct {
	RpcID   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message"`
}
