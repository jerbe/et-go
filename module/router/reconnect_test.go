package router

import (
	"net"
	"testing"
	"time"
)

func TestRouterReconnectFlow(t *testing.T) {
	component := &RouterComponent{}
	component.Awake()
	transport := &stubTransport{}
	component.SetTransport(transport)
	now := time.Now()
	component.SetNowFunc(func() time.Time { return now })
	system := NewRouterComponentSystem(component)

	outerAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1000}
	node, err := system.HandleRouterSYN(123, "127.0.0.1:2000", outerAddr)
	if err != nil {
		t.Fatalf("HandleRouterSYN err = %v", err)
	}
	if !system.HandleRouterACK(456, node.ConnectId) {
		t.Fatal("Router ACK failed")
	}
	if node.Status != RouterStatusMsg {
		t.Fatalf("node status = %s", node.Status)
	}

	newOuterAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 2000}
	if !system.HandleRouterReconnSYN(123, newOuterAddr) {
		t.Fatal("expected reconnect syn to succeed")
	}
	if node.Status != RouterStatusSync {
		t.Fatalf("expected sync state after reconnect, got %s", node.Status)
	}
	if node.OuterAddr.String() != newOuterAddr.String() {
		t.Fatalf("outer addr not updated: want %s, got %s", newOuterAddr, node.OuterAddr)
	}
	if len(transport.packets) == 0 {
		t.Fatal("expected reconnect syn to be forwarded")
	}
	last := transport.packets[len(transport.packets)-1]
	if last.protocol != KcpRouterReconnSYN {
		t.Fatalf("expected reconnect syn packet, got %v", last.protocol)
	}

	transport.packets = transport.packets[:0]
	if !system.HandleRouterReconnACK(123) {
		t.Fatal("expected reconnect ack to succeed")
	}
	if node.Status != RouterStatusMsg {
		t.Fatalf("expected msg state after ack, got %s", node.Status)
	}
	if len(transport.packets) == 0 {
		t.Fatal("expected reconnect ack packet")
	}
	last = transport.packets[len(transport.packets)-1]
	if last.protocol != KcpRouterReconnACK {
		t.Fatalf("expected reconnect ack packet, got %v", last.protocol)
	}
	if last.addr != newOuterAddr.String() {
		t.Fatalf("ack sent to wrong address: %s", last.addr)
	}

	transport.packets = transport.packets[:0]
	if !system.HandleOuterMsg(123, []byte("ping"), newOuterAddr) {
		t.Fatal("expected message to forward after reconnect")
	}
	if len(transport.packets) == 0 {
		t.Fatal("expected forward packet")
	}
	last = transport.packets[len(transport.packets)-1]
	if last.protocol != KcpMSG {
		t.Fatalf("expected MSG packet, got %v", last.protocol)
	}
	if last.addr != node.InnerAddr.String() {
		t.Fatalf("expected inner addr %s, got %s", node.InnerAddr, last.addr)
	}
}
