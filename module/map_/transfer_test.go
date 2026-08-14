package map_

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etmath "github.com/jerbe/et-go/engine/math"
	etlog "github.com/jerbe/et-go/internal/log"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/unit"
	"github.com/jerbe/et-go/proto"
	actorlocationpb "github.com/jerbe/et-go/proto/actorlocationpb"
	gproto "google.golang.org/protobuf/proto"
)

type stubLocationProxy struct {
	ecs.BaseComponent
	locks         int
	unlocks       int
	lastUnlockOld actor.ActorID
	lastUnlockNew actor.ActorID
}

func (c *stubLocationProxy) Type() string { return "LocationProxyComponent" }
func (c *stubLocationProxy) Lock(locationType int, key int64, actorID actor.ActorID, timeMs int) error {
	c.locks++
	return nil
}
func (c *stubLocationProxy) Unlock(locationType int, key int64, oldActorID, newActorID actor.ActorID) error {
	c.unlocks++
	c.lastUnlockOld = oldActorID
	c.lastUnlockNew = newActorID
	return nil
}

type stubMessageSender struct {
	ecs.BaseComponent
	target       *ecs.Scene
	lastTransfer *M2MUnitTransferRequest
}

func (c *stubMessageSender) Type() string { return "MessageSender" }
func (c *stubMessageSender) Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	_ = ctx
	_ = actorID
	if msgID != MsgM2MUnitTransferRequest {
		return nil, nil
	}
	req, err := unmarshalUnitTransferRequest(payload)
	if err != nil {
		return nil, err
	}
	c.lastTransfer = req
	resp := HandleUnitTransfer(c.target, req)
	return marshalUnitTransferResponse(&resp)
}

type stubNotifier struct {
	ecs.BaseComponent
	unitID   int64
	scene    string
	unitInfo *proto.UnitInfo
}

func (c *stubNotifier) Type() string { return "MessageLocationSenderComponent" }
func (c *stubNotifier) NotifyTransfer(unitID int64, sceneName string, unitInfo *proto.UnitInfo) error {
	c.unitID = unitID
	c.scene = sceneName
	c.unitInfo = unitInfo
	return nil
}

type transferLocationRPC struct {
	target actor.ActorID
}

func (c *transferLocationRPC) Call(_ context.Context, _ actor.ActorID, msgID uint16, _ []byte) ([]byte, error) {
	if msgID != actorlocation.MsgObjectGetRequest {
		return nil, errors.New("unexpected location RPC")
	}
	return gproto.Marshal(&actorlocationpb.ObjectGetResponse{
		ActorId: &actorlocationpb.ActorId{
			ProcessId:  int32(c.target.ProcessID),
			FiberId:    c.target.FiberID,
			InstanceId: c.target.InstanceID,
		},
	})
}

type transferNotificationSender struct {
	msgID   uint16
	payload []byte
}

type rejectedFrameScheduler struct{}

func (rejectedFrameScheduler) AddFrameFinishTask(func()) error {
	return fiber.ErrFiberClosed
}
func (rejectedFrameScheduler) ID() int64      { return 2 }
func (rejectedFrameScheduler) ProcessID() int { return 1 }

func (s *transferNotificationSender) Send(_ actor.ActorID, msgID uint16, payload []byte) error {
	s.msgID = msgID
	s.payload = append([]byte(nil), payload...)
	return nil
}

func (s *transferNotificationSender) Call(context.Context, actor.ActorID, uint16, []byte) ([]byte, error) {
	return nil, errors.New("unexpected notification RPC")
}

