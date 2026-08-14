package map_

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/statesync"
	"github.com/jerbe/et-go/module/unit"
	"github.com/jerbe/et-go/proto"
)

const (
	transferLockTimeout = 5000
	transferCallTimeout = 10 * time.Second
)

var transferRPCID atomic.Uint32

type locationProxy interface {
	Lock(locationType int, key int64, actorID actor.ActorID, timeMs int) error
	Unlock(locationType int, key int64, oldActorID, newActorID actor.ActorID) error
}

type messageSender interface {
	Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
}

type sceneNotifier interface {
	NotifyTransfer(unitID int64, sceneName string, unitInfo *proto.UnitInfo) error
}

// HandleTransferMap 处理已路由到目标 Unit MailBox 的客户端切图请求。
func HandleTransferMap(scene *ecs.Scene, targetActorID actor.ActorID, req C2MTransferMap) M2CTransferMap {
	u := unitForActor(scene, targetActorID)
	if u == nil {
		return M2CTransferMap{RpcID: req.RpcID, Error: 1, Message: ErrTransferUnitMissing.Error()}
	}

	targetName, targetActorID, err := resolveTarget(scene)
	if err != nil {
		return M2CTransferMap{RpcID: req.RpcID, Error: 1, Message: err.Error()}
	}

	scheduler, ok := scene.Fiber().(interface{ AddFrameFinishTask(func()) error })
	if !ok {
		return M2CTransferMap{
			RpcID:   req.RpcID,
			Error:   1,
			Message: "map_: frame-finish scheduler missing",
		}
	}
	if err := scheduler.AddFrameFinishTask(func() {
		if err := transferUnit(scene, u, targetActorID, targetName); err != nil {
			slog.Error("Map transfer failed", "unit_id", u.ID(), "rpc_id", req.RpcID, "err", err)
			if reportErr := reportTransferFailure(scene, u.ID(), req.RpcID, err); reportErr != nil {
				slog.Error("Map transfer failed", "unit_id", u.ID(), "rpc_id", req.RpcID, "err", errors.Join(err, reportErr))
			}
		}
	}); err != nil {
		return M2CTransferMap{RpcID: req.RpcID, Error: 1, Message: err.Error()}
	}
	return M2CTransferMap{RpcID: req.RpcID}
}

// TransferAtFrameFinish 在当前帧结束后执行转移。
func TransferAtFrameFinish(scene *ecs.Scene, u *unit.Unit, targetActorID actor.ActorID, targetMapName string) error {
	if scene == nil || scene.IsDisposed() || u == nil {
		return ErrTransferUnitMissing
	}

	if waiter, ok := scene.Fiber().(interface{ WaitFrameFinish() <-chan struct{} }); ok {
		<-waiter.WaitFrameFinish()
	}
	if scene.IsDisposed() {
		return ErrTransferUnitMissing
	}
	return transferUnit(scene, u, targetActorID, targetMapName)
}

