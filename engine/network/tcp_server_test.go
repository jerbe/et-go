package network

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/network/codec"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestTCPServerConnectMessageAndStop(t *testing.T) {
	addr := pickFreeAddr(t)
	server := NewTCPServer(addr, etlog.New("error"))

	connectCh := make(chan *Session, 1)
	messageCh := make(chan *codec.Packet, 1)

	server.OnConnect(func(session *Session) {
		select {
		case connectCh <- session:
		default:
		}
	})
	server.OnMessage(func(_ *Session, packet *codec.Packet) {
		select {
		case messageCh <- packet:
		default:
		}
	})

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start tcp server error: %v", err)
	}
	defer server.Stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial tcp server error: %v", err)
	}
	defer conn.Close()

	var session *Session
	select {
	case session = <-connectCh:
	case <-time.After(time.Second):
		t.Fatal("wait connect callback timeout")
	}
	if session == nil {
		t.Fatal("session should not be nil")
	}

	packet := &codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   5001,
		Payload: []byte("ping"),
	}
	encoded, err := codec.Encode(packet)
	if err != nil {
		t.Fatalf("encode packet error: %v", err)
	}
	if _, err := conn.Write(encoded); err != nil {
		t.Fatalf("write packet error: %v", err)
	}

	select {
	case got := <-messageCh:
		if got.MsgID != packet.MsgID {
			t.Fatalf("MsgID = %d, want %d", got.MsgID, packet.MsgID)
		}
		if string(got.Payload) != string(packet.Payload) {
			t.Fatalf("payload = %q, want %q", got.Payload, packet.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("wait message callback timeout")
	}

	server.Stop()
}

func TestTCPServerRejectsNilContext(t *testing.T) {
	server := NewTCPServer("127.0.0.1:0", etlog.New("error"))
	if err := server.Start(nil); err != ErrContextRequired {
		t.Fatalf("Start(nil) error = %v, want %v", err, ErrContextRequired)
	}
}

func TestTCPServerCanRestartAfterStop(t *testing.T) {
	addr := pickFreeAddr(t)
	server := NewTCPServer(addr, etlog.New("error"))

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("first Start error: %v", err)
	}
	server.Stop()

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("second Start error: %v", err)
	}
	defer server.Stop()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial restarted server error: %v", err)
	}
	defer conn.Close()

	if !waitUntilTCP(time.Second, func() bool {
		server.mu.RLock()
		defer server.mu.RUnlock()
		return len(server.sessions) == 1
	}) {
		t.Fatal("restarted server should accept a connection")
	}
}

func TestTCPServerRemoveSessionCallback(t *testing.T) {
	addr := pickFreeAddr(t)
	server := NewTCPServer(addr, etlog.New("error"))
	defer server.Stop()

	var connectedID atomic.Int64
	disconnectCh := make(chan int64, 1)

	server.OnConnect(func(session *Session) {
		connectedID.Store(session.ID())
	})
	server.OnDisconnect(func(session *Session) {
		select {
		case disconnectCh <- session.ID():
		default:
		}
	})

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start tcp server error: %v", err)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial tcp server error: %v", err)
	}
	defer conn.Close()

	var id int64
	if !waitUntilTCP(500*time.Millisecond, func() bool {
		id = connectedID.Load()
		return id > 0
	}) {
		t.Fatal("session id should be available after connect")
	}

	session, ok := server.GetSession(id)
	if !ok || session == nil {
		t.Fatal("session should exist before remove")
	}

	server.RemoveSession(id)

	select {
	case got := <-disconnectCh:
		if got != id {
			t.Fatalf("disconnect session id = %d, want %d", got, id)
		}
	case <-time.After(time.Second):
		t.Fatal("wait disconnect callback timeout")
	}

	session.Close()
}

func TestKCPConsts(t *testing.T) {
	if KcpSYN != 1 || KcpACK != 2 || KcpFIN != 3 || KcpMSG != 4 {
		t.Fatal("kcp base constants mismatch")
	}
	if KcpRouterReconnSYN != 5 || KcpRouterReconnACK != 6 || KcpRouterSYN != 7 || KcpRouterACK != 8 {
		t.Fatal("kcp router constants mismatch")
	}
	if KcpConnectTimeout != 20000 {
		t.Fatalf("KcpConnectTimeout = %d, want 20000", KcpConnectTimeout)
	}
	if KcpInnerConfig.MTU != 1400 || KcpOuterConfig.MTU != 470 {
		t.Fatal("kcp config mtu mismatch")
	}
}

func pickFreeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free addr error: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func waitUntilTCP(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return condition()
}
