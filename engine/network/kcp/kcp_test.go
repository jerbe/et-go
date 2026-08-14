package kcp

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/jerbe/et-go/engine/network/codec"
	etlog "github.com/jerbe/et-go/internal/log"
)

func TestConfigFactoriesAndProtocolConstants(t *testing.T) {
	inner := InnerConfig()
	if inner.WndSend != 1024 || inner.WndRecv != 1024 || inner.MTU != 1400 {
		t.Fatalf("unexpected inner config: %+v", inner)
	}
	outer := OuterConfig()
	if outer.WndSend != 256 || outer.WndRecv != 256 || outer.MTU != 470 {
		t.Fatalf("unexpected outer config: %+v", outer)
	}

	if ProtoSYN != 1 || ProtoACK != 2 || ProtoFIN != 3 || ProtoMSG != 4 ||
		ProtoRouterReconnSYN != 5 || ProtoRouterReconnACK != 6 || ProtoRouterSYN != 7 || ProtoRouterACK != 8 {
		t.Fatal("protocol constants mismatch")
	}
	if ConnectTimeout != 20000 || AcceptTimeout != 20000 || MaxMessageSize != 10000 {
		t.Fatal("timeout or size constants mismatch")
	}
}

func TestKCPConfigRetainedAndQueues(t *testing.T) {
	var sent [][]byte
	sender := NewKCPWithConv(7, InnerConfig(), func(data []byte) error {
		sent = append(sent, append([]byte(nil), data...))
		return nil
	})
	receiver := NewKCPWithConv(7, InnerConfig(), nil)
	if sender.Config().MTU != 1400 {
		t.Fatalf("config MTU = %d, want 1400", sender.Config().MTU)
	}
	if err := sender.Send([]byte("hello")); err != nil {
		t.Fatalf("sender.Send error = %v", err)
	}
	if err := sender.Update(0); err != nil {
		t.Fatalf("sender.Update error = %v", err)
	}
	if len(sent) == 0 {
		t.Fatal("sender.Update should output a standard KCP segment")
	}
	for _, frame := range sent {
		if err := receiver.Input(frame); err != nil {
			t.Fatalf("receiver.Input error = %v", err)
		}
	}
	payload, ok := receiver.Recv()
	if !ok || string(payload) != "hello" {
		t.Fatalf("recv = %q, want hello", payload)
	}
}

