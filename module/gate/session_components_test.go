package gate

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
)

func waitUntil(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return condition()
}

func TestSessionAcceptTimeoutComponent(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := network.NewSession(context.Background(), 1, serverConn, nil)
	entity := ecs.NewEntity()
	session.SetEntity(entity)
	component := NewSessionAcceptTimeoutComponent(session, 40*time.Millisecond)
	entity.AddComponent(component)
	entity.RemoveComponent(component.Type())
	time.Sleep(80 * time.Millisecond)
	if session.IsClosed() {
		t.Fatal("session should remain open after auth")
	}
	session.Close()
}

func TestSessionIdleCheckerComponent(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := network.NewSession(context.Background(), 2, serverConn, nil)
	component := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 40*time.Millisecond)
	component.Awake()
	defer component.OnDestroy()

	if !waitUntil(300*time.Millisecond, session.IsClosed) {
		t.Fatal("session should close on idle timeout")
	}
}

func TestSessionIdleCheckerTouchKeepsAlive(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := network.NewSession(context.Background(), 3, serverConn, nil)
	component := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 50*time.Millisecond)
	component.Awake()
	defer component.OnDestroy()

	for i := 0; i < 5; i++ {
		time.Sleep(20 * time.Millisecond)
		component.Touch()
	}
	if session.IsClosed() {
		t.Fatal("session should stay alive while touched")
	}
	session.Close()
}
