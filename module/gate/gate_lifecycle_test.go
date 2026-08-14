package gate

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
)

func TestGateSessionLifecycle(t *testing.T) {
	t.Run("accept-timeout", func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer clientConn.Close()
		session := network.NewSession(context.Background(), 1, serverConn, nil)
		entity := ecs.NewEntity()
		session.SetEntity(entity)
		entity.AddComponent(NewSessionAcceptTimeoutComponent(session, 30*time.Millisecond))
		if !waitUntil(200*time.Millisecond, session.IsClosed) {
			t.Fatal("session should close before auth")
		}
	})

	t.Run("idle-timeout", func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer clientConn.Close()
		session := network.NewSession(context.Background(), 2, serverConn, nil)
		entity := ecs.NewEntity()
		session.SetEntity(entity)
		acceptTimeout := NewSessionAcceptTimeoutComponent(session, 100*time.Millisecond)
		entity.AddComponent(acceptTimeout)
		entity.AddComponent(NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 40*time.Millisecond))
		entity.RemoveComponent(acceptTimeout.Type())
		if !waitUntil(300*time.Millisecond, session.IsClosed) {
			t.Fatal("session should close after idle timeout")
		}
	})

	t.Run("keepalive", func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer clientConn.Close()
		session := network.NewSession(context.Background(), 3, serverConn, nil)
		entity := ecs.NewEntity()
		session.SetEntity(entity)
		acceptTimeout := NewSessionAcceptTimeoutComponent(session, 100*time.Millisecond)
		entity.AddComponent(acceptTimeout)
		idle := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 50*time.Millisecond)
		entity.AddComponent(idle)
		entity.RemoveComponent(acceptTimeout.Type())
		for i := 0; i < 5; i++ {
			time.Sleep(20 * time.Millisecond)
			session.TouchRecv()
			idle.Touch()
		}
		if session.IsClosed() {
			t.Fatal("session should stay alive with keepalive")
		}
		session.Close()
	})
}