func TestHandleUnitTransferAndTransferAtFrameFinish(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	sourceFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	targetFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	if sourceFiber == nil || targetFiber == nil {
		t.Fatal("map fibers should be created")
	}

	sourceScene := sourceFiber.Root()
	targetScene := targetFiber.Root()

	sourceMapRaw, ok := sourceScene.GetComponent("MapUnitManagerComponent")
	if !ok {
		t.Fatal("source map manager missing")
	}
	targetMapRaw, ok := targetScene.GetComponent("MapUnitManagerComponent")
	if !ok {
		t.Fatal("target map manager missing")
	}
	sourceMapManager := sourceMapRaw.(*MapUnitManagerComponent)
	targetMapManager := targetMapRaw.(*MapUnitManagerComponent)
	sourceMapManager.MapName = "Map1"
	targetMapManager.MapName = "Map2"
	if err := sourceMapManager.SetTarget("Map2", actor.ActorID{ProcessID: 1, FiberID: targetFiber.ID(), InstanceID: targetScene.InstanceID()}); err != nil {
		t.Fatalf("SetTarget error = %v", err)
	}

	sourceLocation := &stubLocationProxy{}
	targetLocation := &stubLocationProxy{}
	sourceScene.AddComponent(sourceLocation)
	targetScene.AddComponent(targetLocation)

	sourceSender := &stubMessageSender{target: targetScene}
	sourceScene.AddComponent(sourceSender)
	targetScene.AddComponent(&stubMessageSender{})

	notifier := &stubNotifier{}
	targetScene.AddComponent(notifier)

	player, err := unit.CreatePlayer(sourceScene, 101)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	player.SetPosition(etmath.Vector3{X: 2, Y: 0, Z: 3})

	done := make(chan error, 1)
	go func() {
		done <- TransferAtFrameFinish(sourceScene, player, actor.ActorID{ProcessID: 1, FiberID: targetFiber.ID(), InstanceID: targetScene.InstanceID()}, "Map2")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TransferAtFrameFinish error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transfer should finish")
	}

	if !player.IsDisposed() {
		t.Fatal("source unit should be disposed after transfer")
	}
	sourceUnitsRaw, ok := sourceScene.GetComponent("UnitComponent")
	if !ok {
		t.Fatal("source unit component missing")
	}
	if _, ok := sourceUnitsRaw.(*unit.UnitComponent).Get(101); ok {
		t.Fatal("source scene should remove transferred unit")
	}
	targetUnitsRaw, ok := targetScene.GetComponent("UnitComponent")
	if !ok {
		t.Fatal("target unit component missing")
	}
	targetUnits := targetUnitsRaw.(*unit.UnitComponent)
	transferred, ok := targetUnits.Get(101)
	if !ok || transferred == nil {
		t.Fatal("target scene should contain transferred unit")
	}
	if transferred.Position() != (etmath.Vector3{X: 2, Y: 0, Z: 3}) {
		t.Fatalf("transferred position = %v, want (2,0,3)", transferred.Position())
	}
	if sourceLocation.locks != 1 {
		t.Fatalf("source location locks = %d, want 1", sourceLocation.locks)
	}
	if targetLocation.unlocks != 1 {
		t.Fatalf("target location unlocks = %d, want 1", targetLocation.unlocks)
	}
	if sourceLocation.unlocks != 0 {
		t.Fatalf("source location unlocks = %d, want 0 after committed transfer", sourceLocation.unlocks)
	}
	if notifier.unitID != 101 || notifier.unitInfo == nil || notifier.scene != "Map2" {
		t.Fatalf("notification mismatch: unitID=%d scene=%q info=%+v", notifier.unitID, notifier.scene, notifier.unitInfo)
	}
	if !targetLocation.lastUnlockNew.IsValid() {
		t.Fatalf("target location unlock new actor missing")
	}
	if targetLocation.lastUnlockNew.ProcessID != 1 {
		t.Fatalf("target location unlock new actor mismatched %v", targetLocation.lastUnlockNew)
	}
	if sourceSender.lastTransfer == nil {
		t.Fatal("transfer request should be captured")
	}
	duplicate := HandleUnitTransfer(targetScene, sourceSender.lastTransfer)
	if duplicate.Error != 0 || duplicate.RpcID != sourceSender.lastTransfer.RpcID {
		t.Fatalf("duplicate transfer response = %+v, want cached success", duplicate)
	}
	conflict := *sourceSender.lastTransfer
	conflict.Unit = append([]byte(nil), conflict.Unit...)
	conflict.Unit[0] ^= 0xff
	conflictResponse := HandleUnitTransfer(targetScene, &conflict)
	if conflictResponse.Error == 0 || conflictResponse.Message != ErrTransferCorrelationConflict.Error() {
		t.Fatalf("transfer correlation conflict response = %+v", conflictResponse)
	}
}

