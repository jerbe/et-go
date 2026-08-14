package actor

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestProcessInnerSenderSend(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	msgCh := make(chan fiber.Message, 1)
	target := manager.Create(ecs.SceneTypeMain, 1, 1, func(_ *fiber.Fiber, msg fiber.Message) {
		select {
		case msgCh <- msg:
		default:
		}
	})
	if target == nil {
		t.Fatal("target fiber create failed")
	}

	inner := NewProcessInnerSender(1, manager, NewRpcManager())
	inner.Awake()

	actorID := ActorID{
		ProcessID:  1,
		FiberID:    target.ID(),
		InstanceID: 10086,
	}
	payload := []byte("inner-send")
	const msgID = uint16(3001)

	if err := inner.Send(actorID, msgID, payload); err != nil {
		t.Fatalf("inner send error = %v", err)
	}

	select {
	case got := <-msgCh:
		if got.To != actorID.InstanceID {
			t.Fatalf("msg.To = %d, want %d", got.To, actorID.InstanceID)
		}
		if got.MsgID != msgID {
			t.Fatalf("msg.MsgID = %d, want %d", got.MsgID, msgID)
		}
		if !bytes.Equal(got.Payload, payload) {
			t.Fatalf("msg.Payload = %q, want %q", got.Payload, payload)
		}
	case <-time.After(time.Second):
		t.Fatal("wait target message timeout")
	}

	err := inner.Send(ActorID{ProcessID: 1, FiberID: 999999, InstanceID: 1}, msgID, payload)
	if !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("send missing actor err = %v, want %v", err, ErrActorNotFound)
	}
}

func TestSendersRejectNilDependencies(t *testing.T) {
	var inner *ProcessInnerSender
	if err := inner.Send(ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1}, 1, nil); !errors.Is(err, ErrFiberManagerMissing) {
		t.Fatalf("nil inner sender error = %v, want %v", err, ErrFiberManagerMissing)
	}

	var sender *MessageSender
	if err := sender.Send(ActorID{ProcessID: 1, FiberID: 1, InstanceID: 1}, 1, nil); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("nil message sender error = %v, want %v", err, ErrActorNotFound)
	}

	var mailbox *MailBox
	if _, err := mailbox.Dispatch(1, nil); !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("nil mailbox error = %v, want %v", err, ErrActorNotFound)
	}
}

func TestProcessInnerSenderCallDispatchesOnFiberQueue(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	const msgID = uint16(3002)
	var target *fiber.Fiber
	target = manager.Create(ecs.SceneTypeMain, 1, 1, func(_ *fiber.Fiber, msg fiber.Message) {
		payload, err := DispatchFiberMessage(target.Root(), msg)
		if msg.Reply != nil {
			msg.Reply <- fiber.MessageResponse{Payload: payload, Err: err}
		}
	})
	if target == nil {
		t.Fatal("target fiber create failed")
	}
	target.Root().AddComponent(NewMailBox(ActorID{
		ProcessID:  1,
		FiberID:    target.ID(),
		InstanceID: target.Root().InstanceID(),
	}, MailBoxTypeUnOrderedMessage))
	mailboxComponent, ok := target.Root().GetComponent("MailBox")
	if !ok {
		t.Fatal("target mailbox missing")
	}
	mailbox := mailboxComponent.(*MailBox)
	mailbox.RegisterHandler(msgID, func(_ ActorID, _ uint16, payload []byte) ([]byte, error) {
		return append([]byte("reply:"), payload...), nil
	})

	inner := NewProcessInnerSender(1, manager, NewRpcManager(WithRPCTimeout(time.Second)))
	inner.Awake()
	actorID := ActorID{ProcessID: 1, FiberID: target.ID(), InstanceID: target.Root().InstanceID()}
	got, err := inner.Call(context.Background(), actorID, msgID, []byte("queued"))
	if err != nil {
		t.Fatalf("inner Call error = %v", err)
	}
	if string(got) != "reply:queued" {
		t.Fatalf("inner Call payload = %q, want reply:queued", got)
	}
}

