package actor

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
)

func TestMailBoxTypeValues(t *testing.T) {
	if MailBoxTypeUnOrderedMessage != 1001 {
		t.Fatalf("MailBoxTypeUnOrderedMessage = %d, want 1001", MailBoxTypeUnOrderedMessage)
	}
	if MailBoxTypeOrderedMessage != 3001 {
		t.Fatalf("MailBoxTypeOrderedMessage = %d, want 3001", MailBoxTypeOrderedMessage)
	}
	if MailBoxTypeGateSession != 9001 {
		t.Fatalf("MailBoxTypeGateSession = %d, want 9001", MailBoxTypeGateSession)
	}
}

func TestMailBoxAsComponentLifecycle(t *testing.T) {
	actorID := ActorID{ProcessID: 1, FiberID: 2, InstanceID: 3}
	mailbox := NewMailBox(actorID, MailBoxTypeUnOrderedMessage)

	entity := ecs.NewEntity()
	component, ok := any(mailbox).(ecs.Component)
	if !ok {
		t.Fatalf("MailBox should implement ecs.Component")
	}

	entity.AddComponent(component)

	c, ok := entity.GetComponent("MailBox")
	if !ok {
		t.Fatalf("entity should contain MailBox component")
	}
	if c != component {
		t.Fatalf("entity component mismatch")
	}

	messageID := uint16(101)
	expected := []byte("mailbox-ok")
	mailbox.RegisterHandler(messageID, func(gotActorID ActorID, gotMsgID uint16, payload []byte) ([]byte, error) {
		if gotActorID != actorID {
			t.Fatalf("actor id = %+v, want %+v", gotActorID, actorID)
		}
		if gotMsgID != messageID {
			t.Fatalf("msg id = %d, want %d", gotMsgID, messageID)
		}
		if !bytes.Equal(payload, []byte("request")) {
			t.Fatalf("payload = %q, want %q", payload, "request")
		}
		return expected, nil
	})

	response, err := mailbox.Dispatch(messageID, []byte("request"))
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}
	if !bytes.Equal(response, expected) {
		t.Fatalf("response = %q, want %q", response, expected)
	}

	entity.RemoveComponent("MailBox")
	_, exists := entity.GetComponent("MailBox")
	if exists {
		t.Fatalf("MailBox should be removed after RemoveComponent")
	}
}

func TestMailBoxDispatchHandlerNotFound(t *testing.T) {
	mailbox := NewMailBox(ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1}, MailBoxTypeUnOrderedMessage)

	_, err := mailbox.Dispatch(999, []byte("x"))
	if !errors.Is(err, ErrHandlerNotFound) {
		t.Fatalf("dispatch error = %v, want %v", err, ErrHandlerNotFound)
	}
}

func TestMailBoxDoesNotReopenAfterDestroy(t *testing.T) {
	mailbox := NewMailBox(ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1}, MailBoxTypeUnOrderedMessage)
	mailbox.OnDestroy()
	mailbox.RegisterHandler(1, func(ActorID, uint16, []byte) ([]byte, error) {
		return []byte("unexpected"), nil
	})

	if _, err := mailbox.Dispatch(1, nil); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("dispatch after destroy error = %v, want %v", err, ErrActorNotFound)
	}
}
