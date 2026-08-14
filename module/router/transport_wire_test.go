package router

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRouterFrameUsesTargetLittleEndianLayout(t *testing.T) {
	frame := encodeRouterFrame(KcpRouterSYN, 0x01020304, 0x05060708, 0x0a0b0c0d, []byte("inner"))
	if len(frame) != 18 {
		t.Fatalf("RouterSYN length = %d, want 18", len(frame))
	}
	if frame[0] != byte(KcpRouterSYN) {
		t.Fatalf("RouterSYN protocol = %d", frame[0])
	}
	if got := binary.LittleEndian.Uint32(frame[1:5]); got != 0x01020304 {
		t.Fatalf("RouterSYN outer = %#x", got)
	}
	if got := binary.LittleEndian.Uint32(frame[5:9]); got != 0x05060708 {
		t.Fatalf("RouterSYN inner = %#x", got)
	}
	if got := binary.LittleEndian.Uint32(frame[9:13]); got != 0x0a0b0c0d {
		t.Fatalf("RouterSYN connect = %#x", got)
	}
	if !bytes.Equal(frame[13:], []byte("inner")) {
		t.Fatalf("RouterSYN address = %q", frame[13:])
	}

	protocol, outer, inner, connect, payload, err := decodeRouterFrame(frame)
	if err != nil {
		t.Fatalf("decode RouterSYN error = %v", err)
	}
	if protocol != KcpRouterSYN || outer != 0x01020304 || inner != 0x05060708 || connect != 0x0a0b0c0d || string(payload) != "inner" {
		t.Fatalf("decoded RouterSYN = protocol=%d outer=%#x inner=%#x connect=%#x payload=%q", protocol, outer, inner, connect, payload)
	}

	ack := encodeRouterFrameForDestination(routerFrameToOuter, KcpACK, 0x11223344, 0x55667788, 0, nil)
	if want := []byte{byte(KcpACK), 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11}; !bytes.Equal(ack, want) {
		t.Fatalf("ACK wire = %v, want %v", ack, want)
	}
	protocol, outer, inner, connect, payload, err = decodeRouterFrame(ack)
	if err != nil {
		t.Fatalf("decode ACK error = %v", err)
	}
	if protocol != KcpACK || outer != 0x11223344 || inner != 0x55667788 || connect != 0 || len(payload) != 0 {
		t.Fatalf("decoded ACK = protocol=%d outer=%#x inner=%#x connect=%#x payload=%q", protocol, outer, inner, connect, payload)
	}

	reconnect := encodeRouterFrameForDestination(routerFrameToInner, KcpRouterReconnSYN, 7, 8, 0, nil)
	if len(reconnect) != routerFrameHeaderSize {
		t.Fatalf("forwarded RouterReconnectSYN length = %d, want %d", len(reconnect), routerFrameHeaderSize)
	}

	fin := encodeRouterFrameForDestination(routerFrameToOuter, KcpFIN, 7, 8, 99, nil)
	if len(fin) != routerFrameHeaderSize+4 {
		t.Fatalf("FIN length = %d, want %d", len(fin), routerFrameHeaderSize+4)
	}
	_, outer, inner, connect, _, err = decodeRouterFrameFromSource(fin, routerFrameFromInner)
	if err != nil || outer != 7 || inner != 8 || connect != 99 {
		t.Fatalf("decoded FIN = outer=%d inner=%d error=%d err=%v", outer, inner, connect, err)
	}
}
