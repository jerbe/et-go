package login

import (
	"context"
	"net"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
)

type directGateSender struct {
	ecs.BaseComponent
	scene *ecs.Scene
}

func (s *directGateSender) Type() string { return "MessageSender" }
func (s *directGateSender) Call(_ context.Context, _ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
	req, err := unmarshalR2GGateAssign(payload)
	if err != nil {
		return nil, err
	}
	resp, err := HandleR2GGateAssign(s.scene, req)
	if err != nil {
		return nil, err
	}
	return marshalG2RGateAssign(resp)
}

func TestLoginFlow(t *testing.T) {
	realmFiber := fiber.New(context.Background(), ecs.SceneTypeRealm, 1, 1)
	realmScene := realmFiber.Root()
	realmScene.SetID(9003)
	realmScene.SetName("Realm")
	realmScene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	realmScene.AddComponent(&stubLocationProxy{})
	registry := &GateRegistryComponent{}
	realmScene.AddComponent(registry)
	gateScene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	gateScene.SetID(1)
	gateScene.AddComponent(NewGateSessionKeyComponent(0))
	gateScene.AddComponent(&PlayerComponent{})
	gateScene.AddComponent(&stubCentralSender{playerID: 3001})
	gateScene.AddComponent(&stubLocationProxy{locations: map[int64]actor.ActorID{
		3001: {ProcessID: 1, FiberID: 2, InstanceID: 3},
	}})
	centralFiber := fiber.New(context.Background(), ecs.SceneTypeCentral, 1, 1)
	centralFiber.Root().SetID(20001)
	centralFiber.Root().SetName("Central")
	actor.UpdateSceneRegistry(centralFiber.Root())
	defer actor.RemoveSceneRegistry(centralFiber.Root())
	registry.SetGates(1, []GateEndpoint{{
		GateId:  1,
		Address: "127.0.0.1:10000",
		ActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: gateScene.InstanceID()},
	}})
	realmScene.AddComponent(&directGateSender{scene: gateScene})

	token, err := GenerateAccessToken(3001)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	loginResp, err := HandleC2RLogin(realmScene, nil, &C2RLogin{
		RpcId:       1,
		AccessToken: token,
		ZoneId:      1,
	})
	if err != nil {
		t.Fatalf("HandleC2RLogin err = %v", err)
	}
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 2, serverConn, nil)
	session.SetEntity(ecs.NewEntity())
	gateResp, err := HandleC2GLoginGate(gateScene, session, &C2GLoginGate{
		RpcId:  2,
		Token:  loginResp.Token,
		GateId: loginResp.GateId,
	})
	if err != nil {
		t.Fatalf("HandleC2GLoginGate err = %v", err)
	}
	if gateResp.PlayerId == 0 {
		t.Fatal("expected player id")
	}
	pingResp, err := HandleC2GPing(session, &C2GPing{RpcId: 3})
	if err != nil {
		t.Fatalf("HandleC2GPing err = %v", err)
	}
	if pingResp.Time <= 0 {
		t.Fatal("expected ping time")
	}
}
