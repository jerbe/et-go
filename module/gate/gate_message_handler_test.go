package gate

import (
	"context"
	"net"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/actorlocation"
	actorlocationpb "github.com/jerbe/et-go/proto/actorlocationpb"
	gproto "google.golang.org/protobuf/proto"
)

type stubLocationMessageSender struct {
	payload []byte
}

func (s *stubLocationMessageSender) Send(_ actor.ActorID, _ uint16, _ []byte) error { return nil }
func (s *stubLocationMessageSender) Call(_ context.Context, _ actor.ActorID, msgID uint16, _ []byte) ([]byte, error) {
	if msgID == actorlocation.MsgObjectGetRequest {
		return gproto.Marshal(&actorlocationpb.ObjectGetResponse{
			ActorId: &actorlocationpb.ActorId{
				ProcessId:  1,
				FiberId:    2,
				InstanceId: 3,
			},
		})
	}
	return s.payload, nil
}

func addStubLocationSender(scene *ecs.Scene, payload []byte) {
	client := &stubLocationMessageSender{payload: payload}
	proxy := &actorlocation.LocationProxyComponent{}
	proxy.SetCaller(client)
	proxy.SetLocationActor(actor.ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1})
	component := &actorlocation.MessageLocationSenderComponent{}
	component.SetDependencies(proxy, client)
	scene.AddComponent(component)
}

type stubSessionPlayerComponent struct {
	ecs.BaseComponent
	playerID int64
	unitID   int64
}

func (c *stubSessionPlayerComponent) Type() string       { return "SessionPlayerComponent" }
func (c *stubSessionPlayerComponent) GetPlayerID() int64 { return c.playerID }
func (c *stubSessionPlayerComponent) GetUnitID() int64   { return c.unitID }

func TestGateMessageHandlerLocalAndLocation(t *testing.T) {
	RegisterSessionPacketHandler(8000, func(_ *ecs.Scene, _ *network.Session, packet *codec.Packet) (*codec.Packet, error) {
		return &codec.Packet{Type: codec.PacketTypeResponse, MsgID: packet.MsgID, RpcID: packet.RpcID, Payload: []byte("local")}, nil
	})
	defer RegisterSessionPacketHandler(8000, nil)
	RegisterLocationRequest(8001)

	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	addStubLocationSender(scene, []byte("remote"))
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 1, serverConn, nil)
	entity := ecs.NewEntity()
	entity.AddComponent(&stubSessionPlayerComponent{playerID: 1, unitID: 99})
	session.SetEntity(entity)

	handler := &GateMessageHandler{}
	resp, err := handler.Handle(scene, session, &codec.Packet{MsgID: 8000, RpcID: 1})
	if err != nil || string(resp.Payload) != "local" {
		t.Fatalf("local resp=%+v err=%v", resp, err)
	}

	resp, err = handler.Handle(scene, session, &codec.Packet{MsgID: 8001, RpcID: 2})
	if err != nil || string(resp.Payload) != "remote" {
		t.Fatalf("remote resp=%+v err=%v", resp, err)
	}
}

func TestGateMessageHandlerUsesResponseMessageID(t *testing.T) {
	const requestID uint16 = 8801
	const responseID uint16 = 8802
	RegisterLocationRequestWithResponse(requestID, responseID)
	defer RegisterLocationRequestWithResponse(requestID, 0)

	scene := ecs.NewScene(ecs.SceneTypeGate, 1, "gate")
	addStubLocationSender(scene, []byte("remote"))
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	session := network.NewSession(context.Background(), 2, serverConn, nil)
	entity := ecs.NewEntity()
	entity.AddComponent(&stubSessionPlayerComponent{playerID: 1, unitID: 99})
	session.SetEntity(entity)

	resp, err := (&GateMessageHandler{}).Handle(scene, session, &codec.Packet{
		MsgID: requestID,
		RpcID: 7,
	})
	if err != nil {
		t.Fatalf("Handle error = %v", err)
	}
	if resp == nil || resp.MsgID != responseID || resp.RpcID != 7 {
		t.Fatalf("response = %+v, want msg id %d and rpc id 7", resp, responseID)
	}
}

func TestGateMessageHandlerRejectsNilScene(t *testing.T) {
	_, err := (&GateMessageHandler{}).Handle(nil, nil, &codec.Packet{MsgID: 1})
	if err != ErrSceneMissing {
		t.Fatalf("Handle nil scene error = %v, want %v", err, ErrSceneMissing)
	}
}
