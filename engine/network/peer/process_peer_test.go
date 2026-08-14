package peer

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
)

const processPeerTestMessageID uint16 = 4901

func TestProcessPeerHandshakeAndOuterRPC(t *testing.T) {
	portA := reserveUDPPort(t)
	portB := reserveUDPPort(t)
	addressA := "127.0.0.1:" + portA
	addressB := "127.0.0.1:" + portB

	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	target := manager.Create(ecs.SceneTypeMain, 1, 2, func(f *fiber.Fiber, msg fiber.Message) {
		_, _ = actor.DispatchFiberMessage(f.Root(), msg)
	})
	if target == nil {
		manager.StopAll()
		world.Shutdown()
		t.Fatal("target fiber create failed")
	}
	mailbox := actor.NewMailBox(actor.SceneActorID(target.Root()), actor.MailBoxTypeUnOrderedMessage)
	target.Root().AddComponent(mailbox)
	mailbox.RegisterHandler(processPeerTestMessageID, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		return append([]byte("reply:"), payload...), nil
	})
	senderA := actor.NewProcessOuterSender(actor.NewRpcManager(actor.WithRPCTimeout(2 * time.Second)))
	senderB := actor.NewProcessOuterSender(actor.NewRpcManager(actor.WithRPCTimeout(2 * time.Second)))
	senderB.SetFiberManager(manager)

	peerA := NewProcessPeerComponent(1, addressA, senderA, []PeerEndpoint{
		{ProcessID: 2, Address: addressB, Secret: "shared-secret"},
	})
	peerB := NewProcessPeerComponent(2, addressB, senderB, []PeerEndpoint{
		{ProcessID: 1, Address: addressA, Secret: "shared-secret"},
	})
	peerA.HandshakeTimeout = 500 * time.Millisecond
	peerB.HandshakeTimeout = 500 * time.Millisecond
	peerA.ReconnectInterval = 20 * time.Millisecond
	peerB.ReconnectInterval = 20 * time.Millisecond
	peerA.SetContext(context.Background())
	peerB.SetContext(context.Background())
	if err := peerA.Start(); err != nil {
		manager.StopAll()
		world.Shutdown()
		t.Fatalf("peer A start error = %v", err)
	}
	if err := peerB.Start(); err != nil {
		peerA.OnDestroy()
		manager.StopAll()
		world.Shutdown()
		t.Fatalf("peer B start error = %v", err)
	}

	stopPump := make(chan struct{})
	pumpStopped := make(chan struct{})
	go func() {
		defer close(pumpStopped)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPump:
				return
			case <-ticker.C:
				peerA.Update()
				peerB.Update()
			}
		}
	}()
	defer func() {
		close(stopPump)
		<-pumpStopped
		peerA.OnDestroy()
		peerB.OnDestroy()
		manager.StopAll()
		world.Shutdown()
	}()

	waitForProcessPeer(t, 5*time.Second, func() bool {
		return hasProcess(peerA.ConnectedProcesses(), 2) &&
			hasProcess(peerB.ConnectedProcesses(), 1)
	})

	actorID := actor.ActorID{
		ProcessID:  2,
		FiberID:    target.ID(),
		InstanceID: target.Root().InstanceID(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	payload, err := senderA.Call(ctx, actorID, processPeerTestMessageID, []byte("ping"))
	if err != nil {
		t.Fatalf("outer RPC error = %v", err)
	}
	if string(payload) != "reply:ping" {
		t.Fatalf("outer RPC payload = %q, want %q", payload, "reply:ping")
	}
}

func TestProcessPeerReconnectReplacesSession(t *testing.T) {
	portA := reserveUDPPort(t)
	portB := reserveUDPPort(t)
	peerA := NewProcessPeerComponent(1, "127.0.0.1:"+portA, actor.NewProcessOuterSender(actor.NewRpcManager()), []PeerEndpoint{
		{ProcessID: 2, Address: "127.0.0.1:" + portB, Secret: "shared-secret"},
	})
	peerB := NewProcessPeerComponent(2, "127.0.0.1:"+portB, actor.NewProcessOuterSender(actor.NewRpcManager()), []PeerEndpoint{
		{ProcessID: 1, Address: "127.0.0.1:" + portA, Secret: "shared-secret"},
	})
	peerA.ReconnectInterval = 10 * time.Millisecond
	peerB.ReconnectInterval = 10 * time.Millisecond
	peerA.HandshakeTimeout = 500 * time.Millisecond
	peerB.HandshakeTimeout = 500 * time.Millisecond
	peerA.SetContext(context.Background())
	peerB.SetContext(context.Background())
	if err := peerA.Start(); err != nil {
		t.Fatalf("peer A start error = %v", err)
	}
	if err := peerB.Start(); err != nil {
		peerA.OnDestroy()
		t.Fatalf("peer B start error = %v", err)
	}

	stopPump := make(chan struct{})
	pumpStopped := make(chan struct{})
	go func() {
		defer close(pumpStopped)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopPump:
				return
			case <-ticker.C:
				peerA.Update()
				peerB.Update()
			}
		}
	}()
	defer func() {
		close(stopPump)
		<-pumpStopped
		peerA.OnDestroy()
		peerB.OnDestroy()
	}()

	waitForProcessPeer(t, 5*time.Second, func() bool {
		return hasProcess(peerA.ConnectedProcesses(), 2)
	})
	peerA.mu.Lock()
	oldSession := peerA.connected[2]
	peerA.mu.Unlock()
	if oldSession == nil {
		t.Fatal("connected peer session is nil")
	}
	oldSession.Close()

	waitForProcessPeer(t, 5*time.Second, func() bool {
		peerA.mu.Lock()
		current := peerA.connected[2]
		peerA.mu.Unlock()
		return current != nil && current != oldSession && !current.IsClosed()
	})
}

func TestPeerHandshakeRejectsWrongSecret(t *testing.T) {
	handshake, err := marshalPeerHandshake(peerHandshake{
		Version:         processPeerProtocolVersion,
		SenderProcessID: 1,
		TargetProcessID: 2,
		Nonce:           "nonce",
	}, "secret-a")
	if err != nil {
		t.Fatalf("marshal handshake error = %v", err)
	}
	decoded, err := unmarshalPeerHandshake(handshake)
	if err != nil {
		t.Fatalf("unmarshal handshake error = %v", err)
	}
	if verifyPeerHandshake(decoded, "secret-b") {
		t.Fatal("handshake with wrong secret should be rejected")
	}
	if !verifyPeerHandshake(decoded, "secret-a") {
		t.Fatal("handshake with matching secret should be accepted")
	}
}

func reserveUDPPort(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("reserve UDP port error = %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatalf("release UDP port error = %v", err)
	}
	return strconv.Itoa(port)
}

func waitForProcessPeer(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("process peer condition timeout")
}

func hasProcess(processes []int, target int) bool {
	for _, processID := range processes {
		if processID == target {
			return true
		}
	}
	return false
}
