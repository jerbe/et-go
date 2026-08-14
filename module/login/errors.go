package login

import "errors"

const (
	// ERR_ConnectGateKeyError 表示 Gate Token 无效。
	ERR_ConnectGateKeyError int32 = 100009001
	// ERR_UsernameOrPasswordIncorrectError 表示用户名或密码错误。
	ERR_UsernameOrPasswordIncorrectError int32 = 100009002
	// ERR_TokenInvalidError 表示 AccessToken 无效。
	ERR_TokenInvalidError int32 = 100009003
	// ERR_TokenExpiredError 表示 AccessToken 已过期。
	ERR_TokenExpiredError int32 = 100009004
)

var (
	// ErrConnectGateKey 表示 Gate Token 无效。
	ErrConnectGateKey = errors.New("login: invalid gate token")
	// ErrUsernameOrPasswordIncorrect 表示用户名或密码错误。
	ErrUsernameOrPasswordIncorrect = errors.New("Username or password is incorrect")
	// ErrTokenInvalid 表示 AccessToken 无效。
	ErrTokenInvalid = errors.New("login: token invalid")
	// ErrTokenExpired 表示 AccessToken 已过期。
	ErrTokenExpired = errors.New("login: token expired")
	// ErrTokenConfigInvalid 表示签名 Token 配置非法。
	ErrTokenConfigInvalid = errors.New("login: token config invalid")
	// ErrTokenConfigRequired 表示生成或校验 Token 前尚未安装安全配置。
	ErrTokenConfigRequired = errors.New("login: token config required")
	// ErrTokenKeyUnknown 表示签名 Token 使用了未配置的 key id。
	ErrTokenKeyUnknown = errors.New("login: token key unknown")
	// ErrTokenRevoked 表示 AccessToken 已被撤销。
	ErrTokenRevoked = errors.New("login: token revoked")
	// ErrTokenRevocationStoreRequired 表示撤销操作缺少存储实现。
	ErrTokenRevocationStoreRequired = errors.New("login: token revocation store required")
	// ErrTokenRevocationStoreUnavailable 表示撤销存储访问失败。
	ErrTokenRevocationStoreUnavailable = errors.New("login: token revocation store unavailable")
	// ErrTokenContextRequired 表示 Token 操作缺少 context。
	ErrTokenContextRequired = errors.New("login: token context required")
	// ErrGateRegistryMissing 表示 Realm 缺少 Gate 列表。
	ErrGateRegistryMissing = errors.New("login: gate registry missing")
	// ErrMessageSenderMissing 表示场景缺少消息发送器。
	ErrMessageSenderMissing = errors.New("login: message sender missing")
	// ErrInvalidLoginRequest 表示登录请求缺少必要字段。
	ErrInvalidLoginRequest = errors.New("login: invalid login request")
	// ErrAccountIDRequired 表示生成 Token 时缺少有效账号 ID。
	ErrAccountIDRequired = errors.New("login: account id required")
	// ErrGateTokenGeneration 表示 Gate 一次性 Token 生成失败。
	ErrGateTokenGeneration = errors.New("login: gate token generation failed")
	// ErrCentralActorMissing 表示 Gate 所属 Zone 没有 Central。
	ErrCentralActorMissing = errors.New("login: central actor missing")
	// ErrLocationProxyMissing 表示 Gate 缺少 Location 代理。
	ErrLocationProxyMissing = errors.New("login: location proxy missing")
	// ErrMapActorMissing 表示当前 Zone 没有 Home Map。
	ErrMapActorMissing = errors.New("login: home map actor missing")
	// ErrGateIDMismatch 表示客户端连接到了错误的 Gate。
	ErrGateIDMismatch = errors.New("login: gate id mismatch")
	// ErrLocationRegistration 表示登录实体 Location 注册失败。
	ErrLocationRegistration = errors.New("login: location registration failed")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("login: message is nil")
	// ErrMessageHandlerMissing 表示 Realm 收到未注册的网络消息。
	ErrMessageHandlerMissing = errors.New("login: message handler missing")
	// ErrGateSessionKeyClosed 表示 Gate Token 组件已经销毁。
	ErrGateSessionKeyClosed = errors.New("login: gate session key component closed")
	// ErrGateSessionKeyTimerMissing 表示 Gate Token 缺少有效过期定时器。
	ErrGateSessionKeyTimerMissing = errors.New("login: gate session key timer missing")
	// ErrGateAssignmentInvalid 表示 Gate 返回了不可用的 Token 分配结果。
	ErrGateAssignmentInvalid = errors.New("login: invalid gate assignment")
	// ErrPlayerComponentClosed 表示 Gate 玩家注册表已经销毁。
	ErrPlayerComponentClosed = errors.New("login: player component closed")
	// ErrPlayerAlreadyExists 表示账号已经绑定其他玩家实体。
	ErrPlayerAlreadyExists = errors.New("login: player already exists")
)
