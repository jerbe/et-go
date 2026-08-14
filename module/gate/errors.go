package gate

import "errors"

var (
	// ErrGateSessionMissing 表示 GateSession 绑定不存在。
	ErrGateSessionMissing = errors.New("gate: gate session component missing")
	// ErrSessionNil 表示 GateSession 绑定缺少实际连接。
	ErrSessionNil = errors.New("gate: session is nil")
	// ErrNotLoggedIn 表示连接尚未完成登录。
	ErrNotLoggedIn = errors.New("gate: session not logged in")
	// ErrLocationSenderMissing 表示缺少定位消息发送器。
	ErrLocationSenderMissing = errors.New("gate: location sender component missing")
	// ErrMessageHandlerMissing 表示收到的消息没有注册处理器。
	ErrMessageHandlerMissing = errors.New("gate: message handler missing")
	// ErrSceneMissing 表示 Gate 消息处理缺少所属 Scene。
	ErrSceneMissing = errors.New("gate: scene missing")
)
