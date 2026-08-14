package login

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tokenVersion        = "v2"
	tokenExpireDuration = 7 * 24 * time.Hour
	legacyTokenKey      = "whosyourdaddy"
)

// AccessTokenKey 是一个可轮换的 HMAC Token 密钥。
type AccessTokenKey struct {
	ID     string
	Secret string
}

// AccessTokenConfig 是进程级 AccessToken 配置。
//
// Keys 可以同时包含 current 和 previous key；生成只使用 CurrentKeyID，
// 验证允许使用所有 Keys。LegacyKey 只用于显式兼容旧 XOR Token。
type AccessTokenConfig struct {
	CurrentKeyID   string
	Keys           []AccessTokenKey
	LegacyKey      string
	AllowLegacy    bool
	GenerateLegacy bool
	ExpireDuration time.Duration
	// RevocationStore 是撤销状态存储。需要强制撤销时同时设置
	// RequireRevocation=true；不提供持久化实现不会被伪装成跨进程安全。
	RevocationStore   AccessTokenRevocationStore
	RequireRevocation bool
}

// AccessTokenClaims 是经过签名或显式 legacy 校验后的 Token 声明。
type AccessTokenClaims struct {
	AccountID int64
	IssuedAt  time.Time
	ExpiresAt time.Time
	TokenID   string
	KeyID     string
	Legacy    bool
}

// AccessTokenRevocation 是撤销存储使用的稳定 Token 标识。
type AccessTokenRevocation struct {
	TokenID   string
	AccountID int64
	ExpiresAt time.Time
}

// AccessTokenRevocationStore 定义撤销状态的持久化边界。
//
// 实现可以使用 MongoDB、Redis 或其他共享存储。接口故意不接受明文 Token，
// 调用方只传 SHA-256 TokenID 和过期时间。
type AccessTokenRevocationStore interface {
	IsRevoked(context.Context, AccessTokenRevocation) (bool, error)
	Revoke(context.Context, AccessTokenRevocation) error
}

type accessTokenManager struct {
	currentKeyID      string
	keys              map[string][]byte
	legacyKey         string
	allowLegacy       bool
	generateLegacy    bool
	expireDuration    time.Duration
	revocationStore   AccessTokenRevocationStore
	requireRevocation bool
}

