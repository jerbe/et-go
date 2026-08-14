package login

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
)

type stubMessageSender struct {
	ecs.BaseComponent
	payload []byte
	err     error
}

func (s *stubMessageSender) Type() string { return "MessageSender" }
func (s *stubMessageSender) Call(_ context.Context, _ actor.ActorID, _ uint16, _ []byte) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.payload, nil
}

func newTestSession(t *testing.T) (*network.Session, net.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	})
	return network.NewSession(context.Background(), 1, serverConn, nil), clientConn
}

func TestHandleC2RLogin(t *testing.T) {
	sceneFiber := fiber.New(context.Background(), ecs.SceneTypeRealm, 1, 1)
	scene := sceneFiber.Root()
	scene.SetID(9003)
	scene.SetName("Realm")
	locker := &coroutinelock.CoroutineLockComponent{}
	scene.AddComponent(locker)
	scene.AddComponent(&stubLocationProxy{})
	registry := &GateRegistryComponent{}
	scene.AddComponent(registry)
	registry.SetGates(1, []GateEndpoint{{
		GateId:  11,
		Address: "127.0.0.1:10000",
		ActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
	}})
	payload, _ := marshalG2RGateAssign(&G2RGateAssign{GateId: 11, Token: "gate-token"})
	scene.AddComponent(&stubMessageSender{payload: payload})

	oldDelay := realmCloseDelay
	realmCloseDelay = 20 * time.Millisecond
	defer func() { realmCloseDelay = oldDelay }()

	session, _ := newTestSession(t)
	accessToken, err := GenerateAccessToken(1001)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	resp, err := HandleC2RLogin(scene, session, &C2RLogin{
		RpcId:       1,
		AccessToken: accessToken,
		ZoneId:      1,
	})
	if err != nil {
		t.Fatalf("HandleC2RLogin err = %v", err)
	}
	if resp.Token != "gate-token" || resp.GateId != 11 || resp.Address == "" {
		t.Fatalf("unexpected resp: %+v", resp)
	}

	time.Sleep(60 * time.Millisecond)
	if !session.IsClosed() {
		t.Fatal("session should be closed")
	}
}

func TestHandleC2RLoginErrors(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeRealm, 1, "realm")
	scene.AddComponent(&GateRegistryComponent{})
	scene.AddComponent(&stubMessageSender{})

	resp, err := HandleC2RLogin(scene, nil, &C2RLogin{RpcId: 2, AccessToken: "bad"})
	if err != nil {
		t.Fatalf("HandleC2RLogin err = %v", err)
	}
	if resp.Error != ERR_TokenInvalidError {
		t.Fatalf("resp.Error = %d", resp.Error)
	}
	if resp.Message != "token invalid" {
		t.Fatalf("resp.Message = %q", resp.Message)
	}

	oldDelay := realmCloseDelay
	realmCloseDelay = 20 * time.Millisecond
	defer func() { realmCloseDelay = oldDelay }()
	session, _ := newTestSession(t)
	oldNowFunc := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Now().Add(-tokenExpireDuration - time.Hour) }
	expiredToken, err := GenerateAccessToken(88)
	tokenNowFunc = oldNowFunc
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	resp, err = HandleC2RLogin(scene, session, &C2RLogin{RpcId: 9, AccessToken: expiredToken})
	if err != nil {
		t.Fatalf("Handle expired token err = %v", err)
	}
	if resp.Error != ERR_TokenExpiredError || resp.Message != "token invalid" {
		t.Fatalf("unexpected expired resp %+v", resp)
	}
	time.Sleep(60 * time.Millisecond)
	if !session.IsClosed() {
		t.Fatal("expired token session should be closed")
	}

	scene = ecs.NewScene(ecs.SceneTypeRealm, 1, "realm")
	registry := &GateRegistryComponent{}
	scene.AddComponent(registry)
	registry.SetGates(1, []GateEndpoint{{
		GateId:  11,
		Address: "127.0.0.1:10000",
		ActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
	}})
	scene.AddComponent(&stubMessageSender{err: errors.New("rpc failed")})
	token, _ := GenerateAccessToken(99)
	if _, err := HandleC2RLogin(scene, nil, &C2RLogin{RpcId: 3, AccessToken: token, ZoneId: 1}); err == nil {
		t.Fatal("expected rpc error")
	}
}

func TestHandleC2RLoginRejectsEmptyGateToken(t *testing.T) {
	sceneFiber := fiber.New(context.Background(), ecs.SceneTypeRealm, 1, 1)
	scene := sceneFiber.Root()
	t.Cleanup(func() {
		sceneFiber.RequestStop()
		sceneFiber.Wait(2 * time.Second)
	})
	scene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	scene.AddComponent(&stubLocationProxy{})
	registry := &GateRegistryComponent{}
	scene.AddComponent(registry)
	registry.SetGates(1, []GateEndpoint{{
		GateId:  11,
		Address: "127.0.0.1:10000",
		ActorID: actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3},
	}})
	payload, err := marshalG2RGateAssign(&G2RGateAssign{GateId: 11})
	if err != nil {
		t.Fatalf("marshal gate assignment error = %v", err)
	}
	scene.AddComponent(&stubMessageSender{payload: payload})
	token, err := GenerateAccessToken(1001)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}

	if _, err := HandleC2RLogin(scene, nil, &C2RLogin{AccessToken: token, ZoneId: 1}); !errors.Is(err, ErrGateAssignmentInvalid) {
		t.Fatalf("empty gate token error = %v, want %v", err, ErrGateAssignmentInvalid)
	}
}
