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
	fiber.RegisterFiberInit(ecs.SceneTypeMatch, initMatchFiber)
}

func initMatchFiber(f *fiber.Fiber) error {
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
	scene.AddComponent(NewMatchComponent())
	RegisterMatchScene(scene)
	mailbox.RegisterHandler(MsgG2MatchMatch, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalG2MatchMatch(payload)
		if err != nil {
			return nil, err
		}
		return marshalG2CMatch(HandleMatch(scene, req))
	})
	return nil
}
