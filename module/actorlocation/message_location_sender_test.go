package actorlocation

import (
	"context"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
)

type sequenceRPCClient struct {
	payloads [][]byte
	errs     []error
	calls    int
}

func (m *sequenceRPCClient) Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	_ = ctx
	_ = actorID
	_ = msgID
	_ = payload

	index := m.calls
	m.calls++

	var err error
	if index < len(m.errs) {
		err = m.errs[index]
	}

	var response []byte
	if index < len(m.payloads) {
		response = append([]byte(nil), m.payloads[index]...)
	}
	return response, err
}

type flakyMessageSender struct {
	errs   []error
	actors []actor.ActorID
	calls  int
}

func (m *flakyMessageSender) Send(actorID actor.ActorID, msgID uint16, payload []byte) error {
	_ = msgID
	_ = payload
	m.actors = append(m.actors, actorID)
	index := m.calls
	m.calls++
	if index < len(m.errs) {
		return m.errs[index]
	}
	return nil
}

func (m *flakyMessageSender) Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error) {
	_ = ctx
	_ = actorID
	_ = msgID
	_ = payload
	return nil, nil
}

func TestMessageLocationSenderRefreshesLocationAfterSendFailure(t *testing.T) {
	locationActor := actor.ActorID{ProcessID: 1, FiberID: 10, InstanceID: 100}
	firstTarget := actor.ActorID{ProcessID: 1, FiberID: 11, InstanceID: 101}
	secondTarget := actor.ActorID{ProcessID: 1, FiberID: 12, InstanceID: 102}

	resp1, _ := marshalGetResponse(&ObjectGetResponse{ActorID: firstTarget})
	resp2, _ := marshalGetResponse(&ObjectGetResponse{ActorID: secondTarget})

	proxy := &LocationProxyComponent{}
	proxy.SetCaller(&sequenceRPCClient{payloads: [][]byte{resp1, resp2}})
	proxy.SetLocationActor(locationActor)

	senderClient := &flakyMessageSender{errs: []error{errors.New("actor moved"), nil}}
	sender := NewMessageLocationSender(LocationTypeUnit, proxy, senderClient)

	if err := sender.Send(100, 2001, []byte("payload")); err != nil {
		t.Fatalf("Send error = %v", err)
	}
	if senderClient.calls != 2 {
		t.Fatalf("send call count = %d, want 2", senderClient.calls)
	}
	if len(senderClient.actors) != 2 {
		t.Fatalf("actor send count = %d, want 2", len(senderClient.actors))
	}
	if senderClient.actors[0] != firstTarget {
		t.Fatalf("first actor = %+v, want %+v", senderClient.actors[0], firstTarget)
	}
	if senderClient.actors[1] != secondTarget {
		t.Fatalf("second actor = %+v, want %+v", senderClient.actors[1], secondTarget)
	}
}

func TestMessageLocationSenderRejectsMissingDependencies(t *testing.T) {
	sender := NewMessageLocationSender(LocationTypeUnit, nil, nil)
	if err := sender.Send(100, 2001, []byte("payload")); !errors.Is(err, ErrLocationProxyRequired) {
		t.Fatalf("Send error = %v, want %v", err, ErrLocationProxyRequired)
	}

	proxy := &LocationProxyComponent{}
	sender = NewMessageLocationSender(LocationTypeUnit, proxy, nil)
	if err := sender.Send(100, 2001, []byte("payload")); !errors.Is(err, ErrMessageSenderRequired) {
		t.Fatalf("Send error = %v, want %v", err, ErrMessageSenderRequired)
	}
}

func TestMessageLocationSenderRejectsNegativeKey(t *testing.T) {
	proxy := &LocationProxyComponent{}
	proxy.SetCaller(&mockRPCClient{})
	proxy.SetLocationActor(actor.ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3})
	sender := NewMessageLocationSender(LocationTypeUnit, proxy, &flakyMessageSender{})

	if err := sender.Send(-1, 1, []byte("payload")); err != ErrZeroLocationKey {
		t.Fatalf("negative key error = %v, want %v", err, ErrZeroLocationKey)
	}
}

func TestMessageLocationSenderComponentInitializesBeforeGet(t *testing.T) {
	component := &MessageLocationSenderComponent{}
	sender := component.Get(int(LocationTypeUnit))
	if sender == nil {
		t.Fatal("Get should initialize sender cache before creating sender")
	}
}

func TestMessageLocationSenderComponentDoesNotReopenAfterDestroy(t *testing.T) {
	component := &MessageLocationSenderComponent{}
	component.OnDestroy()
	if sender := component.Get(int(LocationTypeUnit)); sender != nil {
		t.Fatalf("Get after destroy = %+v, want nil", sender)
	}
}
