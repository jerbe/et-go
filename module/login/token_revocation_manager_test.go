package login

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jerbe/et-go/db"
)

func TestNewDBManagerAccessTokenRevocationStoreRequiresDependencies(t *testing.T) {
	if store, err := NewDBManagerAccessTokenRevocationStore(nil, 1); store != nil ||
		!errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("nil manager result = store %#v err %v", store, err)
	}
	if store, err := NewDBManagerAccessTokenRevocationStore(nil, 0); store != nil ||
		!errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("invalid zone result = store %#v err %v", store, err)
	}
}

func TestDBManagerAccessTokenRevocationStoreFailsExplicitlyWhenManagerClosed(t *testing.T) {
	manager := &db.DBManagerComponent{}
	store, err := NewDBManagerAccessTokenRevocationStore(manager, 1)
	if err != nil {
		t.Fatalf("NewDBManagerAccessTokenRevocationStore error = %v", err)
	}
	_, err = store.IsRevoked(context.Background(), AccessTokenRevocation{
		TokenID:   "token",
		AccountID: 1,
		ExpiresAt: tokenNowFunc().Add(time.Hour),
	})
	if err == nil {
		t.Fatal("IsRevoked should fail without configured DB manager")
	}
}

func TestMemoryAccessTokenRevocationStoreRejectsInvalidQuery(t *testing.T) {
	store := NewMemoryAccessTokenRevocationStore()
	if _, err := store.IsRevoked(context.Background(), AccessTokenRevocation{
		TokenID: "token",
	}); !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("invalid IsRevoked error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
}

func TestMemoryAccessTokenRevocationStoreRejectsAccountMismatch(t *testing.T) {
	store := NewMemoryAccessTokenRevocationStore()
	token := AccessTokenRevocation{
		TokenID:   "token",
		AccountID: 1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Revoke(context.Background(), token); err != nil {
		t.Fatalf("Revoke error = %v", err)
	}
	if err := store.Revoke(context.Background(), AccessTokenRevocation{
		TokenID:   token.TokenID,
		AccountID: 2,
		ExpiresAt: token.ExpiresAt,
	}); !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("mismatched Revoke error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
	if _, err := store.IsRevoked(context.Background(), AccessTokenRevocation{
		TokenID:   token.TokenID,
		AccountID: 2,
		ExpiresAt: token.ExpiresAt,
	}); !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("mismatched IsRevoked error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
}
