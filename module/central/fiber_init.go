package central

import (
	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/gamelogin"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeCentral, initCentralFiber)
}

func initCentralFiber(f *fiber.Fiber) error {
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
	dbManager := &db.DBManagerComponent{}
	if cfg := config.GetGlobal(); cfg != nil {
		dbManager.SetConfig(cfg)
	}
	scene.AddComponent(dbManager)

	mailbox.RegisterHandler(MsgR2CentralAccountLogin, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalR2CentralAccountLogin(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleAccountLogin(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalAccountLoginResponse(resp)
	})
	mailbox.RegisterHandler(gamelogin.MsgG2GameLogin, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := gamelogin.UnmarshalG2GameLogin(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleGameLogin(scene, req)
		if err != nil {
			return nil, err
		}
		return gamelogin.MarshalGame2GLogin(resp)
	})
	actor.UpdateSceneRegistry(scene)
	return nil
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
