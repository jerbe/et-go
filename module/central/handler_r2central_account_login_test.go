package central

import (
	"context"
	"errors"
	"testing"

	"github.com/jerbe/et-go/module/login"
)

type stubAccountStore struct {
	account    *CAccount
	err        error
	updateErr  error
	updateHash string
	updateAlgo string
}

func (s *stubAccountStore) FindByUsername(_ context.Context, _ string) (*CAccount, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.account, nil
}

func (s *stubAccountStore) UpdatePassword(_ context.Context, _ int64, passwordHash string, algorithm string) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updateHash = passwordHash
	s.updateAlgo = algorithm
	if s.account != nil {
		s.account.PasswordHash = passwordHash
		s.account.PasswordAlgorithm = algorithm
	}
	return nil
}

func TestHandleAccountLoginWithStore(t *testing.T) {
	passwordHash, err := HashPassword("pass")
	if err != nil {
		t.Fatalf("HashPassword err = %v", err)
	}
	success, err := HandleAccountLoginWithStore(&stubAccountStore{
		account: &CAccount{
			Id:                1001,
			PasswordHash:      passwordHash,
			PasswordAlgorithm: PasswordAlgorithmArgon2id,
		},
	}, &R2CentralAccountLogin{
		RpcId:    1,
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("success err = %v", err)
	}
	if success.AccessToken == "" {
		t.Fatal("expected access token")
	}
	accountID, err := login.VerifyAccessToken(success.AccessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken err = %v", err)
	}
	if accountID != 1001 {
		t.Fatalf("accountID = %d, want 1001", accountID)
	}

	invalid, err := HandleAccountLoginWithStore(&stubAccountStore{}, &R2CentralAccountLogin{RpcId: 2})
	if err != nil {
		t.Fatalf("invalid err = %v", err)
	}
	if invalid.Error != ERR_UsernameOrPasswordIncorrectError {
		t.Fatalf("invalid error = %d", invalid.Error)
	}

	boom := errors.New("boom")
	if _, err := HandleAccountLoginWithStore(&stubAccountStore{err: boom}, &R2CentralAccountLogin{RpcId: 3}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestHandleAccountLoginUpgradesLegacyMD5(t *testing.T) {
	store := &stubAccountStore{
		account: &CAccount{
			Id:           1002,
			PasswordHash: MD5Hash("pass"),
		},
	}
	if _, err := HandleAccountLoginWithStore(store, &R2CentralAccountLogin{
		RpcId:    4,
		Username: "legacy",
		Password: "pass",
	}); err != nil {
		t.Fatalf("legacy login err = %v", err)
	}
	if store.updateAlgo != PasswordAlgorithmArgon2id || store.updateHash == "" {
		t.Fatalf("legacy password was not upgraded: algo=%q hash=%q", store.updateAlgo, store.updateHash)
	}
	valid, needsUpgrade, err := VerifyPassword("pass", store.updateHash, store.updateAlgo)
	if err != nil || !valid || needsUpgrade {
		t.Fatalf("upgraded password valid=%v upgrade=%v err=%v", valid, needsUpgrade, err)
	}
}
