package map_

import (
	"fmt"
	"strings"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/aoi"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/lockstep"
	"github.com/jerbe/et-go/module/maprpc"
	"github.com/jerbe/et-go/module/move"
	"github.com/jerbe/et-go/module/statesync"
	"github.com/jerbe/et-go/module/unit"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeMap, initMapFiber)
	gate.RegisterLocationRequestWithResponse(MsgC2MTransferMap, MsgM2CTransferMap)
	statesync.RegisterUnitMessageHandler(MsgC2MTransferMap, handleOrderedTransferMap)
}

func handleOrderedTransferMap(scene *ecs.Scene, targetActorID actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
	req, err := unmarshalTransferMap(payload)
	if err != nil {
		return nil, err
	}
	resp := HandleTransferMap(scene, targetActorID, *req)
	return marshalTransferMapResponse(&resp)
}

func initMapFiber(f *fiber.Fiber) error {
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

	roomManager := &lockstep.RoomManagerComponent{}
	roomManager.SetFiberManager(f.Manager())
	scene.AddComponent(roomManager)
	dbManager := &db.DBManagerComponent{}
	if cfg := config.GetGlobal(); cfg != nil {
		dbManager.SetConfig(cfg)
	}
	scene.AddComponent(dbManager)
	if config.GetGlobal() != nil {
		// 生产配置存在时，transfer 必须先写 durable journal；没有配置的
		// 进程内测试不安装该组件，避免把测试替身冒充生产恢复能力。
		scene.AddComponent(&TransferJournalComponent{})
	}
	mapManager := &MapUnitManagerComponent{MapName: scene.Name()}
	scene.AddComponent(mapManager)
	if sceneCfg, ok := configuredMapScene(scene); ok && strings.TrimSpace(sceneCfg.NavMeshFile) != "" {
		finder, err := move.NewFinder(scene.Name(), strings.TrimSpace(sceneCfg.NavMeshFile))
		if err != nil {
			return fmt.Errorf("map %q create path finder: %w", scene.Name(), err)
		}
		mapManager.SetPathfindingFinder(finder)
	}
	scene.AddComponent(&unit.UnitComponent{})
	scene.AddComponent(&aoi.AOIManagerComponent{})
	scene.AddComponent(&TransferLedgerComponent{
		RequireDurable: config.GetGlobal() != nil,
	})
	scene.AddComponent(&UnitDumperComponent{})
	// 导航网格配置存在但没有部署层 Finder 工厂时，initMapFiber 直接失败；
	// 没有 navMeshFile 的地图则保留严格的 ErrFinderMissing 语义。
	statesync.RegisterAOIHandlers(scene.EventBus())
	lockstep.RegisterMapScene(scene)

	registerMapHandlers(scene, mailbox)
	inventory.RegisterMapHandlers(scene, mailbox)
	lockstep.RegisterMapHandlers(scene, mailbox)
	actor.UpdateSceneRegistry(scene)
	return nil
}

func configuredMapScene(scene *ecs.Scene) (config.StartSceneConfig, bool) {
	cfg := config.GetGlobal()
	if cfg == nil || scene == nil {
		return config.StartSceneConfig{}, false
	}
	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.ID == int(scene.ID()) {
			return sceneCfg, true
		}
	}
	return config.StartSceneConfig{}, false
}

func registerMapHandlers(scene *ecs.Scene, mailbox *actor.MailBox) {
	if scene == nil || mailbox == nil {
		return
	}

	mailbox.RegisterHandler(MsgC2MTransferMap, func(targetActorID actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalTransferMap(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleTransferMap(scene, targetActorID, *req)
		return marshalTransferMapResponse(&resp)
	})
	mailbox.RegisterHandler(MsgG2MEnterMap, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := maprpc.UnmarshalG2MEnterMap(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleG2MEnterMap(scene, req)
		return maprpc.MarshalM2GEnterMap(resp)
	})

	mailbox.RegisterHandler(MsgM2MUnitTransferRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalUnitTransferRequest(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleUnitTransfer(scene, req)
		return marshalUnitTransferResponse(&resp)
	})
}
