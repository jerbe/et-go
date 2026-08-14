package router

import (
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/http"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeRouter, initRouterManagerFiber)
}

func initRouterManagerFiber(f *fiber.Fiber) error {
	scene := f.Root()
	scene.AddComponent(actor.NewMailBox(sceneActorID(scene), actor.MailBoxTypeUnOrderedMessage))
	scene.AddComponent(&timer.TimerComponent{})
	innerSender := actor.NewProcessInnerSender(f.ProcessID(), nil, actor.NewRpcManager())
	scene.AddComponent(innerSender)
	scene.AddComponent(actor.NewMessageSender(f.ProcessID(), innerSender, nil))
	addr, err := network.ResolveSceneListenAddr(scene, false)
	if err != nil {
		return err
	}
	httpComp := http.NewBareHttpComponent(addr)
	scene.AddComponent(httpComp)
	httpComp.Dispatcher().Register("/login", &httpRouterLoginHandler{})
	httpComp.Dispatcher().Register("/router/list", &httpRouterListHandler{})
	httpComp.Dispatcher().Register("/zone/list", &httpZoneListHandler{})
	httpComp.Dispatcher().Register("/zone/last", &httpLastZoneHandler{})
	if err := httpComp.Start(); err != nil {
		return err
	}
	actor.UpdateSceneRegistry(scene)
	return nil
}
