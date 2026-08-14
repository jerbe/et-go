package login

import (
	"context"
	"testing"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

func TestLoginFiberInitAlignment(t *testing.T) {
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, nil)
	oldConfig := config.GetGlobal()
	config.SetGlobal(&config.Config{
		Machines:  []config.StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1", OuterIP: "127.0.0.1"}},
		Processes: []config.StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []config.StartSceneConfig{
			{ID: 9003, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "Realm", OuterPort: 0},
			{ID: 9004, ProcessID: 1, Zone: 1, SceneType: "Gate", Name: "Gate", OuterPort: 0},
		},
		Zones: []config.StartZoneConfig{{ID: 1, Name: "test", DBName: "test", DBAddr: "mongodb://127.0.0.1:27017"}},
	})
	t.Cleanup(func() { config.SetGlobal(oldConfig) })
	t.Cleanup(func() {
		manager.StopAll()
		world.Shutdown()
	})

	realmFiber := manager.Create(ecs.SceneTypeRealm, 1, 1, nil)
	gateFiber := manager.Create(ecs.SceneTypeGate, 1, 1, nil)
	if realmFiber == nil || gateFiber == nil {
		t.Fatal("expected realm and gate fibers")
	}

	realmScene := realmFiber.Root()
	if _, ok := realmScene.GetComponent("DBManagerComponent"); !ok {
		t.Fatal("realm should install DBManagerComponent")
	}
	if _, ok := realmScene.GetComponent("GateRegistryComponent"); ok {
		t.Fatal("realm should not install GateRegistryComponent")
	}
	if _, ok := realmScene.GetComponent("LocationProxyComponent"); !ok {
		t.Fatal("realm should install LocationProxyComponent")
	}
	if _, ok := realmScene.GetComponent("NetComponent"); !ok {
		t.Fatal("realm should install NetComponent")
	}

	gateScene := gateFiber.Root()
	if _, ok := gateScene.GetComponent("DBManagerComponent"); ok {
		t.Fatal("gate should not install DBManagerComponent")
	}
	if _, ok := gateScene.GetComponent("MessageLocationSenderComponent"); !ok {
		t.Fatal("gate should install MessageLocationSenderComponent")
	}
	if _, ok := gateScene.GetComponent("GateSessionKeyComponent"); !ok {
		t.Fatal("gate should install GateSessionKeyComponent")
	}
	if _, ok := gateScene.GetComponent("PlayerComponent"); !ok {
		t.Fatal("gate should install PlayerComponent")
	}
	if _, ok := gateScene.GetComponent("NetComponent"); !ok {
		t.Fatal("gate should install NetComponent")
	}
}
