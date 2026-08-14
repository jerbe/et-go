//go:build integration

package db

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestRunMigrationsSerializesOwnersIntegration(t *testing.T) {
	uri := os.Getenv("ETGO_MONGO_URI")
	if uri == "" {
		t.Skip("ETGO_MONGO_URI not set")
	}

	clientA, err := New(context.Background(), uri, "etgo_migration_lock_test")
	if err != nil {
		t.Fatalf("New client A error = %v", err)
	}
	defer clientA.Close(context.Background())
	clientB, err := New(context.Background(), uri, "etgo_migration_lock_test")
	if err != nil {
		t.Fatalf("New client B error = %v", err)
	}
	defer clientB.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = clientA.Collection(migrationCollectionName).Drop(ctx)
	_ = clientA.Collection(migrationLockCollectionName).Drop(ctx)

	var upCount atomic.Int32
	migrations := []Migration{{
		Version: 1,
		Name:    "serialized_test_migration",
		Up: func(ctx context.Context, client *Client) error {
			upCount.Add(1)
			select {
			case <-time.After(150 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}}
	options := MigrationOptions{
		LeaseDuration: 2 * time.Second,
		PollInterval:  20 * time.Millisecond,
	}

	var wg sync.WaitGroup
	errorsCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errorsCh <- RunMigrationsWithOptions(ctx, clientA, migrations, options)
	}()
	go func() {
		defer wg.Done()
		errorsCh <- RunMigrationsWithOptions(ctx, clientB, migrations, options)
	}()
	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		if err != nil {
			t.Fatalf("RunMigrationsWithOptions error = %v", err)
		}
	}
	if upCount.Load() != 1 {
		t.Fatalf("migration Up count = %d, want 1", upCount.Load())
	}

	var applied appliedMigration
	if err := clientA.Collection(migrationCollectionName).FindOne(ctx, bson.M{"_id": migrations[0].Version}).Decode(&applied); err != nil {
		t.Fatalf("query applied migrations error = %v", err)
	}
	count, err := clientA.Collection(migrationCollectionName).CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("count applied migrations error = %v", err)
	}
	if count != 1 || applied.Name != migrations[0].Name {
		t.Fatalf("applied migrations count=%d value=%+v", count, applied)
	}
}
