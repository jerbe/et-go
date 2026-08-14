package lockstep

import (
	"github.com/jerbe/et-go/engine/actor"
	etmath "github.com/jerbe/et-go/engine/math"
)

const (
	// 外部 Lockstep 协议。
	MsgC2GMatch                uint16 = 3501
	MsgG2CMatch                uint16 = 3502
	MsgG2CNotifyMatchSuccess   uint16 = 3503
	MsgC2RoomChangeSceneFinish uint16 = 3504
	MsgRoom2CStart             uint16 = 3506
	MsgFrameMessage            uint16 = 3507
	MsgOneFrameInputs          uint16 = 3508
	MsgRoom2CAdjustUpdateTime  uint16 = 3509
	MsgC2RoomCheckHash         uint16 = 3510
	MsgRoom2CCheckHashFail     uint16 = 3511

	// 内部 Lockstep 协议。
	MsgG2MatchMatch              uint16 = 23501
	MsgMatch2GMatch              uint16 = 23502
	MsgMatch2MapGetRoom          uint16 = 23503
	MsgMap2MatchGetRoom          uint16 = 23504
	MsgG2RoomReconnect           uint16 = 23505
	MsgRoom2GReconnect           uint16 = 23506
	MsgRoomManager2RoomInit      uint16 = 23507
	MsgRoom2RoomManagerInit      uint16 = 23508
	MsgRoom2MNotifyRoomDispose   uint16 = 23509
	MsgMatch2GNotifyMatchSuccess uint16 = 23510
	MsgG2MRoomExists             uint16 = 23511
	MsgM2GRoomExists             uint16 = 23512
	MsgM2RoomPlayerOffline       uint16 = 23513
	MsgMatch2MapCancelRoom       uint16 = 23514
	MsgMap2MatchCancelRoom       uint16 = 23515
	MsgMatch2GCancelMatchSuccess uint16 = 23516

	// MsgRoom2CReconnect 是 Go 侧将内部重连响应转成客户端消息时使用的
	// 外部语义别名；当前协议源没有为它分配独立编号，必须沿用请求上下文。
	MsgRoom2CReconnect uint16 = MsgRoom2GReconnect
)

type C2GMatch struct {
	RpcId    uint32 `json:"rpc_id"`
	PlayerId int64  `json:"player_id"`
}

type G2CMatch struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
}

type Match2GNotifyMatchSuccess struct {
	PlayerId    int64         `json:"player_id"`
	MapActorId  int64         `json:"map_actor_id"`
	RoomActorId int64         `json:"room_actor_id"`
	MapActor    actor.ActorID `json:"map_actor"`
	RoomActor   actor.ActorID `json:"room_actor"`
}

type G2MatchMatch struct {
	RpcId    uint32 `json:"rpc_id"`
	PlayerId int64  `json:"player_id"`
}

type Match2MapGetRoom struct {
	RpcId     uint32  `json:"rpc_id"`
	PlayerIds []int64 `json:"player_ids"`
}

type Map2MatchGetRoom struct {
	RpcId       uint32        `json:"rpc_id"`
	MapActorId  int64         `json:"map_actor_id"`
	RoomActorId int64         `json:"room_actor_id"`
	MapActor    actor.ActorID `json:"map_actor"`
	RoomActor   actor.ActorID `json:"room_actor"`
}

// Match2MapCancelRoom 请求 Map 回收刚创建的 Room。
type Match2MapCancelRoom struct {
	RpcId     uint32        `json:"rpc_id"`
	RoomActor actor.ActorID `json:"room_actor"`
	PlayerIds []int64       `json:"player_ids,omitempty"`
}

// Map2MatchCancelRoom 返回 Room 回收结果。
type Map2MatchCancelRoom struct {
	RpcId     uint32 `json:"rpc_id"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message,omitempty"`
}

// Match2GCancelMatchSuccess 清理 Gate 中已经写入的 Room 绑定。
type Match2GCancelMatchSuccess struct {
	PlayerId  int64         `json:"player_id"`
	MapActor  actor.ActorID `json:"map_actor"`
	RoomActor actor.ActorID `json:"room_actor"`
}

type LockStepUnitInfo struct {
	PlayerId int64             `json:"player_id"`
	Position etmath.Vector3     `json:"position"`
	Rotation etmath.Quaternion `json:"rotation"`
}

type Room2CStart struct {
	RpcId     uint32              `json:"rpc_id"`
	StartTime int64               `json:"start_time"`
	UnitInfos []*LockStepUnitInfo `json:"unit_infos,omitempty"`
}

type Room2CAdjustUpdateTime struct {
	RpcId    uint32 `json:"rpc_id"`
	DiffTime int64  `json:"diff_time"`
}

type Room2CReconnect struct {
	RpcId         uint32              `json:"rpc_id"`
	StartTime     int64               `json:"start_time"`
	UnitInfos     []*LockStepUnitInfo `json:"unit_infos,omitempty"`
	Frame         int                 `json:"frame"`
	SnapshotFrame int                 `json:"snapshot_frame"`
	Snapshot      []byte              `json:"snapshot,omitempty"`
	FrameInputs   []*OneFrameInputs   `json:"frame_inputs,omitempty"`
}

type Room2CCheckHashFail struct {
	RpcId         uint32 `json:"rpc_id"`
	Frame         int    `json:"frame"`
	SnapshotFrame int    `json:"snapshot_frame"`
	Snapshot      []byte `json:"snapshot"`
}

type FrameMessageRequest struct {
	RpcId    uint32   `json:"rpc_id"`
	PlayerId int64    `json:"player_id"`
	Frame    int      `json:"frame"`
	Input    *LSInput `json:"input"`
}

type FrameMessageResponse struct {
	RpcId    uint32 `json:"rpc_id"`
	Accepted bool   `json:"accepted"`
}

type ChangeSceneFinish struct {
	RpcId    uint32 `json:"rpc_id"`
	PlayerId int64  `json:"player_id"`
}

type CheckHashRequest struct {
	RpcId    uint32 `json:"rpc_id"`
	PlayerId int64  `json:"player_id"`
	Frame    int    `json:"frame"`
	Hash     int64  `json:"hash"`
}

type CheckHashResponse struct {
	RpcId         uint32 `json:"rpc_id"`
	Frame         int    `json:"frame"`
	SnapshotFrame int    `json:"snapshot_frame"`
	Snapshot      []byte `json:"snapshot"`
}

type ReconnectRequest struct {
	RpcId    uint32 `json:"rpc_id"`
	PlayerId int64  `json:"player_id"`
}

type Room2MNotifyRoomDispose struct {
	RoomActorId int64         `json:"room_actor_id"`
	PlayerIds   []int64       `json:"player_ids,omitempty"`
	DisposeAt   int64         `json:"dispose_at"`
	RoomActor   actor.ActorID `json:"room_actor"`
}

type M2RoomPlayerOffline struct {
	PlayerId int64 `json:"player_id"`
}
