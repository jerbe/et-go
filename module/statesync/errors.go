package statesync

import "errors"

var (
	// ErrUnitMissing 表示目标单位不存在。
	ErrUnitMissing = errors.New("statesync: unit missing")
	// ErrUnsupportedMessage 表示状态同步消息类型未注册。
	ErrUnsupportedMessage = errors.New("statesync: unsupported message")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("statesync: message is nil")
	// ErrSceneMissing 表示状态同步缺少所属 Scene。
	ErrSceneMissing = errors.New("statesync: scene missing")
	// ErrPlayerIDInvalid 表示状态同步目标玩家 ID 非法。
	ErrPlayerIDInvalid = errors.New("statesync: invalid player id")
	// ErrMessageSenderMissing 表示状态同步缺少位置消息发送器。
	ErrMessageSenderMissing = errors.New("statesync: message sender missing")
	// ErrMessageIDMissing 表示业务消息没有注册协议编号。
	ErrMessageIDMissing = errors.New("statesync: message id missing")
	// ErrMessagePayloadMissing 表示业务消息编码结果为空。
	ErrMessagePayloadMissing = errors.New("statesync: message payload missing")
)
