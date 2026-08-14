package central

import "errors"

const (
	// ERR_UsernameOrPasswordIncorrectError 表示用户名或密码错误。
	ERR_UsernameOrPasswordIncorrectError int32 = 100009002
)

var (
	// ErrUsernameOrPasswordIncorrect 表示用户名或密码错误。
	ErrUsernameOrPasswordIncorrect = errors.New("Username or password is incorrect")
	// ErrAccountStoreMissing 表示场景缺少账号查询依赖。
	ErrAccountStoreMissing = errors.New("central: account store missing")
	// ErrAccountDuplicate 表示同一用户名存在多个账号文档。
	ErrAccountDuplicate = errors.New("central: duplicate account")
	// ErrPasswordHashInvalid 表示账号密码哈希格式损坏。
	ErrPasswordHashInvalid = errors.New("central: invalid password hash")
	// ErrPasswordAlgorithmUnsupported 表示账号密码算法未实现。
	ErrPasswordAlgorithmUnsupported = errors.New("central: unsupported password algorithm")
	// ErrPasswordUpgradeFailed 表示旧密码哈希验证成功但升级失败。
	ErrPasswordUpgradeFailed = errors.New("central: password upgrade failed")
	// ErrSceneMissing 表示业务请求缺少目标 Scene。
	ErrSceneMissing = errors.New("central: scene missing")
	// ErrPlayerProfileStoreMissing 表示玩家档案持久化依赖未配置。
	ErrPlayerProfileStoreMissing = errors.New("central: player profile store missing")
	// ErrPlayerProfileInvalid 表示玩家档案数据非法。
	ErrPlayerProfileInvalid = errors.New("central: invalid player profile")
	// ErrPlayerProfileDuplicate 表示数据库中存在多个同一账号/Zone 档案。
	ErrPlayerProfileDuplicate = errors.New("central: duplicate player profile")
	// ErrInvalidAccountLoginRequest 表示账号登录请求缺少必要字段。
	ErrInvalidAccountLoginRequest = errors.New("central: invalid account login request")
	// ErrMessageNil 表示协议编码器收到 nil 业务消息。
	ErrMessageNil = errors.New("central: message is nil")
)
