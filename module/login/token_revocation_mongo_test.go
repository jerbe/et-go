package login

import (
	"context"
	"errors"
	"testing"
)

func TestNewMongoAccessTokenRevocationStoreRequiresDatabase(t *testing.T) {
	store, err := NewMongoAccessTokenRevocationStore(nil)
	if store != nil {
		t.Fatalf("store = %#v, want nil", store)
	}
	if !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
}

func TestMongoAccessTokenRevocationStoreRejectsInvalidOperations(t *testing.T) {
	store := &MongoAccessTokenRevocationStore{}
	token := AccessTokenRevocation{
		TokenID:   "token",
		AccountID: 1,
	}
	if _, err := store.IsRevoked(context.Background(), token); !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("IsRevoked error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
	if err := store.Revoke(context.Background(), token); !errors.Is(err, ErrTokenRevocationStoreUnavailable) {
		t.Fatalf("Revoke error = %v, want %v", err, ErrTokenRevocationStoreUnavailable)
	}
}