func transferUnit(scene *ecs.Scene, u *unit.Unit, targetActorID actor.ActorID, targetMapName string) error {
	locationService, err := getLocationProxy(scene)
	if err != nil {
		return err
	}
	if !targetActorID.IsValid() || targetMapName == "" {
		return ErrMapTargetNotFound
	}
	oldActorID := actorIDForEntity(scene, &u.Entity)
	if !oldActorID.IsValid() {
		return ErrTransferRequestInvalid
	}
	locked := false
	defer func() {
		if locked {
			if err := locationService.Unlock(int(actorlocation.LocationTypeUnit), u.ID(), oldActorID, oldActorID); err != nil {
				slog.Error("Map transfer rollback unlock failed", "unit_id", u.ID(), "err", err)
			}
		}
	}()

	if err := locationService.Lock(int(actorlocation.LocationTypeUnit), u.ID(), oldActorID, transferLockTimeout); err != nil {
		return err
	}
	locked = true

	unitBytes, err := SerializeUnit(u)
	if err != nil {
		return err
	}
	componentBytes, err := SerializeTransferComponents(u)
	if err != nil {
		return err
	}

	rpcSender, err := getMessageSender(scene)
	if err != nil {
		return err
	}
	request := &M2MUnitTransferRequest{
		RpcID:      nextTransferRPCID(),
		OldActorID: oldActorID,
		Unit:       unitBytes,
		Entitys:    componentBytes,
	}
	payload, err := marshalUnitTransferRequest(request)
	if err != nil {
		return err
	}
	journal, err := transferJournalForScene(scene)
	if err != nil {
		return err
	}
	var transaction *TransferTransactionRecord
	if journal != nil {
		transaction, err = journal.Begin(context.Background(), scene, request, targetActorID, targetMapName)
		if err != nil {
			return err
		}
	}
	callContext, cancel := context.WithTimeout(context.Background(), transferCallTimeout)
	responsePayload, err := rpcSender.Call(callContext, targetActorID, MsgM2MUnitTransferRequest, payload)
	cancel()
	if err != nil {
		if transaction != nil {
			if journalErr := journal.MarkState(context.Background(), scene, transaction, TransferTransactionPending, err); journalErr != nil {
				err = errors.Join(err, journalErr)
			}
		}
		return err
	}
	if len(responsePayload) == 0 {
		err = fmt.Errorf("map_: empty transfer response")
		if transaction != nil {
			if journalErr := journal.MarkState(context.Background(), scene, transaction, TransferTransactionPending, err); journalErr != nil {
				err = errors.Join(err, journalErr)
			}
		}
		return err
	}
	response, err := unmarshalUnitTransferResponse(responsePayload)
	if err != nil {
		if transaction != nil {
			if journalErr := journal.MarkState(context.Background(), scene, transaction, TransferTransactionPending, err); journalErr != nil {
				err = errors.Join(err, journalErr)
			}
		}
		return err
	}
	if response.Error != 0 {
		if response.Message == "" {
			err = fmt.Errorf("map_: target transfer rejected")
		} else {
			err = errors.New(response.Message)
		}
		if transaction != nil {
			if journalErr := journal.MarkState(context.Background(), scene, transaction, TransferTransactionFailed, err); journalErr != nil {
				err = errors.Join(err, journalErr)
			}
		}
		return err
	}
	if transaction != nil {
		if err := journal.MarkState(context.Background(), scene, transaction, TransferTransactionCommitted, nil); err != nil {
			return err
		}
	}

	// 目标地图在成功准备并完成 Location ownership 切换后才返回成功。
	// 此时源地图才提交本地销毁，避免 RPC 失败导致单位丢失。
	detachUnitFromScene(scene, u)
	u.Dispose()
	locked = false
	if transaction != nil {
		if err := journal.MarkState(context.Background(), scene, transaction, TransferTransactionSourceDisposed, nil); err != nil {
			return err
		}
	}
	return nil
}

// HandleUnitTransfer 处理来自其他地图的单位转移。
func HandleUnitTransfer(scene *ecs.Scene, req *M2MUnitTransferRequest) M2MUnitTransferResponse {
	if scene == nil || req == nil {
		return transferErrorResponse(req, ErrTransferUnitMissing)
	}
	if req.RpcID == 0 || !req.OldActorID.IsValid() {
		return transferErrorResponse(req, ErrTransferRequestInvalid)
	}
	ledger, err := transferLedgerForScene(scene)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	handle, err := ledger.begin(scene, req)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	if !handle.owner {
		return ledger.response(handle)
	}
	response := handleUnitTransferOnce(scene, req)
	if err := ledger.complete(context.Background(), scene, handle, response); err != nil {
		return transferErrorResponse(req, err)
	}
	return response
}

