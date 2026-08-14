package network

import (
	"context"
	"net"
	"testing"
	"time"

	etlog "github.com/jerbe/et-go/internal/log"
)

func TestSessionAcceptTimeoutComponentClosesSession(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 1, serverConn, etlog.New("error"))
	component := NewSessionAcceptTimeoutComponent(session, 40*time.Millisecond)
	component.Awake()
	defer component.OnDestroy()

	if !waitUntil(300*time.Millisecond, session.IsClosed) {
		t.Fatal("session should close on accept timeout")
	}
}

func TestSessionAcceptTimeoutComponentDestroyStopsTimer(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 2, serverConn, etlog.New("error"))
	component := NewSessionAcceptTimeoutComponent(session, 80*time.Millisecond)
	component.Awake()

	component.OnDestroy()
	component.OnDestroy()

	time.Sleep(120 * time.Millisecond)
	if session.IsClosed() {
		t.Fatal("session should stay open when timeout component destroyed")
	}
	session.Close()
}

func TestSessionAcceptTimeoutDoesNotReopenAfterDestroy(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 5, serverConn, etlog.New("error"))
	component := NewSessionAcceptTimeoutComponent(session, 20*time.Millisecond)
	component.OnDestroy()
	component.Awake()

	time.Sleep(60 * time.Millisecond)
	if session.IsClosed() {
		t.Fatal("accept timeout component should not reopen after destroy")
	}
	session.Close()
}

func TestSessionIdleCheckerClosesIdleSession(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 3, serverConn, etlog.New("error"))
	component := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 40*time.Millisecond)
	component.Awake()
	defer component.OnDestroy()

	if !waitUntil(300*time.Millisecond, session.IsClosed) {
		t.Fatal("session should close on idle timeout")
	}
}

func TestSessionIdleCheckerTouchKeepsSessionAlive(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 4, serverConn, etlog.New("error"))
	component := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 50*time.Millisecond)
	component.Awake()
	defer component.OnDestroy()

	for index := 0; index < 5; index++ {
		time.Sleep(20 * time.Millisecond)
		component.Touch()
	}
	if session.IsClosed() {
		t.Fatal("session should stay alive while touched")
	}
}

func TestSessionIdleCheckerDoesNotReopenAfterDestroy(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 6, serverConn, etlog.New("error"))
	component := NewSessionIdleCheckerComponent(session, 10*time.Millisecond, 20*time.Millisecond)
	component.OnDestroy()
	component.Awake()

	time.Sleep(60 * time.Millisecond)
	if session.IsClosed() {
		t.Fatal("idle checker should not reopen after destroy")
	}
	session.Close()
}

func TestNetComponentDoesNotRestartAfterDestroy(t *testing.T) {
	component := NewNetComponent("kcp", "127.0.0.1:0")
	component.OnDestroy()

	if err := component.Start(); err != ErrNetComponentClosed {
		t.Fatalf("Start after destroy error = %v, want %v", err, ErrNetComponentClosed)
	}
}

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
