package gate

import (
	"context"
	"net"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
)

func TestGateSessionDispatcher(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	session := network.NewSession(context.Background(), 1, serverConn, nil)
	entity := ecs.NewEntity()
	entity.AddComponent(&GateSessionComponent{Session: session})
	session.SetEntity(entity)
	session.StartWriteLoop()

	dispatcher := &GateSessionDispatcher{}
	if _, err := dispatcher.Handle(entity, actor.ActorID{}, 100, []byte("hello")); err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	packet, err := codec.Decode(clientConn)
	if err != nil {
		t.Fatalf("Decode err = %v", err)
	}
	if packet.MsgID != 100 || string(packet.Payload) != "hello" {
		t.Fatalf("packet = %+v", packet)
	}
}
