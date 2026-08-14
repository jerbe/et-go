package router

import (
	"errors"
	"net"
	"testing"
	"time"
)

type sentPacket struct {
	protocol  KcpProtocol
	outerConn uint32
	innerConn uint32
	connectID uint32
	payload   []byte
	addr      string
}

type stubTransport struct {
	packets []sentPacket
}

func (t *stubTransport) Send(to *net.UDPAddr, protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) error {
	addr := ""
	if to != nil {
		addr = to.String()
	}
	t.packets = append(t.packets, sentPacket{
		protocol:  protocol,
		outerConn: outerConn,
		innerConn: innerConn,
		connectID: connectID,
		payload:   append([]byte(nil), payload...),
		addr:      addr,
	})
	return nil
}

func TestRouterStateMachine(t *testing.T) {
	component := &RouterComponent{}
	component.Awake()
	transport := &stubTransport{}
	component.SetTransport(transport)
	now := time.Now()
	component.SetNowFunc(func() time.Time { return now })
	system := NewRouterComponentSystem(component)

	outerAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1000}
	node, err := system.HandleRouterSYN(100, "127.0.0.1:2000", outerAddr)
	if err != nil {
		t.Fatalf("HandleRouterSYN err = %v", err)
	}
	if node.Status != RouterStatusSync || len(transport.packets) != 1 {
		t.Fatalf("unexpected node after syn: %+v packets=%d", node, len(transport.packets))
	}
	if !system.HandleRouterACK(200, node.ConnectId) {
		t.Fatal("expected ack success")
	}
	if node.Status != RouterStatusMsg {
		t.Fatalf("node status = %s", node.Status)
	}
	if !system.HandleOuterMsg(100, []byte("hello"), outerAddr) {
		t.Fatal("expected outer msg forward")
	}
	if !system.HandleInnerMsg(200, 100, []byte("world")) {
		t.Fatal("expected inner msg forward")
	}
	system.HandleFIN(100)
	if _, ok := component.GetNodeByOuter(100); ok {
		t.Fatal("node should be removed after FIN")
	}
}

func TestRouterLimitAndTimeout(t *testing.T) {
	component := &RouterComponent{}
	component.Awake()
	now := time.Now()
	component.SetNowFunc(func() time.Time { return now })
	system := NewRouterComponentSystem(component)
	node := &RouterNode{
		OuterConnID:       1,
		ConnectId:         1,
		OuterAddr:         &net.UDPAddr{},
		InnerAddr:         &net.UDPAddr{},
		Status:            RouterStatusMsg,
		LastRecvOuterTime: now.Add(-60 * time.Second),
		LastRecvInnerTime: now.Add(-60 * time.Second),
	}
	component.AddNode(node)
	system.Update()
	if _, ok := component.GetNodeByOuter(1); ok {
		t.Fatal("node should be cleaned by timeout")
	}

	node = &RouterNode{}
	node.Awake()
	for i := 0; i < 1000; i++ {
		if !node.CheckOuterCount(now) {
			t.Fatalf("check failed at %d", i)
		}
	}
	if node.CheckOuterCount(now) {
		t.Fatal("limit should reject 1001st packet")
	}
}

func TestRouterRejectsRequestsWhenTransportIsUnavailable(t *testing.T) {
	component := &RouterComponent{}
	component.Awake()
	now := time.Now()
	component.SetNowFunc(func() time.Time { return now })
	system := NewRouterComponentSystem(component)
	outerAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1000}

	node, err := system.HandleRouterSYN(100, "127.0.0.1:2000", outerAddr)
	if !errors.Is(err, ErrRouterTransportClosed) {
		t.Fatalf("HandleRouterSYN err = %v, want %v", err, ErrRouterTransportClosed)
	}
	if node != nil {
		t.Fatalf("HandleRouterSYN node = %+v, want nil", node)
	}
	if _, ok := component.GetNodeByOuter(100); ok {
		t.Fatal("transport failure must not register a route node")
	}

	node = &RouterNode{
		OuterConnID:       100,
		ConnectId:         200,
		InnerConn:         300,
		OuterAddr:         outerAddr,
		InnerAddr:         &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2000},
		Status:            RouterStatusMsg,
		LastRecvOuterTime: now,
		LastRecvInnerTime: now,
	}
	component.AddNode(node)

	if system.HandleRouterACK(301, node.ConnectId) {
		t.Fatal("HandleRouterACK must reject missing transport")
	}
	if node.Status != RouterStatusMsg || node.InnerConn != 300 {
		t.Fatalf("HandleRouterACK mutated node without transport: %+v", node)
	}
	if system.HandleOuterMsg(node.OuterConn(), []byte("outer"), outerAddr) {
		t.Fatal("HandleOuterMsg must reject missing transport")
	}
	if system.HandleInnerMsg(node.InnerConn, node.OuterConn(), []byte("inner")) {
		t.Fatal("HandleInnerMsg must reject missing transport")
	}
	if system.HandleRouterReconnSYN(node.OuterConn(), outerAddr) {
		t.Fatal("HandleRouterReconnSYN must reject missing transport")
	}
	if system.HandleRouterReconnACK(node.OuterConn()) {
		t.Fatal("HandleRouterReconnACK must reject missing transport")
	}
}
