package lockstep

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
)

var roomCancelRPCID atomic.Uint32

const (
	compensationMaxAttempts  = 3
	compensationRetryBackoff = 25 * time.Millisecond
	compensationCallTimeout  = 2 * time.Second
)

// RegisterMapHandlers 注册地图侧 Lockstep 处理器。
func RegisterMapHandlers(scene *ecs.Scene, mailbox *actor.MailBox) {
	if scene == nil || mailbox == nil {
		return
	}
	mailbox.RegisterHandler(MsgMatch2MapGetRoom, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalMatch2MapGetRoom(payload)
		if err != nil {
			return nil, err
		}
		resp, err := handleGetRoom(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalMap2MatchGetRoom(resp)
	})
	mailbox.RegisterHandler(MsgRoom2MNotifyRoomDispose, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalRoomDispose(payload)
		if err != nil {
			return nil, err
		}
		if !req.RoomActor.IsValid() {
			return nil, ErrRoomActorMissing
		}
		return marshalRoomDispose(HandleRoomDispose(scene, req))
	})
	mailbox.RegisterHandler(MsgMatch2MapCancelRoom, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalMatch2MapCancelRoom(payload)
		if err != nil {
			return nil, err
		}
		resp, err := handleCancelRoom(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalMap2MatchCancelRoom(resp)
	})
}

// HandleMatch 处理 Match 请求。
func HandleMatch(scene *ecs.Scene, req *G2MatchMatch) *G2CMatch {
	if scene == nil || req == nil {
		return &G2CMatch{Error: 1, Message: ErrMatchRequestInvalid.Error()}
	}
	if req.PlayerId <= 0 {
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchRequestInvalid.Error()}
	}
	component, ok := scene.GetComponent("MatchComponent")
	if !ok {
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchComponentMissing.Error()}
	}
	matchComponent, ok := component.(*MatchComponent)
	if !ok {
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: ErrMatchComponentMissing.Error()}
	}
	playerIDs := matchComponent.Match(req.PlayerId)
	if len(playerIDs) == 0 {
		return &G2CMatch{RpcId: req.RpcId}
	}
	mapScene, mapErr := ResolveDefaultMapSceneForZone(scene.Zone())
	if mapScene == nil {
		matchComponent.Requeue(playerIDs)
		if mapErr == nil {
			mapErr = ErrRoomSceneMissing
		}
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: mapErr.Error()}
	}
	room, err := handleGetRoom(mapScene, &Match2MapGetRoom{
		RpcId:     req.RpcId,
		PlayerIds: playerIDs,
	})
	if err != nil {
		matchComponent.Requeue(playerIDs)
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: err.Error()}
	}
	if err := notifyMatchSuccess(scene, playerIDs, room); err != nil {
		if cancelErr := cancelCreatedRoom(scene, playerIDs, room); cancelErr != nil {
			err = errors.Join(err, cancelErr)
		} else {
			// Map Room 已回收且 Gate 侧的幂等清理消息全部成功，重新入队
			// 不会与未知的 PlayerRoom 绑定并发创建第二个房间。
			matchComponent.Requeue(playerIDs)
		}
		return &G2CMatch{RpcId: req.RpcId, Error: 1, Message: err.Error()}
	}
	matchComponent.setLastRoom(room)
	return &G2CMatch{RpcId: req.RpcId}
}

