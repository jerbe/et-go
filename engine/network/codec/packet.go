package codec

// PacketType 数据包类型
type PacketType byte

const (
	PacketTypeMessage  PacketType = 1 // 普通消息
	PacketTypeRequest  PacketType = 2 // RPC 请求
	PacketTypeResponse PacketType = 3 // RPC 响应
)

// Packet 网络数据包。
// 格式：[PacketType(1) | MsgID(2) | RpcID(4) | PayloadLen(4) | Payload(...)]
type Packet struct {
	Type    PacketType
	MsgID   uint16
	RpcID   uint32
	Payload []byte
}

// HeaderSize 包头固定长度
const HeaderSize = 1 + 2 + 4 + 4 // Type + MsgID + RpcID + PayloadLen = 11 bytes
