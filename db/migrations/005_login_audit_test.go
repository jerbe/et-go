package migrations

import (
	"context"
	"errors"
	"testing"
)

func TestEnsureLoginAuditIndexRequiresClient(t *testing.T) {
	if err := EnsureLoginAuditIndex(context.Background(), nil); !errors.Is(err, ErrClientRequired) {
		t.Fatalf("EnsureLoginAuditIndex error = %v, want %v", err, ErrClientRequired)
	}
}