func TestProcessOuterSenderSendViaSession(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	logger := etlog.New("error")
	session := network.NewSession(context.Background(), 1, serverConn, logger)
	session.StartWriteLoop()
	defer session.Close()

	outer := &ProcessOuterSender{}
	outer = NewProcessOuterSender(NewRpcManager())
	outer.Awake()
	outer.AddSession(2, session)

	payload := []byte("outer-send")
	const msgID = uint16(4001)
	actorID := ActorID{ProcessID: 2, FiberID: 99, InstanceID: 100}

	if err := outer.Send(actorID, msgID, payload); err != nil {
		t.Fatalf("outer send error = %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	packet, err := codec.Decode(clientConn)
	if err != nil {
		t.Fatalf("decode packet error = %v", err)
	}
	if packet == nil {
		t.Fatal("packet should not be nil")
	}
	if packet.MsgID == 0 {
		t.Fatal("packet msg id should not be zero")
	}
	if len(packet.Payload) == 0 {
		t.Fatal("packet payload should not be empty")
	}
	if !bytes.Contains(packet.Payload, payload) && !bytes.Equal(packet.Payload, payload) {
		t.Fatalf("packet payload should contain original payload, got=%q want=%q", packet.Payload, payload)
	}

	outer.RemoveSession(2)

	err = outer.Send(actorID, msgID, payload)
	if !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("outer send without session err = %v, want %v", err, ErrActorNotFound)
	}
}

type recordingPacketSession struct {
	sent chan *codec.Packet
}

func (s *recordingPacketSession) Send(packet *codec.Packet) error {
	select {
	case s.sent <- packet:
	default:
	}
	return nil
}

func TestProcessOuterSenderFailsPendingCallOnSessionRemoval(t *testing.T) {
	outer := NewProcessOuterSender(NewRpcManager(WithRPCTimeout(time.Second)))
	session := &recordingPacketSession{sent: make(chan *codec.Packet, 1)}
	outer.AddSession(2, session)

	result := make(chan error, 1)
	go func() {
		_, err := outer.Call(context.Background(), ActorID{
			ProcessID:  2,
			FiberID:    9,
			InstanceID: 10,
		}, 4201, []byte("pending"))
		result <- err
	}()

	select {
	case packet := <-session.sent:
		if packet == nil || packet.Type != codec.PacketTypeRequest {
			t.Fatalf("unexpected request packet: %+v", packet)
		}
	case <-time.After(time.Second):
		t.Fatal("request was not sent")
	}

	outer.RemoveSession(2)

	select {
	case err := <-result:
		if !errors.Is(err, ErrProcessSessionClosed) {
			t.Fatalf("Call error = %v, want %v", err, ErrProcessSessionClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Call was not failed after session removal")
	}
}

func TestProcessOuterSenderRemovesPendingCallAfterRPCManagerTimeout(t *testing.T) {
	outer := NewProcessOuterSender(NewRpcManager(WithRPCTimeout(20 * time.Millisecond)))
	session := &recordingPacketSession{sent: make(chan *codec.Packet, 1)}
	outer.AddSession(2, session)

	_, err := outer.Call(context.Background(), ActorID{
		ProcessID:  2,
		FiberID:    9,
		InstanceID: 10,
	}, 4202, []byte("timeout"))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Call error = %v, want %v", err, ErrTimeout)
	}

	outer.mu.RLock()
	pendingCount := len(outer.pending)
	outer.mu.RUnlock()
	if pendingCount != 0 {
		t.Fatalf("pending RPC count = %d after timeout, want 0", pendingCount)
	}
}

func TestProcessOuterSenderReceivesThroughFiberQueue(t *testing.T) {
	world := ecs.NewWorld()
	defer world.Shutdown()

	manager := fiber.NewManager(context.Background(), world, etlog.New("error"))
	defer manager.StopAll()

	var target *fiber.Fiber
	target = manager.Create(ecs.SceneTypeMain, 1, 1, func(_ *fiber.Fiber, msg fiber.Message) {
		_, _ = DispatchFiberMessage(target.Root(), msg)
	})
	if target == nil {
		t.Fatal("target fiber create failed")
	}
	mailbox := NewMailBox(ActorID{
		ProcessID:  1,
		FiberID:    target.ID(),
		InstanceID: target.Root().InstanceID(),
	}, MailBoxTypeUnOrderedMessage)
	target.Root().AddComponent(mailbox)
	mailbox.RegisterHandler(4101, func(_ ActorID, _ uint16, payload []byte) ([]byte, error) {
		return append([]byte("remote:"), payload...), nil
	})

	outer := NewProcessOuterSender(NewRpcManager(WithRPCTimeout(time.Second)))
	outer.SetFiberManager(manager)
	actorID := ActorID{ProcessID: 1, FiberID: target.ID(), InstanceID: target.Root().InstanceID()}
	envelope, err := encodeActorEnvelope(actorID, []byte("request"))
	if err != nil {
		t.Fatalf("encode envelope error = %v", err)
	}
	response, err := outer.HandlePacket(context.Background(), &codec.Packet{
		Type:    codec.PacketTypeRequest,
		MsgID:   4101,
		RpcID:   9,
		Payload: envelope,
	})
	if err != nil {
		t.Fatalf("HandlePacket error = %v", err)
	}
	if response == nil || response.Type != codec.PacketTypeResponse || response.RpcID != 9 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if string(response.Payload) != "remote:request" {
		t.Fatalf("response payload = %q", response.Payload)
	}
}

func TestProcessOuterSenderRejectsResponseWithoutRPCID(t *testing.T) {
	outer := NewProcessOuterSender(NewRpcManager())
	if _, err := outer.HandlePacket(context.Background(), &codec.Packet{
		Type:  codec.PacketTypeResponse,
		MsgID: 4101,
	}); !errors.Is(err, ErrInvalidPacket) {
		t.Fatalf("response without RPC ID error = %v, want %v", err, ErrInvalidPacket)
	}
}

func TestProcessOuterSenderDoesNotReopenAfterDestroy(t *testing.T) {
	outer := NewProcessOuterSender(NewRpcManager())
	outer.OnDestroy()
	outer.AddSession(2, &recordingPacketSession{sent: make(chan *codec.Packet, 1)})

	err := outer.Send(ActorID{ProcessID: 2, FiberID: 1, InstanceID: 1}, 1, nil)
	if !errors.Is(err, ErrActorNotFound) {
		t.Fatalf("Send after destroy error = %v, want %v", err, ErrActorNotFound)
	}
}

func TestMessageSenderResolvesRegisteredProcessOuterSender(t *testing.T) {
	outer := NewProcessOuterSender(NewRpcManager())
	session := &recordingPacketSession{sent: make(chan *codec.Packet, 1)}
	outer.AddSession(2, session)
	if err := RegisterProcessOuterSender(1, outer); err != nil {
		t.Fatalf("RegisterProcessOuterSender error = %v", err)
	}
	defer outer.OnDestroy()

	sender := NewMessageSender(1, nil, nil)
	target := ActorID{ProcessID: 2, FiberID: 9, InstanceID: 10}
	if err := sender.Send(target, 4201, []byte("payload")); err != nil {
		t.Fatalf("MessageSender.Send error = %v", err)
	}
	packet := <-session.sent
	if packet == nil || packet.Type != codec.PacketTypeMessage || packet.MsgID != 4201 {
		t.Fatalf("unexpected packet = %+v", packet)
	}
	gotActor, gotPayload, err := DecodeActorEnvelope(packet.Payload)
	if err != nil {
		t.Fatalf("DecodeActorEnvelope error = %v", err)
	}
	if gotActor != target || string(gotPayload) != "payload" {
		t.Fatalf("packet actor/payload = %v/%q, want %v/%q", gotActor, gotPayload, target, "payload")
	}
}
