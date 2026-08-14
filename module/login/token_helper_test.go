package login

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGenerateAndVerifyAccessToken(t *testing.T) {
	oldNow := tokenNowFunc
	oldRand := tokenRandU64
	tokenNowFunc = func() time.Time { return time.Unix(1700000000, 0) }
	tokenRandU64 = func() uint64 { return 12345 }
	defer func() {
		tokenNowFunc = oldNow
		tokenRandU64 = oldRand
	}()

	token, err := GenerateAccessToken(88)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	got, err := VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("VerifyAccessToken err = %v", err)
	}
	if got != 88 {
		t.Fatalf("accountID = %d, want 88", got)
	}
}

func TestVerifyAccessTokenExpiredAndInvalid(t *testing.T) {
	oldNow := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Unix(200, 0) }
	defer func() { tokenNowFunc = oldNow }()

	tokenNowFunc = func() time.Time { return time.Unix(100, 0) }
	token, err := GenerateAccessToken(99)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}
	tokenNowFunc = func() time.Time { return time.Unix(100+int64(tokenExpireDuration.Seconds())+1, 0) }
	if _, err := VerifyAccessToken(token); err != ErrTokenExpired {
		t.Fatalf("VerifyAccessToken err = %v, want ErrTokenExpired", err)
	}
	if _, err := VerifyAccessToken("bad-token"); err != ErrTokenInvalid {
		t.Fatalf("VerifyAccessToken invalid err = %v", err)
	}
}

func TestGenerateAccessTokenRejectsInvalidAccountID(t *testing.T) {
	if token, err := GenerateAccessToken(0); err != ErrAccountIDRequired || token != "" {
		t.Fatalf("GenerateAccessToken invalid account = token %q err %v", token, err)
	}
}

func TestGenerateGateTokenPreservesWireFormat(t *testing.T) {
	token, err := generateGateToken(88)
	if err != nil {
		t.Fatalf("generateGateToken error = %v", err)
	}
	if token == "" {
		t.Fatal("generateGateToken should return a non-empty token")
	}
	if accountID, err := ParseGateToken(token); err != nil || accountID != 88 {
		t.Fatalf("ParseGateToken = (%d, %v), want (88, nil)", accountID, err)
	}
}

func TestAccessTokenRequiresExplicitConfiguration(t *testing.T) {
	tokenConfigMu.Lock()
	tokenManager = &accessTokenManager{}
	tokenConfigMu.Unlock()
	defer ConfigureLegacyAccessTokenForTests()

	if _, err := GenerateAccessToken(88); !errors.Is(err, ErrTokenConfigRequired) {
		t.Fatalf("GenerateAccessToken error = %v, want %v", err, ErrTokenConfigRequired)
	}
	if _, err := VerifyAccessToken("not-configured"); !errors.Is(err, ErrTokenConfigRequired) {
		t.Fatalf("VerifyAccessToken error = %v, want %v", err, ErrTokenConfigRequired)
	}
}

func TestSignedAccessTokenAndKeyRotation(t *testing.T) {
	oldNow := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() {
		tokenNowFunc = oldNow
		ConfigureLegacyAccessTokenForTests()
	}()

	if err := ConfigureAccessToken(AccessTokenConfig{
		CurrentKeyID: "new",
		Keys: []AccessTokenKey{
			{ID: "new", Secret: "01234567890123456789012345678901"},
			{ID: "old", Secret: "abcdefghijklmnopqrstuvwxyz123456"},
		},
		ExpireDuration: time.Hour,
	}); err != nil {
		t.Fatalf("ConfigureAccessToken error = %v", err)
	}
	token, err := GenerateAccessToken(88)
	if err != nil {
		t.Fatalf("signed GenerateAccessToken error = %v", err)
	}
	if len(token) < 4 || token[:3] != "v2." {
		t.Fatalf("token = %q, want v2 prefix", token)
	}
	if accountID, err := VerifyAccessToken(token); err != nil || accountID != 88 {
		t.Fatalf("VerifyAccessToken = (%d, %v), want (88, nil)", accountID, err)
	}

	if err := ConfigureAccessToken(AccessTokenConfig{
		CurrentKeyID: "old",
		Keys: []AccessTokenKey{
			{ID: "old", Secret: "abcdefghijklmnopqrstuvwxyz123456"},
			{ID: "new", Secret: "01234567890123456789012345678901"},
		},
		ExpireDuration: time.Hour,
	}); err != nil {
		t.Fatalf("ConfigureAccessToken rotation error = %v", err)
	}
	if accountID, err := VerifyAccessToken(token); err != nil || accountID != 88 {
		t.Fatalf("rotated VerifyAccessToken = (%d, %v), want (88, nil)", accountID, err)
	}
}

