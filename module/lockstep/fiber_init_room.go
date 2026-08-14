package lockstep

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/actorlocation"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeRoom, initRoomFiber)
}

func initRoomFiber(f *fiber.Fiber) error {
	scene := f.Root()
	mailbox := actor.NewMailBox(sceneActorID(scene), actor.MailBoxTypeUnOrderedMessage)
	scene.AddComponent(mailbox)
	scene.AddComponent(&timer.TimerComponent{})
	scene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	innerSender := actor.NewProcessInnerSender(f.ProcessID(), nil, actor.NewRpcManager())
	scene.AddComponent(innerSender)
	messageSender := actor.NewMessageSender(f.ProcessID(), innerSender, nil)
	scene.AddComponent(messageSender)
	locationProxy := &actorlocation.LocationProxyComponent{}
	locationProxy.SetCaller(messageSender)
	scene.AddComponent(locationProxy)
	locationSenderComponent := &actorlocation.MessageLocationSenderComponent{}
	locationSenderComponent.SetDependencies(locationProxy, messageSender)
	scene.AddComponent(locationSenderComponent)
	roomServer := NewRoomServerComponent(nil)
	updater := NewLSServerUpdater(nil)
	updater.SetSnapshotProvider(func(current *LockstepRoom) ([]byte, error) {
		return MarshalRoomSnapshot(current, roomServer)
	})
	scene.AddComponent(roomServer)
	roomServer.SetFiberManager(f.Manager())
	scene.AddComponent(updater)
	f.RegisterUpdate(updater)

	// 外部 Room 消息由 gate_room_handlers.go 在 Gate 侧使用已认证玩家的
	// PlayerRoomComponent.RoomActorID 路由到这里；本 Fiber 只负责 Room
	// MailBox 内部业务处理。
	mailbox.RegisterHandler(MsgFrameMessage, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalFrameMessageRequest(payload)
		if err != nil {
			return nil, err
		}
		return marshalFrameMessageResponse(HandleFrameMessage(scene, req))
	})
	mailbox.RegisterHandler(MsgC2RoomChangeSceneFinish, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalChangeSceneFinish(payload)
		if err != nil {
			return nil, err
		}
		return marshalRoom2CStart(HandleChangeSceneFinish(scene, req))
	})
	mailbox.RegisterHandler(MsgC2RoomCheckHash, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalCheckHashRequest(payload)
		if err != nil {
			return nil, err
		}
		resp, err := handleCheckHash(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalCheckHashResponse(resp)
	})
	mailbox.RegisterHandler(MsgG2RoomReconnect, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalReconnectRequest(payload)
		if err != nil {
			return nil, err
		}
		resp, err := handleReconnect(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalRoom2CReconnect(resp)
	})
	mailbox.RegisterHandler(MsgM2RoomPlayerOffline, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalPlayerOffline(payload)
		if err != nil {
			return nil, err
		}
		return marshalPlayerOffline(HandlePlayerOffline(scene, req))
	})
	return nil
}
