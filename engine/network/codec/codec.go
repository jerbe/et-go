package codec

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	// ErrPacketTooLarge 数据包超过最大长度
	ErrPacketTooLarge = errors.New("codec: packet too large")
	// ErrInvalidPacket 无效数据包
	ErrInvalidPacket = errors.New("codec: invalid packet")
	// ErrReaderRequired 表示解码器缺少输入流。
	ErrReaderRequired = errors.New("codec: reader required")
)

// MaxPayloadSize 最大负载长度 (16MB)
const MaxPayloadSize = 16 * 1024 * 1024

// Encode 将 Packet 编码为字节流
func Encode(pkt *Packet) ([]byte, error) {
	if pkt == nil {
		return nil, ErrInvalidPacket
	}
	payloadLen := len(pkt.Payload)
	if payloadLen > MaxPayloadSize {
		return nil, ErrPacketTooLarge
	}

	buf := make([]byte, HeaderSize+payloadLen)
	buf[0] = byte(pkt.Type)
	binary.BigEndian.PutUint16(buf[1:3], pkt.MsgID)
	binary.BigEndian.PutUint32(buf[3:7], pkt.RpcID)
	binary.BigEndian.PutUint32(buf[7:11], uint32(payloadLen))
	copy(buf[HeaderSize:], pkt.Payload)
	return buf, nil
}

// Decode 从 reader 中读取并解码一个 Packet
func Decode(r io.Reader) (*Packet, error) {
	if r == nil {
		return nil, ErrReaderRequired
	}
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	payloadLen := binary.BigEndian.Uint32(header[7:11])
	if payloadLen > MaxPayloadSize {
		return nil, ErrPacketTooLarge
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	return &Packet{
		Type:    PacketType(header[0]),
		MsgID:   binary.BigEndian.Uint16(header[1:3]),
		RpcID:   binary.BigEndian.Uint32(header[3:7]),
		Payload: payload,
	}, nil
}
