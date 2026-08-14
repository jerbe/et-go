package actorlocation

import "github.com/jerbe/et-go/engine/actor"

const (
	// MsgObjectAddRequest 表示注册位置请求。
	MsgObjectAddRequest uint16 = 20001
	// MsgObjectAddResponse 表示注册位置响应。
	MsgObjectAddResponse uint16 = 20002
	// MsgObjectGetRequest 表示查询位置请求。
	MsgObjectGetRequest uint16 = 20009
	// MsgObjectGetResponse 表示查询位置响应。
	MsgObjectGetResponse uint16 = 20010
	// MsgObjectLockRequest 表示锁定位置请求。
	MsgObjectLockRequest uint16 = 20003
	// MsgObjectLockResponse 表示锁定位置响应。
	MsgObjectLockResponse uint16 = 20004
	// MsgObjectUnlockRequest 表示解锁位置请求。
	MsgObjectUnlockRequest uint16 = 20005
	// MsgObjectUnlockResponse 表示解锁位置响应。
	MsgObjectUnlockResponse uint16 = 20006
	// MsgObjectRemoveRequest 表示删除位置请求。
	MsgObjectRemoveRequest uint16 = 20007
	// MsgObjectRemoveResponse 表示删除位置响应。
	MsgObjectRemoveResponse uint16 = 20008
)

// ObjectAddRequest 表示注册位置请求。
type ObjectAddRequest struct {
	RpcID   uint32        `json:"rpc_id"`
	Type    LocationType  `json:"type"`
	Key     int64         `json:"key"`
	ActorID actor.ActorID `json:"actor_id"`
}

// ObjectGetRequest 表示查询位置请求。
type ObjectGetRequest struct {
	RpcID uint32       `json:"rpc_id"`
	Type  LocationType `json:"type"`
	Key   int64        `json:"key"`
}

// ObjectGetResponse 表示查询位置响应。
type ObjectGetResponse struct {
	RpcID   uint32        `json:"rpc_id"`
	Error   int32         `json:"error"`
	Message string        `json:"message"`
	ActorID actor.ActorID `json:"actor_id"`
}

// ObjectLockRequest 表示加锁请求。
type ObjectLockRequest struct {
	RpcID   uint32        `json:"rpc_id"`
	Type    LocationType  `json:"type"`
	Key     int64         `json:"key"`
	ActorID actor.ActorID `json:"actor_id"`
	Time    int           `json:"time"`
}

// ObjectUnlockRequest 表示解锁请求。
type ObjectUnlockRequest struct {
	RpcID      uint32        `json:"rpc_id"`
	Type       LocationType  `json:"type"`
	Key        int64         `json:"key"`
	OldActorID actor.ActorID `json:"old_actor_id"`
	NewActorID actor.ActorID `json:"new_actor_id"`
}

// ObjectRemoveRequest 表示删除位置请求。
type ObjectRemoveRequest struct {
	RpcID uint32       `json:"rpc_id"`
	Type  LocationType `json:"type"`
	Key   int64        `json:"key"`
}

// ObjectRemoveResponse 表示通用操作响应。
type ObjectRemoveResponse struct {
	RpcID   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message"`
}