func handleUnitTransferOnce(scene *ecs.Scene, req *M2MUnitTransferRequest) M2MUnitTransferResponse {
	if scene == nil || req == nil {
		return transferErrorResponse(req, ErrTransferUnitMissing)
	}

	unitComponent, err := requiredUnitComponent(scene)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	locationService, err := getLocationProxy(scene)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	aoiManager, err := requiredAOIManager(scene)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	mapManager, err := requiredMapManager(scene)
	if err != nil {
		return transferErrorResponse(req, err)
	}

	u, err := DeserializeUnit(req.Unit)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	if existing, ok := unitComponent.Get(u.ID()); ok && existing != nil && !existing.IsDisposed() {
		return transferErrorResponse(req, ErrTransferUnitAlreadyExists)
	}

	components, err := DeserializeComponents(req.Entitys)
	if err != nil {
		return transferErrorResponse(req, err)
	}
	for _, component := range components {
		u.AddComponent(component)
	}

	moveComponent := move.NewMoveComponent()
	u.AddComponent(moveComponent)
	moveComponent.Bind(u)

	pathfinding := move.NewPathfindingComponentForScene(scene, mapManager.MapName)
	u.AddComponent(pathfinding)

	newActorID := actorIDForEntity(scene, &u.Entity)
	if !newActorID.IsValid() {
		return transferErrorResponse(req, ErrTransferUnitMissing)
	}
	u.AddComponent(actor.NewMailBox(newActorID, actor.MailBoxTypeOrderedMessage))

	aoiEntity := aoi.NewAOIEntity(u.ID(), int(u.UnitType), 9000)
	aoiEntity.Pos = u.Position()
	u.AddComponent(aoiEntity)
	scene.AddChildWithID(u.ID(), &u.Entity)
	if err := unitComponent.Add(u); err != nil {
		u.Dispose()
		return transferErrorResponse(req, err)
	}
	aoiManager.Enter(aoiEntity, u.Position().X, u.Position().Z)

	if err := sendTransferNotifications(scene, u); err != nil {
		removeTransferredUnit(scene, u, aoiManager, unitComponent)
		return transferErrorResponse(req, err)
	}

	if err := locationService.Unlock(int(actorlocation.LocationTypeUnit), u.ID(), req.OldActorID, newActorID); err != nil {
		removeTransferredUnit(scene, u, aoiManager, unitComponent)
		return transferErrorResponse(req, err)
	}

	return M2MUnitTransferResponse{RpcID: req.RpcID}
}

func transferLedgerForScene(scene *ecs.Scene) (*TransferLedgerComponent, error) {
	if scene == nil {
		return nil, ErrTransferLedgerMissing
	}
	component, ok := scene.GetComponent("TransferLedgerComponent")
	if !ok || component == nil {
		return nil, ErrTransferLedgerMissing
	}
	ledger, ok := component.(*TransferLedgerComponent)
	if !ok || ledger == nil {
		return nil, ErrTransferLedgerMissing
	}
	return ledger, nil
}

func nextTransferRPCID() uint32 {
	for {
		id := transferRPCID.Add(1)
		if id != 0 {
			return id
		}
	}
}

func sendTransferNotifications(scene *ecs.Scene, u *unit.Unit) error {
	if scene == nil || u == nil {
		return ErrTransferNotificationMissing
	}
	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return ErrTransferNotificationMissing
	}

	if notifier, ok := component.(sceneNotifier); ok {
		return notifier.NotifyTransfer(u.ID(), currentMapName(scene), statesync.CreateUnitInfo(u))
	}

	locationSenders, ok := component.(*actorlocation.MessageLocationSenderComponent)
	if !ok {
		return ErrTransferNotificationMissing
	}
	sender := locationSenders.Get(int(actorlocation.LocationTypeGateSession))
	if sender == nil {
		return ErrTransferNotificationMissing
	}

	payload, err := statesync.MarshalStartSceneChange(statesync.NewStartSceneChange(scene.InstanceID(), currentMapName(scene)))
	if err != nil {
		return err
	}
	if err := sender.Send(u.ID(), statesync.MsgStartSceneChange, payload); err != nil {
		return err
	}
	payload, err = statesync.MarshalCreateMyUnit(statesync.NewCreateMyUnit(u))
	if err != nil {
		return err
	}
	return sender.Send(u.ID(), statesync.MsgCreateMyUnit, payload)
}