func TestKCPRetransmitsDroppedSegment(t *testing.T) {
	var senderFrames [][]byte
	sender := NewKCPWithConv(8, InnerConfig(), func(frame []byte) error {
		senderFrames = append(senderFrames, append([]byte(nil), frame...))
		return nil
	})
	receiverToSender := make([][]byte, 0)
	receiver := NewKCPWithConv(8, InnerConfig(), func(frame []byte) error {
		receiverToSender = append(receiverToSender, append([]byte(nil), frame...))
		return nil
	})
	if err := sender.Send([]byte("retransmit")); err != nil {
		t.Fatalf("sender.Send error = %v", err)
	}

	delivered := false
	droppedFirst := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := sender.Update(0); err != nil {
			t.Fatalf("sender.Update error = %v", err)
		}
		for len(senderFrames) > 0 {
			frame := senderFrames[0]
			senderFrames = senderFrames[1:]
			if !droppedFirst {
				droppedFirst = true
				continue
			}
			if err := receiver.Input(frame); err != nil {
				t.Fatalf("receiver.Input error = %v", err)
			}
			if payload, ok := receiver.Recv(); ok {
				if string(payload) != "retransmit" {
					t.Fatalf("payload = %q, want retransmit", payload)
				}
				delivered = true
			}
		}
		if err := receiver.Update(0); err != nil {
			t.Fatalf("receiver.Update error = %v", err)
		}
		for len(receiverToSender) > 0 {
			frame := receiverToSender[0]
			receiverToSender = receiverToSender[1:]
			if err := sender.Input(frame); err != nil {
				t.Fatalf("sender.Input ACK error = %v", err)
			}
		}
		if delivered {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropped KCP segment was not retransmitted")
}

func TestKChannelStateAndTimeout(t *testing.T) {
	channel := NewKChannel(1, nil, InnerConfig(), nil)
	if channel.Status() != ChannelStatusConnecting {
		t.Fatalf("status = %v, want connecting", channel.Status())
	}
	if channel.IsTimeout(channel.createTime + ConnectTimeout - 1) {
		t.Fatal("channel should not time out before threshold")
	}
	if !channel.IsTimeout(channel.createTime + ConnectTimeout + 1) {
		t.Fatal("channel should time out after threshold")
	}
	channel.close(true)
	if channel.Status() != ChannelStatusDisconnected {
		t.Fatalf("status = %v, want disconnected", channel.Status())
	}
}

func TestKChannelRejectsSendWithoutService(t *testing.T) {
	channel := NewKChannel(1, nil, InnerConfig(), nil)
	if err := channel.Send([]byte("payload")); err != ErrServiceNotListening {
		t.Fatalf("Send error = %v, want %v", err, ErrServiceNotListening)
	}
}

func TestKServiceLoopbackConnectAndSend(t *testing.T) {
	server := NewService(InnerConfig(), etlog.New("error"))
	if err := server.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("server listen error: %v", err)
	}
	defer server.Close()

	client := NewService(InnerConfig(), etlog.New("error"))
	if err := client.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("client listen error: %v", err)
	}
	defer client.Close()

	serverAccept := make(chan *KChannel, 1)
	server.SetOnAccept(func(ch *KChannel, _ net.Conn) {
		serverAccept <- ch
	})

	clientChannel, err := client.Connect(server.Addr().String())
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}

	serverChannel := pumpUntilAccepted(t, client, server, serverAccept)
	pumpFor(t, 100*time.Millisecond, client, server)
	if clientChannel.Status() != ChannelStatusConnected {
		t.Fatalf("client status = %v, want connected", clientChannel.Status())
	}
	if serverChannel.Status() != ChannelStatusConnected {
		t.Fatalf("server status = %v, want connected", serverChannel.Status())
	}

	received := make(chan []byte, 1)
	serverChannel.SetOnRecv(func(ch *KChannel, data []byte) {
		received <- append([]byte(nil), data...)
	})

	if err := clientChannel.Send([]byte("payload")); err != nil {
		t.Fatalf("send error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, client, server)

	select {
	case got := <-received:
		if string(got) != "payload" {
			t.Fatalf("payload = %q, want payload", got)
		}
	case <-time.After(time.Second):
		t.Fatal("wait loopback payload timeout")
	}
}

func TestKServiceAcceptsSameClientLocalConnFromDifferentAddresses(t *testing.T) {
	server := NewService(InnerConfig(), etlog.New("error"))
	if err := server.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("server listen error: %v", err)
	}
	defer server.Close()

	clientA := NewService(InnerConfig(), etlog.New("error"))
	if err := clientA.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("client A listen error: %v", err)
	}
	defer clientA.Close()

	clientB := NewService(InnerConfig(), etlog.New("error"))
	if err := clientB.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("client B listen error: %v", err)
	}
	defer clientB.Close()

	accepted := make(chan *KChannel, 2)
	server.SetOnAccept(func(ch *KChannel, _ net.Conn) {
		accepted <- ch
	})

	channelA, err := clientA.Connect(server.Addr().String())
	if err != nil {
		t.Fatalf("client A connect error: %v", err)
	}
	channelB, err := clientB.Connect(server.Addr().String())
	if err != nil {
		t.Fatalf("client B connect error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var serverChannels []*KChannel
	for time.Now().Before(deadline) && len(serverChannels) < 2 {
		clientA.Update()
		clientB.Update()
		server.Update()
		for {
			select {
			case channel := <-accepted:
				serverChannels = append(serverChannels, channel)
			default:
				goto next
			}
		}
	next:
		time.Sleep(5 * time.Millisecond)
	}
	if len(serverChannels) != 2 {
		t.Fatalf("accepted channels = %d, want 2", len(serverChannels))
	}
	pumpFor(t, 100*time.Millisecond, clientA, clientB, server)
	if channelA.Status() != ChannelStatusConnected || channelB.Status() != ChannelStatusConnected {
		t.Fatalf("client statuses = %v, %v; want connected", channelA.Status(), channelB.Status())
	}
	if serverChannels[0] == serverChannels[1] || serverChannels[0].ID() == serverChannels[1].ID() {
		t.Fatalf("server reused accepted channel: %p/%p ids=%d/%d", serverChannels[0], serverChannels[1], serverChannels[0].ID(), serverChannels[1].ID())
	}
	received := make(chan string, 2)
	clientReceived := make(chan string, 2)
	for index, channel := range serverChannels {
		channel.SetOnRecv(func(_ *KChannel, payload []byte) {
			received <- string(payload)
		})
		if index == 0 {
			clientChannel := channelA
			clientChannel.SetOnRecv(func(_ *KChannel, payload []byte) {
				clientReceived <- string(payload)
			})
		} else {
			clientChannel := channelB
			clientChannel.SetOnRecv(func(_ *KChannel, payload []byte) {
				clientReceived <- string(payload)
			})
		}
		if index == 0 {
			if err := channelA.Send([]byte("client-a")); err != nil {
				t.Fatalf("client A send error: %v", err)
			}
		} else {
			if err := channelB.Send([]byte("client-b")); err != nil {
				t.Fatalf("client B send error: %v", err)
			}
		}
	}
	pumpFor(t, 200*time.Millisecond, clientA, clientB, server)
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case payload := <-received:
			got[payload] = true
		default:
			if len(got) != 2 {
				t.Fatalf("received payloads = %v, want client-a and client-b", got)
			}
		}
	}
	if err := serverChannels[0].Send([]byte("server-a")); err != nil {
		t.Fatalf("server A send error: %v", err)
	}
	if err := serverChannels[1].Send([]byte("server-b")); err != nil {
		t.Fatalf("server B send error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, clientA, clientB, server)
	clientGot := map[string]bool{}
	for len(clientGot) < 2 {
		select {
		case payload := <-clientReceived:
			clientGot[payload] = true
		default:
			if len(clientGot) != 2 {
				t.Fatalf("client received payloads = %v, want server-a and server-b", clientGot)
			}
		}
	}
	channelA.Close()
	pumpFor(t, 100*time.Millisecond, clientA, clientB, server)
	if channelB.Status() != ChannelStatusConnected {
		t.Fatalf("client B status after client A close = %v, want connected", channelB.Status())
	}
	if err := channelB.Send([]byte("client-b-after-a-close")); err != nil {
		t.Fatalf("client B send after client A close error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, clientB, server)
	select {
	case payload := <-received:
		if payload != "client-b-after-a-close" {
			t.Fatalf("payload after client A close = %q, want client-b-after-a-close", payload)
		}
	default:
		t.Fatal("client B payload after client A close was not received")
	}
}

func TestKServiceListenIsSingleAssignment(t *testing.T) {
	service := NewService(InnerConfig(), etlog.New("error"))
	defer service.Close()

	const attempts = 8
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer wg.Done()
			results <- service.Listen("127.0.0.1:0")
		}()
	}
	wg.Wait()
	close(results)

	var success int
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent Listen success count = %d, want 1", success)
	}
}

