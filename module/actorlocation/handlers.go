package actorlocation

import "github.com/jerbe/et-go/engine/actor"

// HandleAdd 处理注册位置请求。
func HandleAdd(manager *LocationManagerComponent, req ObjectAddRequest) ObjectRemoveResponse {
	lot := manager.Get(int(req.Type))
	err := lot.Add(req.Key, req.ActorID)
	return commonResponse(req.RpcID, err)
}

// HandleGet 处理查询位置请求。
func HandleGet(manager *LocationManagerComponent, req ObjectGetRequest) (ObjectGetResponse, error) {
	lot := manager.Get(int(req.Type))
	actorID, err := lot.TryGet(req.Key)
	return ObjectGetResponse{
		RpcID:   req.RpcID,
		ActorID: actorID,
	}, err
}

// HandleLock 处理锁定位置请求。
func HandleLock(manager *LocationManagerComponent, req ObjectLockRequest) ObjectRemoveResponse {
	lot := manager.Get(int(req.Type))
	err := lot.Lock(req.Key, req.ActorID, req.Time)
	return commonResponse(req.RpcID, err)
}

// HandleUnlock 处理解锁位置请求。
func HandleUnlock(manager *LocationManagerComponent, req ObjectUnlockRequest) ObjectRemoveResponse {
	lot := manager.Get(int(req.Type))
	return commonResponse(req.RpcID, lot.Unlock(req.Key, req.OldActorID, req.NewActorID))
}

// HandleRemove 处理删除位置请求。
func HandleRemove(manager *LocationManagerComponent, req ObjectRemoveRequest) ObjectRemoveResponse {
	lot := manager.Get(int(req.Type))
	err := lot.Remove(req.Key)
	return commonResponse(req.RpcID, err)
}

func commonResponse(rpcID uint32, err error) ObjectRemoveResponse {
	if err == nil {
		return ObjectRemoveResponse{RpcID: rpcID}
	}
	return ObjectRemoveResponse{
		RpcID:   rpcID,
		Error:   1,
		Message: err.Error(),
	}
}

// ZeroActorID 返回零值 ActorID。
func ZeroActorID() actor.ActorID { return actor.ActorID{} }
