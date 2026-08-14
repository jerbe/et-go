package network

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/network/codec"
)

func TestCodecRoundTrip(t *testing.T) {
	cases := []*codec.Packet{
		{Type: codec.PacketTypeMessage, MsgID: 1, RpcID: 0, Payload: []byte("message")},
		{Type: codec.PacketTypeRequest, MsgID: 2, RpcID: 123, Payload: []byte("request")},
		{Type: codec.PacketTypeResponse, MsgID: 3, RpcID: 456, Payload: []byte("response")},
		{Type: codec.PacketTypeMessage, MsgID: 4, RpcID: 0, Payload: nil},
	}

	for _, packet := range cases {
		encoded, err := codec.Encode(packet)
		if err != nil {
			t.Fatalf("encode error: %v", err)
		}
		decoded, err := codec.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if decoded.Type != packet.Type || decoded.MsgID != packet.MsgID || decoded.RpcID != packet.RpcID {
			t.Fatalf("packet meta mismatch: got=%+v want=%+v", decoded, packet)
		}
		if !bytes.Equal(decoded.Payload, packet.Payload) {
			t.Fatalf("payload mismatch: got=%q want=%q", decoded.Payload, packet.Payload)
		}
	}
}

func TestCodecMaxPayloadAndTooLarge(t *testing.T) {
	maxPayload := bytes.Repeat([]byte{1}, codec.MaxPayloadSize)
	if _, err := codec.Encode(&codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   10,
		Payload: maxPayload,
	}); err != nil {
		t.Fatalf("encode max payload error: %v", err)
	}

	tooLarge := bytes.Repeat([]byte{1}, codec.MaxPayloadSize+1)
	_, err := codec.Encode(&codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   11,
		Payload: tooLarge,
	})
	if !errors.Is(err, codec.ErrPacketTooLarge) {
		t.Fatalf("too large payload err = %v, want %v", err, codec.ErrPacketTooLarge)
	}
}

func TestCodecDecodeTruncatedData(t *testing.T) {
	packet := &codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   12,
		Payload: []byte("truncated"),
	}
	encoded, err := codec.Encode(packet)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	truncated := encoded[:len(encoded)-2]
	if _, err := codec.Decode(bytes.NewReader(truncated)); err == nil {
		t.Fatal("decode truncated data should fail")
	}
}
