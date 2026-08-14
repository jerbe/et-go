package login

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/module/gamelogin"
)

type stubLocationProxy struct {
	ecs.BaseComponent
	locations map[int64]actor.ActorID
	unlocks   int
}

func (s *stubLocationProxy) Type() string { return "LocationProxyComponent" }
func (s *stubLocationProxy) Add(_ int, key int64, actorID actor.ActorID) error {
	if s.locations == nil {
		s.locations = make(map[int64]actor.ActorID)
	}
	s.locations[key] = actorID
	return nil
}
func (s *stubLocationProxy) Get(_ int, key int64) (actor.ActorID, error) {
	return s.locations[key], nil
}
func (s *stubLocationProxy) Lock(_ int, _ int64, _ actor.ActorID, _ int) error { return nil }
func (s *stubLocationProxy) Unlock(_ int, _ int64, _, _ actor.ActorID) error {
	s.unlocks++
	return nil
}

type stubCentralSender struct {
	ecs.BaseComponent
	playerID int64
}

func (s *stubCentralSender) Type() string { return "MessageSender" }
func (s *stubCentralSender) Call(_ context.Context, _ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
	req, err := gamelogin.UnmarshalG2GameLogin(payload)
	if err != nil {
		return nil, err
	}
	return gamelogin.MarshalGame2GLogin(&gamelogin.Game2GLogin{
		RpcId:     req.RpcId,
		AccountId: req.AccountId,
		PlayerId:  s.playerID,
		ZoneId:    1,
	})
}

func TestHandleC2GLoginGate(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	keys := NewGateSessionKeyComponent(0)
	scene.AddComponent(keys)
	scene.AddComponent(&PlayerComponent{})
	locationProxy := &stubLocationProxy{locations: map[int64]actor.ActorID{
		2001: {ProcessID: 1, FiberID: 2, InstanceID: 3},
	}}
	scene.AddComponent(locationProxy)
	scene.AddComponent(&stubCentralSender{playerID: 2001})
	centralFiber := fiber.New(context.Background(), ecs.SceneTypeCentral, 1, 1)
	centralFiber.Root().SetID(20001)
	centralFiber.Root().SetName("Central")
	actor.UpdateSceneRegistry(centralFiber.Root())
	defer actor.RemoveSceneRegistry(centralFiber.Root())
	keys.Add("gate", 2001)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 1, serverConn, nil)
	session.SetEntity(ecs.NewEntity())

	resp, err := HandleC2GLoginGate(scene, session, &C2GLoginGate{
		RpcId:  1,
		Token:  "gate",
		GateId: 1,
	})
	if err != nil {
		t.Fatalf("HandleC2GLoginGate err = %v", err)
	}
	if resp.Error != 0 || resp.PlayerId != 2001 {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if !session.IsAuthed() {
		t.Fatal("session should be authed")
	}
	playerComponent, _ := scene.GetComponent("PlayerComponent")
	players := playerComponent.(*PlayerComponent)
	if players.Count() != 1 {
		t.Fatalf("players.Count() = %d", players.Count())
	}
	if session.Entity().Scene() != scene {
		t.Fatal("session entity should be registered to scene")
	}
	if _, ok := session.Entity().GetComponent("GateSessionComponent"); !ok {
		t.Fatal("GateSessionComponent missing")
	}
	if _, ok := session.Entity().GetComponent("SessionPlayerComponent"); !ok {
		t.Fatal("SessionPlayerComponent missing")
	}
	if locationProxy.unlocks != 1 {
		t.Fatalf("account unlocks = %d, want 1", locationProxy.unlocks)
	}
}

func TestHandleC2GLoginGateInvalidToken(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	scene.AddComponent(NewGateSessionKeyComponent(0))
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 1, serverConn, nil)
	session.SetEntity(ecs.NewEntity())

	resp, err := HandleC2GLoginGate(scene, session, &C2GLoginGate{RpcId: 1, Token: "bad"})
	if err != nil {
		t.Fatalf("HandleC2GLoginGate err = %v", err)
	}
	if resp.Error != ERR_ConnectGateKeyError {
		t.Fatalf("resp.Error = %d", resp.Error)
	}
	if resp.Message != "Gate key验证失败!" {
		t.Fatalf("resp.Message = %q", resp.Message)
	}
	time.Sleep(20 * time.Millisecond)
	if !session.IsClosed() {
		t.Fatal("session should be closed")
	}
}

func TestHandleC2GLoginGateUnlocksAccountWhenHomeMapFails(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	keys := NewGateSessionKeyComponent(0)
	scene.AddComponent(keys)
	scene.AddComponent(&PlayerComponent{})
	locationProxy := &stubLocationProxy{}
	scene.AddComponent(locationProxy)
	scene.AddComponent(&stubCentralSender{playerID: 2001})
	centralFiber := fiber.New(context.Background(), ecs.SceneTypeCentral, 1, 1)
	centralFiber.Root().SetID(20001)
	centralFiber.Root().SetName("Central")
	actor.UpdateSceneRegistry(centralFiber.Root())
	defer actor.RemoveSceneRegistry(centralFiber.Root())
	keys.Add("gate", 2001)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 1, serverConn, nil)
	session.SetEntity(ecs.NewEntity())

	resp, err := HandleC2GLoginGate(scene, session, &C2GLoginGate{
		RpcId:  2,
		Token:  "gate",
		GateId: 1,
	})
	if err != nil {
		t.Fatalf("HandleC2GLoginGate error = %v", err)
	}
	if resp == nil || resp.Error == 0 {
		t.Fatalf("response = %+v, want Home Map failure", resp)
	}
	if locationProxy.unlocks != 1 {
		t.Fatalf("account unlocks = %d, want 1 after failure", locationProxy.unlocks)
	}
}