type signedAccessTokenPayload struct {
	AccountID int64  `json:"account_id"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
	Nonce     uint64 `json:"nonce"`
}

var (
	tokenConfigMu sync.RWMutex
	tokenManager  = &accessTokenManager{}

	tokenNowFunc  = time.Now
	tokenRandU64  = func() uint64 { return mathrand.Uint64() }
	tokenRandIntn = func(n int) int { return mathrand.Intn(n) }
)

// ConfigureAccessToken 配置当前进程的签名 Token key ring。
func ConfigureAccessToken(config AccessTokenConfig) error {
	currentID := strings.TrimSpace(config.CurrentKeyID)
	if !config.GenerateLegacy && currentID == "" {
		return ErrTokenConfigInvalid
	}
	if currentID != "" && strings.Contains(currentID, ".") {
		return ErrTokenConfigInvalid
	}
	if !config.GenerateLegacy && len(config.Keys) == 0 {
		return ErrTokenConfigInvalid
	}
	keys := make(map[string][]byte, len(config.Keys))
	for _, key := range config.Keys {
		keyID := strings.TrimSpace(key.ID)
		if keyID == "" || strings.Contains(keyID, ".") || len([]byte(key.Secret)) < 32 {
			return ErrTokenConfigInvalid
		}
		if _, exists := keys[keyID]; exists {
			return ErrTokenConfigInvalid
		}
		keys[keyID] = []byte(key.Secret)
	}
	if currentID != "" {
		if _, exists := keys[currentID]; !exists {
			return ErrTokenConfigInvalid
		}
	}
	if config.GenerateLegacy && (!config.AllowLegacy || strings.TrimSpace(config.LegacyKey) == "") {
		return ErrTokenConfigInvalid
	}
	legacyKey := strings.TrimSpace(config.LegacyKey)
	if config.AllowLegacy && legacyKey == "" {
		return ErrTokenConfigInvalid
	}
	if config.RequireRevocation && config.RevocationStore == nil {
		return ErrTokenRevocationStoreRequired
	}
	expireDuration := config.ExpireDuration
	if expireDuration <= 0 {
		expireDuration = tokenExpireDuration
	}

	tokenConfigMu.Lock()
	tokenManager = &accessTokenManager{
		currentKeyID:      currentID,
		keys:              keys,
		legacyKey:         legacyKey,
		allowLegacy:       config.AllowLegacy,
		generateLegacy:    config.GenerateLegacy,
		expireDuration:    expireDuration,
		revocationStore:   config.RevocationStore,
		requireRevocation: config.RequireRevocation,
	}
	tokenConfigMu.Unlock()
	return nil
}

// ConfigureLegacyAccessTokenForTests 恢复历史 Token 生成器。
//
// 生产启动不应调用此函数；它只让未经过 cmd/server 配置的进程内兼容测试
// 能继续验证旧协议。
func ConfigureLegacyAccessTokenForTests() {
	tokenConfigMu.Lock()
	tokenManager = &accessTokenManager{
		legacyKey:      legacyTokenKey,
		allowLegacy:    true,
		generateLegacy: true,
		expireDuration: tokenExpireDuration,
	}
	tokenConfigMu.Unlock()
}

func currentTokenManager() *accessTokenManager {
	tokenConfigMu.RLock()
	defer tokenConfigMu.RUnlock()
	manager := *tokenManager
	if tokenManager.keys != nil {
		manager.keys = make(map[string][]byte, len(tokenManager.keys))
		for key, secret := range tokenManager.keys {
			manager.keys[key] = append([]byte(nil), secret...)
		}
	}
	return &manager
}

// GenerateAccessToken 生成当前配置版本的 AccessToken。
//
// 未安装配置时直接返回 ErrTokenConfigRequired。旧 XOR 生成只有通过
// AccessTokenConfig.GenerateLegacy 或测试初始化等显式 legacy 配置后才会启用。
func GenerateAccessToken(accountID int64) (string, error) {
	if accountID <= 0 {
		return "", ErrAccountIDRequired
	}
	manager := currentTokenManager()
	if manager.generateLegacy {
		return generateLegacyAccessToken(manager, accountID)
	}
	if manager.currentKeyID == "" {
		return "", ErrTokenConfigRequired
	}
	return manager.generateSigned(accountID)
}

func generateLegacyAccessToken(manager *accessTokenManager, accountID int64) (string, error) {
	now := tokenNowFunc().Unix()
	expire := now + int64(manager.expireDuration.Seconds())
	plain := fmt.Sprintf("%d|%d:%d:%d", tokenRandU64(), accountID, now, expire)
	cipher := xorBytes([]byte(plain), manager.legacyKey)
	encoded := base64.StdEncoding.EncodeToString(cipher)
	return hex.EncodeToString([]byte(encoded)), nil
}

func (m *accessTokenManager) generateSigned(accountID int64) (string, error) {
	nonce, err := secureTokenNonce()
	if err != nil {
		return "", err
	}
	issuedAt := tokenNowFunc().Unix()
	payload, err := json.Marshal(signedAccessTokenPayload{
		AccountID: accountID,
		IssuedAt:  issuedAt,
		ExpiresAt: issuedAt + int64(m.expireDuration.Seconds()),
		Nonce:     nonce,
	})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	unsigned := tokenVersion + "." + m.currentKeyID + "." + body
	signature := signToken([]byte(unsigned), m.keys[m.currentKeyID])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func secureTokenNonce() (uint64, error) {
	var raw [8]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(raw[:]), nil
}

// VerifyAccessToken 校验签名 Token，或在配置明确允许时校验旧 XOR Token。
func VerifyAccessToken(token string) (int64, error) {
	return VerifyAccessTokenContext(context.Background(), token)
}

// VerifyAccessTokenContext 在调用方 context 中校验 AccessToken。
func VerifyAccessTokenContext(ctx context.Context, token string) (int64, error) {
	if ctx == nil {
		return 0, ErrTokenContextRequired
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	manager := currentTokenManager()
	if manager.currentKeyID == "" && (!manager.allowLegacy || manager.legacyKey == "") {
		return 0, ErrTokenConfigRequired
	}
	if strings.HasPrefix(token, tokenVersion+".") {
		claims, err := manager.signedClaims(token)
		if err != nil {
			return 0, err
		}
		if err := manager.checkRevocation(ctx, claims); err != nil {
			return 0, err
		}
		return claims.AccountID, nil
	}
	if !manager.allowLegacy || manager.legacyKey == "" {
		return 0, ErrTokenInvalid
	}
	claims, err := verifyLegacyAccessToken(manager, token)
	if err != nil {
		return 0, err
	}
	if err := manager.checkRevocation(ctx, claims); err != nil {
		return 0, err
	}
	return claims.AccountID, nil
}

func (m *accessTokenManager) signedClaims(token string) (AccessTokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 4 || parts[0] != tokenVersion {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	secret, ok := m.keys[parts[1]]
	if !ok {
		return AccessTokenClaims{}, ErrTokenKeyUnknown
	}
	received, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	unsigned := strings.Join(parts[:3], ".")
	expected := signToken([]byte(unsigned), secret)
	if !hmac.Equal(received, expected) {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	var payload signedAccessTokenPayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	if payload.AccountID <= 0 || payload.IssuedAt <= 0 || payload.ExpiresAt <= payload.IssuedAt ||
		payload.Nonce == 0 {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	if tokenNowFunc().Unix() >= payload.ExpiresAt {
		return AccessTokenClaims{}, ErrTokenExpired
	}
	return AccessTokenClaims{
		AccountID: payload.AccountID,
		IssuedAt:  time.Unix(payload.IssuedAt, 0),
		ExpiresAt: time.Unix(payload.ExpiresAt, 0),
		TokenID:   accessTokenID(token),
		KeyID:     parts[1],
	}, nil
}

func verifyLegacyAccessToken(manager *accessTokenManager, token string) (AccessTokenClaims, error) {
	raw, err := hex.DecodeString(token)
	if err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	cipher, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	plain := string(xorBytes(cipher, manager.legacyKey))
	parts := strings.SplitN(plain, "|", 2)
	if len(parts) != 2 {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	payload := strings.Split(parts[1], ":")
	if len(payload) != 3 {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	accountID, err := strconv.ParseInt(payload[0], 10, 64)
	if err != nil || accountID <= 0 {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	issuedAt, err := strconv.ParseInt(payload[1], 10, 64)
	if err != nil || issuedAt <= 0 {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	expireAt, err := strconv.ParseInt(payload[2], 10, 64)
	if err != nil {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	if expireAt <= issuedAt {
		return AccessTokenClaims{}, ErrTokenInvalid
	}
	if tokenNowFunc().Unix() >= expireAt {
		return AccessTokenClaims{}, ErrTokenExpired
	}
	return AccessTokenClaims{
		AccountID: accountID,
		IssuedAt:  time.Unix(issuedAt, 0),
		ExpiresAt: time.Unix(expireAt, 0),
		TokenID:   accessTokenID(token),
		KeyID:     "legacy",
		Legacy:    true,
	}, nil
}

func (m *accessTokenManager) checkRevocation(ctx context.Context, claims AccessTokenClaims) error {
	if m == nil {
		return ErrTokenConfigRequired
	}
	if m.revocationStore == nil {
		if m.requireRevocation {
			return ErrTokenRevocationStoreRequired
		}
		return nil
	}
	revoked, err := m.revocationStore.IsRevoked(ctx, AccessTokenRevocation{
		TokenID:   claims.TokenID,
		AccountID: claims.AccountID,
		ExpiresAt: claims.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTokenRevocationStoreUnavailable, err)
	}
	if revoked {
		return ErrTokenRevoked
	}
	return nil
}

// RevokeAccessToken 将一个当前有效的 AccessToken 写入撤销存储。
func RevokeAccessToken(ctx context.Context, token string) error {
	if ctx == nil {
		return ErrTokenContextRequired
	}
	manager := currentTokenManager()
	if manager.revocationStore == nil {
		return ErrTokenRevocationStoreRequired
	}
	var claims AccessTokenClaims
	var err error
	if strings.HasPrefix(token, tokenVersion+".") {
		claims, err = manager.signedClaims(token)
	} else if manager.allowLegacy && manager.legacyKey != "" {
		claims, err = verifyLegacyAccessToken(manager, token)
	} else {
		return ErrTokenInvalid
	}
	if err != nil {
		return err
	}
	if err := manager.revocationStore.Revoke(ctx, AccessTokenRevocation{
		TokenID:   claims.TokenID,
		AccountID: claims.AccountID,
		ExpiresAt: claims.ExpiresAt,
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrTokenRevocationStoreUnavailable, err)
	}
	return nil
}

func accessTokenID(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func signToken(data, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

// GenerateGateToken 生成 Gate 登录 Token。
func GenerateGateToken(accountID int64) string {
	token, err := generateGateToken(accountID)
	if err != nil {
		return ""
	}
	return token
}

func generateGateToken(accountID int64) (string, error) {
	if accountID <= 0 {
		return "", ErrAccountIDRequired
	}
	first, err := cryptorand.Int(cryptorand.Reader, big.NewInt(90000))
	if err != nil {
		return "", fmt.Errorf("%w: random prefix: %v", ErrGateTokenGeneration, err)
	}
	second, err := cryptorand.Int(cryptorand.Reader, big.NewInt(9999))
	if err != nil {
		return "", fmt.Errorf("%w: random suffix: %v", ErrGateTokenGeneration, err)
	}
	plain := fmt.Sprintf("%05d.%d.%04d", first.Int64()+10000, accountID, second.Int64()+1)
	return hex.EncodeToString([]byte(plain)), nil
}

// ParseGateToken 解析 Gate 登录 Token。
func ParseGateToken(token string) (int64, error) {
	raw, err := hex.DecodeString(token)
	if err != nil {
		return 0, ErrConnectGateKey
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		return 0, ErrConnectGateKey
	}
	accountID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || accountID <= 0 {
		return 0, ErrConnectGateKey
	}
	return accountID, nil
}

func xorBytes(data []byte, key string) []byte {
	if len(key) == 0 {
		return append([]byte(nil), data...)
	}
	result := make([]byte, len(data))
	for index := range data {
		result[index] = data[index] ^ key[index%len(key)]
	}
	return result
}