func TestKServiceSessionIntegration(t *testing.T) {
	server := NewService(InnerConfig(), etlog.New("error"))
	if err := server.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("server listen error: %v", err)
	}
	defer server.Close()

	client := NewService(InnerConfig(), etlog.New("error"))
	if err := client.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("client listen error: %v", err)
	}
	defer client.Close()

	packetCh := make(chan *codec.Packet, 1)
	serverAccept := make(chan *KChannel, 1)
	server.SetOnAccept(func(ch *KChannel, conn net.Conn) {
		serverAccept <- ch
		go func() {
			pkt, err := codec.Decode(conn)
			if err == nil {
				packetCh <- pkt
			}
		}()
	})

	clientChannel, err := client.Connect(server.Addr().String())
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	serverChannel := pumpUntilAccepted(t, client, server, serverAccept)

	packet := &codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   1001,
		Payload: []byte("hello"),
	}
	conn := newChannelConn(clientChannel)
	clientChannel.attachConn(conn)
	clientChannel.markConnected()

	encoded, err := codec.Encode(packet)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}
	if _, err := conn.Write(encoded); err != nil {
		t.Fatalf("conn write error: %v", err)
	}

	pumpFor(t, 300*time.Millisecond, client, server)

	select {
	case got := <-packetCh:
		if got.MsgID != packet.MsgID || !bytes.Equal(got.Payload, packet.Payload) {
			t.Fatalf("packet mismatch: got=%+v want=%+v", got, packet)
		}
	case <-time.After(time.Second):
		t.Fatal("wait session packet timeout")
	}

	serverChannel.Close()
	pumpFor(t, 200*time.Millisecond, client, server)
	if serverChannel.Conn() == nil {
		t.Fatal("server channel conn should be created")
	}
}

func TestRouterProtocols(t *testing.T) {
	server := NewService(InnerConfig(), etlog.New("error"))
	if err := server.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("server listen error: %v", err)
	}
	defer server.Close()

	client := NewService(InnerConfig(), etlog.New("error"))
	if err := client.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("client listen error: %v", err)
	}
	defer client.Close()

	channel, err := client.Connect(server.Addr().String())
	if err != nil {
		t.Fatalf("connect error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, client, server)

	if err := client.sendControl(server.Addr(), ProtoRouterReconnSYN, channel.ID(), nil); err != nil {
		t.Fatalf("router reconn syn error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, client, server)

	if err := client.sendControl(server.Addr(), ProtoRouterSYN, channel.ID(), nil); err != nil {
		t.Fatalf("router syn error: %v", err)
	}
	pumpFor(t, 200*time.Millisecond, client, server)
}

func pumpUntilAccepted(t *testing.T, client, server *KService, acceptCh <-chan *KChannel) *KChannel {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		client.Update()
		server.Update()
		select {
		case channel := <-acceptCh:
			return channel
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("wait accept timeout")
	return nil
}

func pumpFor(t *testing.T, duration time.Duration, services ...*KService) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		for _, service := range services {
			if service != nil {
				service.Update()
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
}
