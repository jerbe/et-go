package router

import (
	"net"
	"testing"
	"time"
)

func TestRouterForwardFullFlow(t *testing.T) {
	component := &RouterComponent{}
	component.Awake()
	transport, err := newUDPTransport("127.0.0.1:0")
	if err != nil {
		t.Fatalf("newUDPTransport err = %v", err)
	}
	defer transport.Close()
	component.SetTransport(transport)
	system := NewRouterComponentSystem(component)

	outerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP outer err = %v", err)
	}
	defer outerConn.Close()
	innerConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP inner err = %v", err)
	}
	defer innerConn.Close()

	routerAddr := transport.Addr()
	if routerAddr == nil {
		t.Fatal("router addr nil")
	}

	// outer -> router: RouterSYN with the inner address and connect ID.
	if _, err := outerConn.WriteToUDP(encodeRouterFrame(KcpRouterSYN, 100, 0, 42, []byte(innerConn.LocalAddr().String())), routerAddr); err != nil {
		t.Fatalf("write router syn err = %v", err)
	}
	system.Update()

	buf := make([]byte, 1024)
	_ = outerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err := outerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("outer read router ack err = %v", err)
	}
	protocol, outerID, innerID, connectID, payload, err := decodeRouterFrame(buf[:n])
	if err != nil || protocol != KcpRouterACK || outerID != 100 || innerID != 0 || connectID != 0 {
		t.Fatalf("unexpected router ack protocol=%v outer=%d inner=%d connect=%d err=%v", protocol, outerID, innerID, connectID, err)
	}

	// outer -> router: ordinary KCP SYN, forwarded to the inner service.
	if _, err := outerConn.WriteToUDP(encodeRouterFrame(KcpSYN, 100, 0, 0, nil), routerAddr); err != nil {
		t.Fatalf("write syn err = %v", err)
	}
	system.Update()

	_ = innerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = innerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("inner read syn err = %v", err)
	}
	protocol, outerID, innerID, _, payload, err = decodeRouterFrame(buf[:n])
	if err != nil || protocol != KcpSYN || outerID != 100 || innerID != 0 || string(payload) == "" {
		t.Fatalf("unexpected inner syn protocol=%v outer=%d inner=%d payload=%q err=%v", protocol, outerID, innerID, payload, err)
	}

	// inner -> router: ordinary KCP ACK, forwarded to the outer peer.
	if _, err := innerConn.WriteToUDP(encodeRouterFrameFromInner(KcpACK, 100, 200, 0, nil), routerAddr); err != nil {
		t.Fatalf("write ack err = %v", err)
	}
	system.Update()

	_ = outerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = outerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("outer read ack err = %v", err)
	}
	protocol, outerID, innerID, _, _, err = decodeRouterFrame(buf[:n])
	if err != nil || protocol != KcpACK || outerID != 100 || innerID != 200 {
		t.Fatalf("unexpected ack frame protocol=%v outer=%d inner=%d err=%v", protocol, outerID, innerID, err)
	}

	// outer -> router -> inner
	if _, err := outerConn.WriteToUDP(encodeRouterFrame(KcpMSG, 100, 200, 0, []byte("hello")), routerAddr); err != nil {
		t.Fatalf("write msg err = %v", err)
	}
	system.Update()
	_ = innerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = innerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("inner read msg err = %v", err)
	}
	protocol, outerID, innerID, _, payload, err = decodeRouterFrame(buf[:n])
	if err != nil || protocol != KcpMSG || outerID != 100 || innerID != 200 || string(payload) != "hello" {
		t.Fatalf("unexpected inner msg protocol=%v outer=%d inner=%d payload=%q err=%v", protocol, outerID, innerID, payload, err)
	}

	// inner -> router -> outer
	if _, err := innerConn.WriteToUDP(encodeRouterFrameFromInner(KcpMSG, 100, 200, 0, []byte("world")), routerAddr); err != nil {
		t.Fatalf("write inner msg err = %v", err)
	}
	system.Update()
	_ = outerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = outerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("outer read msg err = %v", err)
	}
	protocol, outerID, innerID, _, payload, err = decodeRouterFrameFromSource(buf[:n], routerFrameFromInner)
	if err != nil || protocol != KcpMSG || outerID != 100 || innerID != 200 || string(payload) != "world" {
		t.Fatalf("unexpected outer msg protocol=%v outer=%d inner=%d payload=%q err=%v", protocol, outerID, innerID, payload, err)
	}

	// outer -> router: FIN
	if _, err := outerConn.WriteToUDP(encodeRouterFrame(KcpFIN, 100, 200, 7, nil), routerAddr); err != nil {
		t.Fatalf("write fin err = %v", err)
	}
	system.Update()
	_ = innerConn.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = innerConn.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("inner read fin err = %v", err)
	}
	protocol, outerID, innerID, connectID, _, err = decodeRouterFrame(buf[:n])
	if err != nil || protocol != KcpFIN || outerID != 100 || innerID != 200 || connectID != 7 {
		t.Fatalf("unexpected inner fin protocol=%v outer=%d inner=%d error=%d err=%v", protocol, outerID, innerID, connectID, err)
	}
	system.HandleFIN(100)
	if _, ok := component.GetNodeByOuter(100); ok {
		t.Fatal("node should be removed after explicit cleanup")
	}
}