func reportTransferFailure(scene *ecs.Scene, unitID int64, rpcID uint32, transferErr error) error {
	if scene == nil || unitID <= 0 || transferErr == nil {
		return ErrTransferNotificationMissing
	}
	component, ok := scene.GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return ErrTransferNotificationMissing
	}
	locationSenders, ok := component.(*actorlocation.MessageLocationSenderComponent)
	if !ok {
		return ErrTransferNotificationMissing
	}
	sender := locationSenders.Get(int(actorlocation.LocationTypeGateSession))
	if sender == nil {
		return ErrTransferNotificationMissing
	}
	payload, err := marshalTransferMapResponse(&M2CTransferMap{
		RpcID:   rpcID,
		Error:   1,
		Message: transferErr.Error(),
	})
	if err != nil {
		return err
	}
	return sender.Send(unitID, MsgM2CTransferMap, payload)
}

func currentMapName(scene *ecs.Scene) string {
	if scene == nil {
		return ""
	}
	if component, ok := scene.GetComponent("MapUnitManagerComponent"); ok {
		if manager, ok := component.(*MapUnitManagerComponent); ok {
			return manager.MapName
		}
	}
	return ""
}

func resolveTarget(scene *ecs.Scene) (string, actor.ActorID, error) {
	if scene == nil {
		return "", actor.ActorID{}, ErrMapTargetNotFound
	}
	component, ok := scene.GetComponent("MapUnitManagerComponent")
	if !ok || component == nil {
		return "", actor.ActorID{}, ErrMapTargetNotFound
	}
	manager, ok := component.(*MapUnitManagerComponent)
	if !ok || manager == nil {
		return "", actor.ActorID{}, ErrMapTargetNotFound
	}
	manager.Awake()
	if manager.MapName == "" {
		return "", actor.ActorID{}, ErrMapManagerMissing
	}

	if len(manager.Targets) != 1 {
		return "", actor.ActorID{}, ErrMapTargetAmbiguous
	}
	for targetName, targetActorID := range manager.Targets {
		if targetName == "" || !targetActorID.IsValid() {
			return "", actor.ActorID{}, ErrMapTargetNotFound
		}
		return targetName, targetActorID, nil
	}
	return "", actor.ActorID{}, ErrMapTargetNotFound
}

func requiredUnitComponent(scene *ecs.Scene) (*unit.UnitComponent, error) {
	if scene == nil {
		return nil, ErrTransferUnitMissing
	}
	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil, ErrTransferUnitMissing
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok || unitComponent == nil {
		return nil, ErrTransferUnitMissing
	}
	return unitComponent, nil
}

func requiredMapManager(scene *ecs.Scene) (*MapUnitManagerComponent, error) {
	if scene == nil {
		return nil, ErrMapManagerMissing
	}
	component, ok := scene.GetComponent("MapUnitManagerComponent")
	if !ok || component == nil {
		return nil, ErrMapManagerMissing
	}
	manager, ok := component.(*MapUnitManagerComponent)
	if !ok || manager == nil || manager.MapName == "" {
		return nil, ErrMapManagerMissing
	}
	return manager, nil
}

func requiredAOIManager(scene *ecs.Scene) (*aoi.AOIManagerComponent, error) {
	if scene == nil {
		return nil, ErrAOIManagerMissing
	}
	component, ok := scene.GetComponent("AOIManagerComponent")
	if !ok || component == nil {
		return nil, ErrAOIManagerMissing
	}
	manager, ok := component.(*aoi.AOIManagerComponent)
	if !ok || manager == nil {
		return nil, ErrAOIManagerMissing
	}
	return manager, nil
}

