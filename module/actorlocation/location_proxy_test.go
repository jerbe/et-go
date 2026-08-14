package actorlocation

import (
	"context"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
)

type mockRPCClient struct {
	lastMsgID   uint16
	lastActor   actor.ActorID
	lastPayload []byte
	response    []byte
	err         error
}

func (m *mockRPCClient) Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	_ = ctx
	m.lastMsgID = msgID
	m.lastActor = actorID
	m.lastPayload = append([]byte(nil), payload...)
	return m.response, m.err
}

func TestLocationProxyAddGet(t *testing.T) {
	client := &mockRPCClient{}
	proxy := &LocationProxyComponent{}
	proxy.SetCaller(client)
	locationActor := actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}
	proxy.SetLocationActor(locationActor)

	target := actor.ActorID{ProcessID: 4, FiberID: 5, InstanceID: 6}
	addResponse, err := marshalAddResponse(&ObjectRemoveResponse{RpcID: 1})
	if err != nil {
		t.Fatalf("marshal add response error = %v", err)
	}
	client.response = addResponse
	if err := proxy.Add(int(LocationTypeUnit), 100, target); err != nil {
		t.Fatalf("Add error = %v", err)
	}
	if client.lastMsgID != MsgObjectAddRequest || client.lastActor != locationActor {
		t.Fatal("proxy add should call location actor with add message")
	}

	respPayload, _ := marshalGetResponse(&ObjectGetResponse{ActorID: target})
	client.response = respPayload
	got, err := proxy.Get(int(LocationTypeUnit), 100)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if got != target {
		t.Fatalf("Get actor = %+v, want %+v", got, target)
	}

}

func TestLocationProxyRejectsNegativeKey(t *testing.T) {
	proxy := &LocationProxyComponent{}
	if _, err := proxy.GetContext(context.Background(), int(LocationTypeUnit), -1); err != ErrZeroLocationKey {
		t.Fatalf("negative key error = %v, want %v", err, ErrZeroLocationKey)
	}
}

func TestMarshalRequestRejectsMismatchedMessageType(t *testing.T) {
	_, err := marshalRequest(MsgObjectAddRequest, &ObjectGetRequest{})
	if !errors.Is(err, ErrMessageTypeInvalid) {
		t.Fatalf("mismatched request error = %v, want %v", err, ErrMessageTypeInvalid)
	}
}
