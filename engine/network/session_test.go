package network

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/network/codec"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestSessionReadWriteLoop(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 10, serverConn, etlog.New("error"))
	received := make(chan *codec.Packet, 1)

	session.StartWriteLoop()
	session.StartReadLoop(func(_ *Session, packet *codec.Packet) {
		select {
		case received <- packet:
		default:
		}
	})

	outbound := &codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   101,
		Payload: []byte("server->client"),
	}
	session.Send(outbound)

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	gotOutbound, err := codec.Decode(clientConn)
	if err != nil {
		t.Fatalf("decode outbound packet error: %v", err)
	}
	if gotOutbound.MsgID != outbound.MsgID {
		t.Fatalf("outbound MsgID = %d, want %d", gotOutbound.MsgID, outbound.MsgID)
	}
	if string(gotOutbound.Payload) != string(outbound.Payload) {
		t.Fatalf("outbound payload = %q, want %q", gotOutbound.Payload, outbound.Payload)
	}

	inbound := &codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   102,
		Payload: []byte("client->server"),
	}
	inboundData, err := codec.Encode(inbound)
	if err != nil {
		t.Fatalf("encode inbound packet error: %v", err)
	}
	if _, err := clientConn.Write(inboundData); err != nil {
		t.Fatalf("write inbound packet error: %v", err)
	}

	select {
	case gotInbound := <-received:
		if gotInbound.MsgID != inbound.MsgID {
			t.Fatalf("inbound MsgID = %d, want %d", gotInbound.MsgID, inbound.MsgID)
		}
		if string(gotInbound.Payload) != string(inbound.Payload) {
			t.Fatalf("inbound payload = %q, want %q", gotInbound.Payload, inbound.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("wait inbound packet timeout")
	}

	session.Close()
}

func TestSessionCloseIdempotent(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 11, serverConn, etlog.New("error"))

	var waitGroup sync.WaitGroup
	for index := 0; index < 10; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			session.Close()
		}()
	}
	waitGroup.Wait()

	if !session.IsClosed() {
		t.Fatal("session should be closed")
	}
}

func TestSessionSendRejectsCloseRaceBoundary(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 14, serverConn, etlog.New("error"))
	session.Close()

	if err := session.Send(&codec.Packet{
		Type:  codec.PacketTypeMessage,
		MsgID: 1,
	}); err != ErrSessionClosed {
		t.Fatalf("Send after Close error = %v, want %v", err, ErrSessionClosed)
	}
}

func TestSessionSetOnCloseAfterCloseInvokesCallback(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 13, serverConn, etlog.New("error"))
	session.Close()

	called := make(chan struct{}, 1)
	session.SetOnClose(func() {
		called <- struct{}{}
	})

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("late onClose callback was not invoked")
	}
}

func TestSessionRPCOptionalAPI(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 12, serverConn, etlog.New("error"))
	session.StartWriteLoop()
	session.StartReadLoop(func(_ *Session, _ *codec.Packet) {})
	defer session.Close()

	go func() {
		request, err := codec.Decode(clientConn)
		if err != nil || request == nil {
			return
		}

		response := &codec.Packet{
			Type:    codec.PacketTypeResponse,
			MsgID:   request.MsgID,
			RpcID:   request.RpcID,
			Payload: []byte("rpc-ok"),
		}
		encoded, err := codec.Encode(response)
		if err != nil {
			return
		}
		_, _ = clientConn.Write(encoded)
	}()

	response, err := session.Call(context.Background(), &codec.Packet{
		MsgID:   200,
		Payload: []byte("rpc-req"),
	})
	if err != nil {
		t.Fatalf("session call error: %v", err)
	}
	if string(response.Payload) != "rpc-ok" {
		t.Fatalf("rpc payload = %q, want %q", response.Payload, "rpc-ok")
	}
}

func TestSessionCallReturnsOnClose(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 15, serverConn, etlog.New("error"))
	result := make(chan error, 1)
	go func() {
		_, err := session.Call(context.Background(), &codec.Packet{MsgID: 201})
		result <- err
	}()

	if !waitUntil(time.Second, func() bool {
		session.callbackMu.Lock()
		defer session.callbackMu.Unlock()
		return len(session.callbacks) == 1
	}) {
		t.Fatal("Session.Call did not register callback")
	}
	session.Close()
	select {
	case err := <-result:
		if err != ErrSessionClosed {
			t.Fatalf("Session.Call after close = %v, want %v", err, ErrSessionClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("Session.Call did not return after session close")
	}
}

func TestSessionCallHonorsContextCancellation(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	session := NewSession(context.Background(), 16, serverConn, etlog.New("error"))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := session.Call(ctx, &codec.Packet{MsgID: 202})
	if err == nil || err != context.DeadlineExceeded {
		t.Fatalf("Session.Call cancellation error = %v, want %v", err, context.DeadlineExceeded)
	}
	session.Close()
}
