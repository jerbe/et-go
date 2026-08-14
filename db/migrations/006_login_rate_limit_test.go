package migrations

import (
	"context"
	"errors"
	"testing"
)

func TestEnsureLoginRateLimitBucketIndexRequiresClient(t *testing.T) {
	if err := EnsureLoginRateLimitBucketIndex(context.Background(), nil); !errors.Is(err, ErrClientRequired) {
		t.Fatalf("EnsureLoginRateLimitBucketIndex error = %v, want %v", err, ErrClientRequired)
	}
}