func unitForActor(scene *ecs.Scene, targetActorID actor.ActorID) *unit.Unit {
	if scene == nil || !targetActorID.IsValid() {
		return nil
	}
	component, ok := scene.GetComponent("UnitComponent")
	if !ok || component == nil {
		return nil
	}
	unitComponent, ok := component.(*unit.UnitComponent)
	if !ok {
		return nil
	}
	for _, u := range unitComponent.GetAll() {
		if u == nil || u.IsDisposed() {
			continue
		}
		if actorIDForEntity(scene, &u.Entity) == targetActorID {
			return u
		}
	}
	return nil
}

func transferErrorResponse(req *M2MUnitTransferRequest, err error) M2MUnitTransferResponse {
	response := M2MUnitTransferResponse{Error: 1}
	if req != nil {
		response.RpcID = req.RpcID
	}
	if err == nil {
		err = ErrTransferRequestInvalid
	}
	response.Message = err.Error()
	return response
}

func detachUnitFromScene(scene *ecs.Scene, u *unit.Unit) {
	if scene == nil || u == nil {
		return
	}
	if component, ok := scene.GetComponent("AOIManagerComponent"); ok && component != nil {
		if manager, ok := component.(*aoi.AOIManagerComponent); ok {
			if aoiComponent, ok := u.GetComponent("AOIEntity"); ok && aoiComponent != nil {
				if aoiEntity, ok := aoiComponent.(*aoi.AOIEntity); ok {
					manager.Leave(aoiEntity)
				}
			}
		}
	}
	if component, ok := scene.GetComponent("UnitComponent"); ok && component != nil {
		if unitComponent, ok := component.(*unit.UnitComponent); ok {
			unitComponent.Remove(u.ID())
		}
	}
}

func removeTransferredUnit(scene *ecs.Scene, u *unit.Unit, aoiManager *aoi.AOIManagerComponent, unitComponent *unit.UnitComponent) {
	if u == nil {
		return
	}
	if aoiManager != nil {
		if component, ok := u.GetComponent("AOIEntity"); ok {
			if aoiEntity, ok := component.(*aoi.AOIEntity); ok {
				aoiManager.Leave(aoiEntity)
			}
		}
	}
	if unitComponent != nil {
		unitComponent.Remove(u.ID())
	}
	u.Dispose()
}

func getLocationProxy(scene *ecs.Scene) (locationProxy, error) {
	if scene == nil {
		return nil, ErrLocationProxyMissing
	}
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return nil, ErrLocationProxyMissing
	}
	service, ok := component.(locationProxy)
	if !ok {
		return nil, ErrLocationProxyMissing
	}
	return service, nil
}

func getMessageSender(scene *ecs.Scene) (messageSender, error) {
	if scene == nil {
		return nil, ErrMessageSenderMissing
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return nil, ErrMessageSenderMissing
	}
	sender, ok := component.(messageSender)
	if !ok {
		return nil, ErrMessageSenderMissing
	}
	return sender, nil
}

func sceneActorID(scene *ecs.Scene) actor.ActorID {
	if scene == nil {
		return actor.ActorID{}
	}
	if fiberRef, ok := scene.Fiber().(interface {
		ID() int64
		ProcessID() int
	}); ok {
		return actor.ActorID{
			ProcessID:  fiberRef.ProcessID(),
			FiberID:    fiberRef.ID(),
			InstanceID: scene.InstanceID(),
		}
	}
	return actor.ActorID{}
}

func actorIDForEntity(scene *ecs.Scene, entity *ecs.Entity) actor.ActorID {
	if entity != nil {
		if component, ok := entity.GetComponent("MailBox"); ok {
			if mailbox, ok := component.(*actor.MailBox); ok {
				return mailbox.ActorID()
			}
		}
	}

	sceneActor := sceneActorID(scene)
	if entity == nil {
		return sceneActor
	}
	sceneActor.InstanceID = entity.InstanceID()
	return sceneActor
}
