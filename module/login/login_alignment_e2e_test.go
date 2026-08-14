package login_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/central"
	login "github.com/jerbe/et-go/module/login"
	_ "github.com/jerbe/et-go/module/map_"
	"github.com/jerbe/et-go/module/unit"
)

type alignmentProfileStore struct{}

func (alignmentProfileStore) LoadOrCreatePlayerProfile(_ context.Context, zone int, accountID int64) (*db.CPlayerProfile, error) {
	return &db.CPlayerProfile{
		Id:        accountID,
		ZoneId:    int32(zone),
		AccountId: accountID,
		ShortId:   "alignment",
		CreatedAt: time.Now(),
	}, nil
}

func TestLoginFlowAlignment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	world := ecs.NewWorld()
	oldConfig := config.GetGlobal()
	config.SetGlobal(&config.Config{
		Machines:  []config.StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1", OuterIP: "127.0.0.1"}},
		Processes: []config.StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []config.StartSceneConfig{
			{ID: 3001, ProcessID: 1, Zone: 1, SceneType: "Location", Name: "Location"},
			{ID: 20001, ProcessID: 1, Zone: 1, SceneType: "Central", Name: "Central"},
			{ID: 18001, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Home"},
			{ID: 9004, ProcessID: 1, Zone: 1, SceneType: "Gate", Name: "Gate", OuterPort: 0},
			{ID: 9003, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "Realm", OuterPort: 0},
		},
		Zones: []config.StartZoneConfig{{ID: 1, Name: "test", DBName: "test", DBAddr: "mongodb://127.0.0.1:27017"}},
	})
	t.Cleanup(func() { config.SetGlobal(oldConfig) })
	manager := fiber.NewManager(ctx, world, nil)
	t.Cleanup(func() {
		manager.StopAll()
		world.Shutdown()
	})

	locationFiber := manager.Create(ecs.SceneTypeLocation, 1, 1, nil)
	centralFiber := manager.Create(ecs.SceneTypeCentral, 1, 1, nil)
	mapFiber := manager.Create(ecs.SceneTypeMap, 1, 1, nil)
	gateFiber := manager.Create(ecs.SceneTypeGate, 1, 1, nil)
	realmFiber := manager.Create(ecs.SceneTypeRealm, 1, 1, nil)
	if locationFiber == nil || centralFiber == nil || mapFiber == nil || gateFiber == nil || realmFiber == nil {
		t.Fatal("expected all fibers created")
	}

	locationScene := locationFiber.Root()
	locationScene.SetID(3001)
	locationScene.SetName("Location")
	actor.UpdateSceneRegistry(locationScene)

	centralScene := centralFiber.Root()
	centralScene.SetID(20001)
	centralScene.SetName("Central")
	profileStore := &central.PlayerProfileStoreComponent{}
	profileStore.SetStore(alignmentProfileStore{})
	centralScene.AddComponent(profileStore)
	actor.UpdateSceneRegistry(centralScene)

	mapScene := mapFiber.Root()
	mapScene.SetID(18001)
	mapScene.SetName("Home")
	actor.UpdateSceneRegistry(mapScene)

	gateScene := gateFiber.Root()
	gateScene.SetID(9004)
	gateScene.SetName("Gate")
	actor.UpdateSceneRegistry(gateScene)

	realmScene := realmFiber.Root()
	realmScene.SetID(9003)
	realmScene.SetName("Realm")
	actor.UpdateSceneRegistry(realmScene)

	registry := &login.GateRegistryComponent{}
	realmScene.AddComponent(registry)
	registry.SetGates(1, []login.GateEndpoint{{
		GateId:  gateScene.ID(),
		Address: "127.0.0.1:10000",
		ActorID: actor.SceneActorID(gateScene),
	}})

	token, err := login.GenerateAccessToken(3001)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	loginResp, err := login.HandleC2RLogin(realmScene, nil, &login.C2RLogin{
		RpcId:       1,
		AccessToken: token,
		ZoneId:      1,
	})
	if err != nil {
		t.Fatalf("HandleC2RLogin err = %v", err)
	}
	if loginResp.Token == "" {
		t.Fatal("expected gate token")
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 2, serverConn, nil)
	session.SetEntity(ecs.NewEntity())
	session.Entity().AddComponent(network.NewSessionAcceptTimeoutComponent(session, time.Second))

	gateResp, err := login.HandleC2GLoginGate(gateScene, session, &login.C2GLoginGate{
		RpcId:  2,
		Token:  loginResp.Token,
		GateId: loginResp.GateId,
	})
	if err != nil {
		t.Fatalf("HandleC2GLoginGate err = %v", err)
	}
	if gateResp.PlayerId != 3001 {
		t.Fatalf("gateResp.PlayerId = %d, want 3001", gateResp.PlayerId)
	}
	if _, ok := session.Entity().GetComponent("SessionAcceptTimeoutComponent"); ok {
		t.Fatal("SessionAcceptTimeoutComponent should be removed")
	}

	locationProxyRaw, ok := gateScene.GetComponent("LocationProxyComponent")
	if !ok || locationProxyRaw == nil {
		t.Fatal("LocationProxyComponent missing")
	}
	locationProxy, ok := locationProxyRaw.(*actorlocation.LocationProxyComponent)
	if !ok {
		t.Fatal("unexpected LocationProxyComponent type")
	}

	unitActorID, err := locationProxy.Get(int(actorlocation.LocationTypeUnit), gateResp.PlayerId)
	if err != nil {
		t.Fatalf("get unit location err = %v", err)
	}
	unitComponentRaw, ok := mapScene.GetComponent("UnitComponent")
	if !ok || unitComponentRaw == nil {
		t.Fatal("UnitComponent missing")
	}
	unitComponent := unitComponentRaw.(*unit.UnitComponent)
	mapUnit, ok := unitComponent.Get(gateResp.PlayerId)
	if !ok || mapUnit == nil {
		t.Fatal("map unit missing")
	}
	wantUnitActor := actor.ActorID{
		ProcessID:  mapFiber.ProcessID(),
		FiberID:    mapFiber.ID(),
		InstanceID: mapUnit.InstanceID(),
	}
	if unitActorID != wantUnitActor {
		t.Fatalf("unitActorID = %+v, want %+v", unitActorID, wantUnitActor)
	}

	playerActorID, err := locationProxy.Get(int(actorlocation.LocationTypePlayer), gateResp.PlayerId)
	if err != nil {
		t.Fatalf("get player location err = %v", err)
	}
	playerComponentRaw, ok := gateScene.GetComponent("PlayerComponent")
	if !ok || playerComponentRaw == nil {
		t.Fatal("PlayerComponent missing")
	}
	playerComponent := playerComponentRaw.(*login.PlayerComponent)
	player, ok := playerComponent.Get(3001)
	if !ok || player == nil {
		t.Fatal("player missing")
	}
	wantPlayerActor := actor.ActorID{
		ProcessID:  gateFiber.ProcessID(),
		FiberID:    gateFiber.ID(),
		InstanceID: player.InstanceID(),
	}
	if playerActorID != wantPlayerActor {
		t.Fatalf("playerActorID = %+v, want %+v", playerActorID, wantPlayerActor)
	}

	gateSessionActorID, err := locationProxy.Get(int(actorlocation.LocationTypeGateSession), gateResp.PlayerId)
	if err != nil {
		t.Fatalf("get gate session location err = %v", err)
	}
	wantSessionActor := actor.ActorID{
		ProcessID:  gateFiber.ProcessID(),
		FiberID:    gateFiber.ID(),
		InstanceID: session.Entity().InstanceID(),
	}
	if gateSessionActorID != wantSessionActor {
		t.Fatalf("gateSessionActorID = %+v, want %+v", gateSessionActorID, wantSessionActor)
	}

	accountActorID, err := locationProxy.Get(int(actorlocation.LocationTypeAccount), 3001)
	if err != nil {
		t.Fatalf("get account lock err = %v", err)
	}
	if accountActorID.IsValid() {
		t.Fatalf("account lock should be cleared, got %+v", accountActorID)
	}
}
