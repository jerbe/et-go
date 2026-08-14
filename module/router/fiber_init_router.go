package router

import (
	"net"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/timer"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeRouterNode, initRouterFiber)
}

func initRouterFiber(f *fiber.Fiber) error {
	scene := f.Root()
	scene.AddComponent(actor.NewMailBox(sceneActorID(scene), actor.MailBoxTypeUnOrderedMessage))
	scene.AddComponent(&timer.TimerComponent{})
	scene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	component := &RouterComponent{}
	bindAddr, innerIP, err := resolveRouterRuntimeConfig(scene)
	if err != nil {
		return err
	}
	outerTransport, err := newUDPTransport(bindAddr)
	if err != nil {
		return err
	}
	innerTransport, err := newUDPTransport(net.JoinHostPort(innerIP, "0"))
	if err != nil {
		_ = outerTransport.Close()
		return err
	}
	tcpTransport, err := newTCPTransport(bindAddr)
	if err != nil {
		_ = innerTransport.Close()
		_ = outerTransport.Close()
		return err
	}
	// TODO(router-transport): the target WebGL build uses WebSocket instead of
	// TCP/UDP. The Go repository has no WebSocket client/transport selection
	// contract yet; do not silently advertise TCP as WebSocket compatibility.
	component.SetOuterTransport(outerTransport)
	component.SetOuterTCPTransport(tcpTransport)
	component.SetInnerTransport(innerTransport)
	component.SetInnerIP(innerIP)
	if addr, err := net.ResolveUDPAddr("udp", bindAddr); err == nil {
		component.SetOuterAddr(addr)
	}
	scene.AddComponent(component)
	system := NewRouterComponentSystem(component)
	f.RegisterUpdate(system)
	actor.UpdateSceneRegistry(scene)
	return nil
}
