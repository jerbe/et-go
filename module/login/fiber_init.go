package login

import (
	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/gate"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeRealm, initRealmFiber)
	fiber.RegisterFiberInit(ecs.SceneTypeGate, initGateFiber)
}

func initRealmFiber(f *fiber.Fiber) error {
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
	dbManager := &db.DBManagerComponent{}
	if cfg := config.GetGlobal(); cfg != nil {
		dbManager.SetConfig(cfg)
	}
	scene.AddComponent(dbManager)
	addr, err := network.ResolveSceneListenAddr(scene, true)
	if err != nil {
		return err
	}
	netComponent := network.NewNetComponent("kcp", addr)
	netComponent.SetPacketHandler(realmPacketHandler)
	if err := netComponent.Start(); err != nil {
		return err
	}
	scene.AddComponent(netComponent)

	mailbox.RegisterHandler(MsgC2RLogin, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalC2RLogin(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleC2RLogin(scene, nil, req)
		if err != nil {
			return nil, err
		}
		return marshalR2CLogin(resp)
	})
	actor.UpdateSceneRegistry(scene)
	return nil
}

func initGateFiber(f *fiber.Fiber) error {
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
	scene.AddComponent(NewGateSessionKeyComponent(0))
	scene.AddComponent(&PlayerComponent{})
	addr, err := network.ResolveSceneListenAddr(scene, true)
	if err != nil {
		return err
	}
	netComponent := network.NewNetComponent("kcp", addr)
	gateHandler := &gate.GateMessageHandler{}
	netComponent.SetPacketHandler(func(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
		return gateHandler.Handle(scene, session, packet)
	})
	if err := netComponent.Start(); err != nil {
		return err
	}
	scene.AddComponent(netComponent)

	mailbox.RegisterHandler(MsgR2GGateAssign, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalR2GGateAssign(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleR2GGateAssign(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalG2RGateAssign(resp)
	})
	actor.UpdateSceneRegistry(scene)
	return nil
}

func realmPacketHandler(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
	if packet == nil {
		return nil, codec.ErrInvalidPacket
	}
	if packet.MsgID != MsgC2RLogin {
		return nil, ErrMessageHandlerMissing
	}
	req, err := unmarshalC2RLogin(packet.Payload)
	if err != nil {
		return nil, err
	}
	resp, err := HandleC2RLogin(scene, session, req)
	if err != nil {
		return nil, err
	}
	payload, err := marshalR2CLogin(resp)
	if err != nil {
		return nil, err
	}
	return &codec.Packet{
		Type:    codec.PacketTypeResponse,
		MsgID:   MsgR2CLogin,
		RpcID:   packet.RpcID,
		Payload: payload,
	}, nil
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
	sceneActor := sceneActorID(scene)
	if entity == nil {
		return sceneActor
	}
	sceneActor.InstanceID = entity.InstanceID()
	return sceneActor
}