func TestReportTransferFailureUsesClientResponseOpcode(t *testing.T) {
	scene := newJournalTestScene()
	proxy := &actorlocation.LocationProxyComponent{}
	proxy.SetLocationActor(actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3})
	proxy.SetCaller(&transferLocationRPC{
		target: actor.ActorID{ProcessID: 1, FiberID: 4, InstanceID: 5},
	})
	sender := &transferNotificationSender{}
	locationSender := &actorlocation.MessageLocationSenderComponent{}
	locationSender.SetDependencies(proxy, sender)
	scene.AddComponent(locationSender)
	defer scene.Dispose()

	if err := reportTransferFailure(scene, 42, 7, errors.New("target rejected")); err != nil {
		t.Fatalf("reportTransferFailure error = %v", err)
	}
	if sender.msgID != MsgM2CTransferMap {
		t.Fatalf("notification msg id = %d, want %d", sender.msgID, MsgM2CTransferMap)
	}
	response, err := unmarshalTransferMapResponse(sender.payload)
	if err != nil {
		t.Fatalf("unmarshal failure response: %v", err)
	}
	if response.RpcID != 7 || response.Error == 0 || response.Message != "target rejected" {
		t.Fatalf("failure response = %+v", response)
	}
}

func TestHandleTransferMapRejectsNilScene(t *testing.T) {
	response := HandleTransferMap(nil, actor.ActorID{}, C2MTransferMap{RpcID: 9})
	if response.RpcID != 9 || response.Error == 0 || response.Message == "" {
		t.Fatalf("nil scene response = %+v", response)
	}
}

func TestHandleTransferMapRejectsClosedFrameScheduler(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	scene.SetFiber(rejectedFrameScheduler{})
	scene.AddComponent(&unit.UnitComponent{})
	manager := &MapUnitManagerComponent{MapName: "Map1"}
	if err := manager.SetTarget("Map2", actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}); err != nil {
		t.Fatalf("SetTarget error = %v", err)
	}
	scene.AddComponent(manager)
	player, err := unit.CreatePlayer(scene, 101)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	defer scene.Dispose()

	response := HandleTransferMap(scene, actor.ActorID{
		ProcessID:  1,
		FiberID:    2,
		InstanceID: player.InstanceID(),
	}, C2MTransferMap{RpcID: 10})
	if response.RpcID != 10 || response.Error == 0 || response.Message != fiber.ErrFiberClosed.Error() {
		t.Fatalf("closed scheduler response = %+v", response)
	}
	if player.IsDisposed() {
		t.Fatal("scheduler rejection must not dispose source unit")
	}
}

func TestHandleTransferMapRequiresRoutedUnitActor(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	scene.AddComponent(&unit.UnitComponent{})
	player, err := unit.CreatePlayer(scene, 101)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	defer scene.Dispose()

	response := HandleTransferMap(scene, actor.ActorID{}, C2MTransferMap{RpcID: 11})
	if response.Error == 0 || response.Message != ErrTransferUnitMissing.Error() {
		t.Fatalf("missing routed actor response = %+v", response)
	}
	if player.IsDisposed() {
		t.Fatal("missing routed actor must not dispose source unit")
	}
}

func TestHandleUnitTransferRequiresValidRequestCorrelation(t *testing.T) {
	response := HandleUnitTransfer(ecs.NewScene(ecs.SceneTypeMap, 1, "Map1"), &M2MUnitTransferRequest{
		RpcID: 0,
	})
	if response.Error == 0 || response.RpcID != 0 || response.Message != ErrTransferRequestInvalid.Error() {
		t.Fatalf("invalid transfer request response = %+v", response)
	}
}

func TestTransferJournalRequiresValidSourceActor(t *testing.T) {
	journal := &TransferJournalComponent{Store: &fakeTransferJournalStore{}}
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	request := &M2MUnitTransferRequest{
		RpcID:      1,
		OldActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
		Unit:       mustMarshalUnitSnapshot(t, 101),
	}

	if _, err := journal.Begin(context.Background(), scene, request,
		actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 4}, "Map2"); !errors.Is(err, ErrTransferRequestInvalid) {
		t.Fatalf("Begin error = %v, want %v", err, ErrTransferRequestInvalid)
	}
}

func TestUnitForActorRequiresExactActorAddress(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "Map1")
	sceneFiber := fiber.New(context.Background(), ecs.SceneTypeMap, 1, 1)
	scene.SetFiber(sceneFiber)
	scene.AddComponent(&unit.UnitComponent{})
	player, err := unit.CreatePlayer(scene, 101)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	defer scene.Dispose()

	exact := actorIDForEntity(scene, &player.Entity)
	if unitForActor(scene, exact) != player {
		t.Fatal("exact actor address should resolve player")
	}
	wrongFiber := exact
	wrongFiber.FiberID++
	if unitForActor(scene, wrongFiber) != nil {
		t.Fatal("actor with same InstanceID but wrong FiberID must not resolve player")
	}
}
