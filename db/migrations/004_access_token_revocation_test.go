package migrations

import (
	"context"
	"errors"
	"testing"
)

func TestEnsureAccessTokenRevocationIndexRequiresClient(t *testing.T) {
	if err := EnsureAccessTokenRevocationIndex(context.Background(), nil); !errors.Is(err, ErrClientRequired) {
		t.Fatalf("EnsureAccessTokenRevocationIndex error = %v, want %v", err, ErrClientRequired)
	}
}
