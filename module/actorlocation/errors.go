package actorlocation

import (
	"errors"
	"fmt"
)

var (
	// ErrProxyCallerRequired 表示 LocationProxy 未配置 RPC 调用器。
	ErrProxyCallerRequired = errors.New("actorlocation: rpc caller required")
	// ErrLocationProxyRequired 表示位置消息发送器未配置 LocationProxy。
	ErrLocationProxyRequired = errors.New("actorlocation: location proxy required")
	// ErrMessageSenderRequired 表示位置消息发送器未配置底层消息发送器。
	ErrMessageSenderRequired = errors.New("actorlocation: message sender required")
	// ErrLocationActorRequired 表示 LocationProxy 未配置位置 Actor。
	ErrLocationActorRequired = errors.New("actorlocation: location actor required")
	// ErrLocationClosed 表示位置管理器已经关闭。
	ErrLocationClosed = errors.New("actorlocation: location manager closed")
	// ErrZeroLocationKey 表示位置查询 key 无效。
	ErrZeroLocationKey = errors.New("actorlocation: location key is zero")
	// ErrLocationLocked 表示位置当前被显式锁定，调用方可以等待后重试。
	ErrLocationLocked = errors.New("actorlocation: location is locked")
	// ErrLocationNotFound 表示位置 key 尚未注册。
	ErrLocationNotFound = errors.New("actorlocation: location not found")
	// ErrInvalidActorID 表示位置记录使用了无效 ActorID。
	ErrInvalidActorID = errors.New("actorlocation: invalid actor id")
	// ErrUnlockFailed 表示位置解锁或 ownership 切换失败。
	ErrUnlockFailed = errors.New("actorlocation: unlock failed")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("actorlocation: message is nil")
	// ErrMessageTypeInvalid 表示协议编码器收到与消息编号不匹配的业务类型。
	ErrMessageTypeInvalid = errors.New("actorlocation: message type invalid")
)

const (
	// ErrorCodeLocationLocked 是 Location 查询锁竞争的稳定错误码。
	ErrorCodeLocationLocked int32 = 1001
	// ErrorCodeLocationNotFound 是 Location 查询缺失记录的稳定错误码。
	ErrorCodeLocationNotFound int32 = 1002
)

// ResponseError 表示位置服务返回的业务错误。
type ResponseError struct {
	Code    int32
	Message string
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("actorlocation: response error %d", e.Code)
}

// Is 支持按业务错误码匹配位置错误。
func (e *ResponseError) Is(target error) bool {
	if target == ErrLocationLocked {
		return e != nil && e.Code == ErrorCodeLocationLocked
	}
	if target == ErrLocationNotFound {
		return e != nil && e.Code == ErrorCodeLocationNotFound
	}
	return false
}