func TestExplicitLegacyAccessTokenFormat(t *testing.T) {
	oldNow := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() {
		tokenNowFunc = oldNow
		ConfigureLegacyAccessTokenForTests()
	}()

	if err := ConfigureAccessToken(AccessTokenConfig{
		LegacyKey:      "whosyourdaddy",
		AllowLegacy:    true,
		GenerateLegacy: true,
		ExpireDuration: time.Hour,
	}); err != nil {
		t.Fatalf("ConfigureAccessToken legacy error = %v", err)
	}
	token, err := GenerateAccessToken(88)
	if err != nil {
		t.Fatalf("legacy GenerateAccessToken error = %v", err)
	}
	if accountID, err := VerifyAccessToken(token); err != nil || accountID != 88 {
		t.Fatalf("legacy VerifyAccessToken = (%d, %v), want (88, nil)", accountID, err)
	}
}

func TestLegacyAccessTokenRequiresExplicitCompatibility(t *testing.T) {
	oldNow := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() {
		tokenNowFunc = oldNow
		ConfigureLegacyAccessTokenForTests()
	}()

	legacyToken, err := GenerateAccessToken(99)
	if err != nil {
		t.Fatalf("legacy GenerateAccessToken error = %v", err)
	}
	if err := ConfigureAccessToken(AccessTokenConfig{
		CurrentKeyID: "primary",
		Keys: []AccessTokenKey{
			{ID: "primary", Secret: "01234567890123456789012345678901"},
		},
	}); err != nil {
		t.Fatalf("ConfigureAccessToken error = %v", err)
	}
	if _, err := VerifyAccessToken(legacyToken); err != ErrTokenInvalid {
		t.Fatalf("legacy VerifyAccessToken error = %v, want %v", err, ErrTokenInvalid)
	}
}

func TestAccessTokenRevocationStore(t *testing.T) {
	oldNow := tokenNowFunc
	tokenNowFunc = func() time.Time { return time.Unix(1700000000, 0) }
	defer func() {
		tokenNowFunc = oldNow
		ConfigureLegacyAccessTokenForTests()
	}()

	store := NewMemoryAccessTokenRevocationStore()
	if err := ConfigureAccessToken(AccessTokenConfig{
		CurrentKeyID: "primary",
		Keys: []AccessTokenKey{
			{ID: "primary", Secret: "01234567890123456789012345678901"},
		},
		RevocationStore:   store,
		RequireRevocation: true,
	}); err != nil {
		t.Fatalf("ConfigureAccessToken error = %v", err)
	}
	token, err := GenerateAccessToken(88)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}
	if accountID, err := VerifyAccessToken(token); err != nil || accountID != 88 {
		t.Fatalf("VerifyAccessToken before revoke = (%d, %v), want (88, nil)", accountID, err)
	}
	if err := RevokeAccessToken(context.Background(), token); err != nil {
		t.Fatalf("RevokeAccessToken error = %v", err)
	}
	if _, err := VerifyAccessToken(token); err != ErrTokenRevoked {
		t.Fatalf("VerifyAccessToken after revoke = %v, want %v", err, ErrTokenRevoked)
	}
}

func TestAccessTokenRequiresRevocationStoreWhenConfigured(t *testing.T) {
	defer ConfigureLegacyAccessTokenForTests()
	err := ConfigureAccessToken(AccessTokenConfig{
		CurrentKeyID: "primary",
		Keys: []AccessTokenKey{
			{ID: "primary", Secret: "01234567890123456789012345678901"},
		},
		RequireRevocation: true,
	})
	if err != ErrTokenRevocationStoreRequired {
		t.Fatalf("ConfigureAccessToken error = %v, want %v", err, ErrTokenRevocationStoreRequired)
	}
}