func cancelCreatedRoom(scene *ecs.Scene, playerIDs []int64, room *Map2MatchGetRoom) error {
	if scene == nil || room == nil || !room.MapActor.IsValid() || !room.RoomActor.IsValid() {
		return ErrRoomCancelMissing
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return ErrRoomCancelMissing
	}
	sender, ok := component.(interface {
		Call(context.Context, actor.ActorID, uint16, []byte) ([]byte, error)
	})
	if !ok {
		return ErrRoomCancelMissing
	}
	rpcID := roomCancelRPCID.Add(1)
	if rpcID == 0 {
		rpcID = roomCancelRPCID.Add(1)
	}
	payload, err := marshalMatch2MapCancelRoom(&Match2MapCancelRoom{
		RpcId:     rpcID,
		RoomActor: room.RoomActor,
		PlayerIds: append([]int64(nil), playerIDs...),
	})
	if err != nil {
		return err
	}
	var responsePayload []byte
	for attempt := 0; attempt < compensationMaxAttempts; attempt++ {
		callCtx, cancel := context.WithTimeout(context.Background(), compensationCallTimeout)
		responsePayload, err = sender.Call(callCtx, room.MapActor, MsgMatch2MapCancelRoom, payload)
		cancel()
		if err == nil {
			break
		}
		if attempt+1 < compensationMaxAttempts {
			time.Sleep(compensationRetryBackoff)
		}
	}
	if err != nil {
		return err
	}
	response, err := unmarshalMap2MatchCancelRoom(responsePayload)
	if err != nil {
		return err
	}
	if response.RpcId != rpcID || !response.Cancelled {
		if response.Message != "" {
			return fmt.Errorf("%w: %s", ErrRoomCancelMissing, response.Message)
		}
		return ErrRoomCancelMissing
	}
	var cleanupErr error
	for _, playerID := range playerIDs {
		if err := sendRoomMessageWithRetry(scene, playerID, &Match2GCancelMatchSuccess{
			PlayerId:  playerID,
			MapActor:  room.MapActor,
			RoomActor: room.RoomActor,
		}); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	return cleanupErr
}

func sendRoomMessageWithRetry(scene *ecs.Scene, playerID int64, message any) error {
	var err error
	for attempt := 0; attempt < compensationMaxAttempts; attempt++ {
		err = sendRoomMessage(scene, playerID, message)
		if err == nil {
			return nil
		}
		if attempt+1 < compensationMaxAttempts {
			time.Sleep(compensationRetryBackoff)
		}
	}
	return err
}

// HandleGetRoom 处理 Map 获取房间请求。
func HandleGetRoom(scene *ecs.Scene, req *Match2MapGetRoom) *Map2MatchGetRoom {
	response, err := handleGetRoom(scene, req)
	if err != nil {
		slog.Error("lockstep legacy HandleGetRoom failed", "err", err)
	}
	return response
}

func handleGetRoom(scene *ecs.Scene, req *Match2MapGetRoom) (*Map2MatchGetRoom, error) {
	if scene == nil || req == nil {
		return &Map2MatchGetRoom{}, ErrRoomSceneMissing
	}
	component, ok := scene.GetComponent("RoomManagerComponent")
	if !ok {
		return &Map2MatchGetRoom{RpcId: req.RpcId}, ErrRoomManagerMissing
	}
	manager, ok := component.(*RoomManagerComponent)
	if !ok {
		return &Map2MatchGetRoom{RpcId: req.RpcId}, ErrRoomManagerMissing
	}
	mapActorID, roomActorID, err := manager.CreateRoom(req.PlayerIds)
	if err != nil {
		return &Map2MatchGetRoom{RpcId: req.RpcId}, err
	}
	room, ok := manager.FindRoomByActorID(roomActorID)
	if !ok || room == nil || !room.RoomActor.IsValid() || !room.MapActor.IsValid() {
		// CreateRoom 已经可能创建了动态 Fiber；发现返回元数据不完整时
		// 必须立即回收，不能把玩家重新入队后留下孤儿 Room。
		manager.RemoveRoomByActorID(roomActorID)
		return &Map2MatchGetRoom{RpcId: req.RpcId}, ErrRoomActorMissing
	}
	return &Map2MatchGetRoom{
		RpcId:       req.RpcId,
		MapActorId:  mapActorID,
		RoomActorId: roomActorID,
		MapActor:    room.MapActor,
		RoomActor:   room.RoomActor,
	}, nil
}

// HandleFrameMessage 写入待处理输入。
func HandleFrameMessage(scene *ecs.Scene, req *FrameMessageRequest) *FrameMessageResponse {
	if req == nil {
		return &FrameMessageResponse{}
	}
	resp := &FrameMessageResponse{RpcId: req.RpcId}
	if scene == nil {
		return resp
	}
	if req.PlayerId <= 0 || req.Frame <= 0 {
		return resp
	}
	updaterComponent, ok := scene.GetComponent("LSServerUpdater")
	if !ok {
		return resp
	}
	updater, ok := updaterComponent.(*LSServerUpdater)
	if !ok || updater.room == nil || updater.frameBuffer == nil {
		return resp
	}
	component, ok := scene.GetComponent("RoomServerComponent")
	if !ok {
		return resp
	}
	roomServer, ok := component.(*RoomServerComponent)
	if !ok {
		return resp
	}
	if !roomServer.SetPlayerOnline(req.PlayerId, true) {
		return resp
	}
	if req.Frame <= 0 {
		return resp
	}
	maxAhead := updater.room.AuthorityFrame + MaxPredictionFrameWindow + AdjustTimeThreshold
	if req.Frame > maxAhead {
		return resp
	}
	if req.Frame < updater.room.AuthorityFrame-MaxPredictionFrameWindow {
		return resp
	}
	if err := updater.AddInput(req.Frame, req.PlayerId, req.Input); err != nil {
		return resp
	}
	if req.Frame > updater.room.PredictionFrame {
		updater.room.PredictionFrame = req.Frame
	}
	diff := req.Frame - updater.room.AuthorityFrame
	if absInt(diff) > AdjustTimeThreshold {
		if err := sendRoomMessage(scene, req.PlayerId, &Room2CAdjustUpdateTime{
			RpcId:    req.RpcId,
			DiffTime: int64(diff) * UpdateIntervalMillis,
		}); err != nil {
			return resp
		}
	}
	resp.Accepted = true
	return resp
}

// HandleChangeSceneFinish 标记玩家准备完成。
func HandleChangeSceneFinish(scene *ecs.Scene, req *ChangeSceneFinish) *Room2CStart {
	if scene == nil || req == nil {
		return &Room2CStart{}
	}
	component, ok := scene.GetComponent("RoomServerComponent")
	if !ok {
		return &Room2CStart{RpcId: req.RpcId}
	}
	roomServer, ok := component.(*RoomServerComponent)
	if !ok {
		return &Room2CStart{RpcId: req.RpcId}
	}
	if !roomServer.SetPlayerProgress(req.PlayerId, 100) {
		return &Room2CStart{RpcId: req.RpcId}
	}
	roomServer.SetPlayerOnline(req.PlayerId, true)
	if roomServer.IsAllPlayerProgress100() {
		if start := roomServer.StartGame(req.RpcId); start != nil {
			return start
		}
	}
	return &Room2CStart{RpcId: req.RpcId}
}

// HandleCheckHash 校验帧哈希。
func HandleCheckHash(scene *ecs.Scene, req *CheckHashRequest) *CheckHashResponse {
	response, err := handleCheckHash(scene, req)
	if err != nil {
		slog.Error("lockstep legacy HandleCheckHash failed", "err", err)
	}
	return response
}

func handleCheckHash(scene *ecs.Scene, req *CheckHashRequest) (*CheckHashResponse, error) {
	if scene == nil || req == nil {
		return &CheckHashResponse{}, ErrSnapshotMissing
	}
	if req.PlayerId <= 0 || req.Frame <= 0 {
		return &CheckHashResponse{RpcId: req.RpcId}, ErrFrameInvalid
	}
	component, ok := scene.GetComponent("LSServerUpdater")
	if !ok {
		return &CheckHashResponse{RpcId: req.RpcId}, ErrSnapshotMissing
	}
	updater, ok := component.(*LSServerUpdater)
	if !ok {
		return &CheckHashResponse{RpcId: req.RpcId}, ErrSnapshotMissing
	}
	if updater.frameBuffer == nil {
		return &CheckHashResponse{RpcId: req.RpcId, Frame: req.Frame}, ErrFrameBufferMissing
	}
	_, _, mismatch := updater.frameBuffer.CheckAndSetHash(req.Frame, req.Hash)
	if mismatch {
		snapshotFrame, snapshot, ok := updater.frameBuffer.GetNearestSnapshot(req.Frame)
		if !ok {
			return &CheckHashResponse{
				RpcId: req.RpcId,
				Frame: req.Frame,
			}, ErrSnapshotMissing
		}
		fail := &Room2CCheckHashFail{
			RpcId:         req.RpcId,
			Frame:         req.Frame,
			SnapshotFrame: snapshotFrame,
			Snapshot:      snapshot,
		}
		if err := broadcastToGate(scene, []int64{req.PlayerId}, MsgRoom2CCheckHashFail, fail); err != nil {
			return &CheckHashResponse{
				RpcId: req.RpcId,
				Frame: req.Frame,
			}, err
		}
		return &CheckHashResponse{
			RpcId:         req.RpcId,
			Frame:         req.Frame,
			SnapshotFrame: snapshotFrame,
			Snapshot:      snapshot,
		}, nil
	}
	return &CheckHashResponse{RpcId: req.RpcId}, nil
}

// HandleReconnect 返回最近的启动信息。
func HandleReconnect(scene *ecs.Scene, req *ReconnectRequest) *Room2CReconnect {
	response, err := handleReconnect(scene, req)
	if err != nil {
		slog.Error("lockstep legacy HandleReconnect failed", "err", err)
	}
	return response
}

func handleReconnect(scene *ecs.Scene, req *ReconnectRequest) (*Room2CReconnect, error) {
	if scene == nil || req == nil {
		return &Room2CReconnect{}, ErrSnapshotMissing
	}
	if req.PlayerId <= 0 {
		return &Room2CReconnect{RpcId: req.RpcId}, ErrPlayerInvalid
	}
	component, ok := scene.GetComponent("RoomServerComponent")
	if !ok {
		return &Room2CReconnect{RpcId: req.RpcId}, ErrSnapshotMissing
	}
	roomServer, ok := component.(*RoomServerComponent)
	if !ok || roomServer.room == nil {
		return &Room2CReconnect{RpcId: req.RpcId}, ErrSnapshotMissing
	}
	if roomServer.room.FrameBuffer == nil {
		return &Room2CReconnect{RpcId: req.RpcId}, ErrFrameBufferMissing
	}
	if roomServer.room.Replay == nil {
		return &Room2CReconnect{RpcId: req.RpcId}, ErrReplayMissing
	}
	unitInfos, err := buildUnitInfosFromWorld(roomServer.room.World, roomServer.PlayerIDs())
	if err != nil {
		return &Room2CReconnect{RpcId: req.RpcId}, err
	}
	roomServer.SetPlayerOnline(req.PlayerId, true)
	snapshotFrame, snapshot, ok := roomServer.room.FrameBuffer.GetNearestSnapshot(roomServer.room.AuthorityFrame)
	if !ok {
		return &Room2CReconnect{
			RpcId: req.RpcId,
			Frame: roomServer.room.AuthorityFrame,
		}, ErrSnapshotMissing
	}
	return &Room2CReconnect{
		RpcId:         req.RpcId,
		StartTime:     roomServer.room.StartTime,
		UnitInfos:     unitInfos,
		Frame:         roomServer.room.AuthorityFrame,
		SnapshotFrame: snapshotFrame,
		Snapshot:      snapshot,
		FrameInputs:   roomServer.room.Replay.GetFrameInputsRange(snapshotFrame, roomServer.room.AuthorityFrame),
	}, nil
}

// HandlePlayerOffline 标记房间内玩家离线。
func HandlePlayerOffline(scene *ecs.Scene, req *M2RoomPlayerOffline) *M2RoomPlayerOffline {
	if scene == nil || req == nil {
		return &M2RoomPlayerOffline{}
	}
	component, ok := scene.GetComponent("RoomServerComponent")
	if !ok {
		return req
	}
	roomServer, ok := component.(*RoomServerComponent)
	if !ok {
		return req
	}
	roomServer.SetPlayerOnline(req.PlayerId, false)
	return req
}

// HandleRoomDispose 从地图移除房间记录。
func HandleRoomDispose(scene *ecs.Scene, req *Room2MNotifyRoomDispose) *Room2MNotifyRoomDispose {
	if scene == nil || req == nil {
		return &Room2MNotifyRoomDispose{}
	}
	component, ok := scene.GetComponent("RoomManagerComponent")
	if !ok {
		return req
	}
	manager, ok := component.(*RoomManagerComponent)
	if !ok {
		return req
	}
	if !req.RoomActor.IsValid() {
		return req
	}
	manager.RemoveRoomByActor(req.RoomActor)
	return req
}

// HandleCancelRoom 回收 Match 已创建但尚未成功通知客户端的 Room。
func HandleCancelRoom(scene *ecs.Scene, req *Match2MapCancelRoom) *Map2MatchCancelRoom {
	response, err := handleCancelRoom(scene, req)
	if err != nil {
		slog.Error("lockstep legacy HandleCancelRoom failed", "err", err)
	}
	return response
}

func handleCancelRoom(scene *ecs.Scene, req *Match2MapCancelRoom) (*Map2MatchCancelRoom, error) {
	if scene == nil || req == nil || req.RpcId == 0 || !req.RoomActor.IsValid() {
		rpcID := uint32(0)
		if req != nil {
			rpcID = req.RpcId
		}
		return &Map2MatchCancelRoom{RpcId: rpcID}, ErrRoomCancelMissing
	}
	component, ok := scene.GetComponent("RoomManagerComponent")
	if !ok || component == nil {
		return &Map2MatchCancelRoom{RpcId: req.RpcId}, ErrRoomCancelMissing
	}
	manager, ok := component.(*RoomManagerComponent)
	if !ok || manager == nil {
		return &Map2MatchCancelRoom{RpcId: req.RpcId}, ErrRoomCancelMissing
	}
	// Desired state is “Room absent”; a repeated cancellation is therefore
	// successful even when the first request already removed it.
	manager.RemoveRoomByActorIfExists(req.RoomActor)
	return &Map2MatchCancelRoom{
		RpcId:     req.RpcId,
		Cancelled: true,
	}, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func notifyMatchSuccess(scene *ecs.Scene, playerIDs []int64, room *Map2MatchGetRoom) error {
	if scene == nil || room == nil {
		return ErrMatchNotificationMissing
	}
	if len(playerIDs) == 0 {
		return ErrRoomPlayersInvalid
	}
	if !room.MapActor.IsValid() || !room.RoomActor.IsValid() {
		return ErrRoomActorMissing
	}
	var sendErr error
	for _, playerID := range playerIDs {
		err := sendRoomMessage(scene, playerID, &Match2GNotifyMatchSuccess{
			PlayerId:    playerID,
			MapActorId:  room.MapActorId,
			RoomActorId: room.RoomActorId,
			MapActor:    room.MapActor,
			RoomActor:   room.RoomActor,
		})
		if err != nil {
			slog.Error("send match success notification failed", "player_id", playerID, "err", err)
			sendErr = errors.Join(sendErr, fmt.Errorf("%w: %v", ErrMatchNotificationMissing, err))
		}
	}
	return sendErr
}
