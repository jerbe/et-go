package router

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPTransportUsesTargetOuterLengthPrefixAndRouterWire(t *testing.T) {
	transport, err := newTCPTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("newTCPTransport error = %v", err)
	}
	defer transport.Close()

	component := &RouterComponent{}
	component.Awake()
	component.SetOuterTCPTransport(transport)
	component.SetInnerTransport(&stubTransport{})
	system := NewRouterComponentSystem(component)

	client, err := net.Dial("tcp", transport.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial error = %v", err)
	}
	defer client.Close()

	frame := encodeRouterFrame(KcpRouterSYN, 100, 0, 55, []byte("127.0.0.1:2000"))
	if err := writeTCPTestFrame(client, frame); err != nil {
		t.Fatalf("write RouterSYN error = %v", err)
	}
	waitForRouterTCPPoll(t, transport, system)

	response, err := readTCPTestFrame(client)
	if err != nil {
		t.Fatalf("read RouterACK error = %v", err)
	}
	protocol, outer, inner, connectID, _, err := decodeRouterFrame(response)
	if err != nil {
		t.Fatalf("decode RouterACK error = %v", err)
	}
	if protocol != KcpRouterACK || outer != 100 || inner != 0 || connectID != 0 {
		t.Fatalf("RouterACK = protocol=%d outer=%d inner=%d connect=%d", protocol, outer, inner, connectID)
	}

	if !system.HandleRouterACK(200, 100) {
		t.Fatal("HandleRouterACK should use the accepted TCP transport")
	}
	response, err = readTCPTestFrame(client)
	if err != nil {
		t.Fatalf("read ordinary ACK error = %v", err)
	}
	protocol, outer, inner, _, _, err = decodeRouterFrame(response)
	if err != nil || protocol != KcpACK || outer != 100 || inner != 200 {
		t.Fatalf("ordinary ACK = protocol=%d outer=%d inner=%d err=%v", protocol, outer, inner, err)
	}
}

func writeTCPTestFrame(conn net.Conn, frame []byte) error {
	if len(frame) > int(^uint16(0)) {
		return io.ErrShortBuffer
	}
	packet := make([]byte, 2+len(frame))
	binary.LittleEndian.PutUint16(packet[:2], uint16(len(frame)))
	copy(packet[2:], frame)
	_, err := writeFull(conn, packet)
	return err
}

func readTCPTestFrame(conn net.Conn) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	frame := make([]byte, int(binary.LittleEndian.Uint16(header)))
	if _, err := io.ReadFull(conn, frame); err != nil {
		return nil, err
	}
	return frame, nil
}

func waitForRouterTCPPoll(t *testing.T, transport *tcpTransport, system *RouterComponentSystem) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		transport.Poll(system)
		if _, ok := system.component.GetNodeByOuter(100); ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("RouterSYN was not accepted by TCP transport")
}
