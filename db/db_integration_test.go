//go:build integration

package db

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestDBComponentCRUDAndIncrementIntegration(t *testing.T) {
	uri := os.Getenv("ETGO_MONGO_URI")
	if uri == "" {
		t.Skip("ETGO_MONGO_URI not set")
	}

	dbName := "etgo_test"
	client, err := New(context.Background(), uri, dbName)
	if err != nil {
		t.Fatalf("New client error = %v", err)
	}
	defer client.Close(context.Background())

	component := NewDBComponent(client, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = client.Collection("account").Drop(ctx)
	_ = client.Collection("increment").Drop(ctx)

	account := &CAccount{Id: 1001, Username: "user", PasswordHash: "hash"}
	if err := component.Insert(ctx, account, account.CollectionName()); err != nil {
		t.Fatalf("Insert error = %v", err)
	}

	var found CAccount
	if err := component.FindOne(ctx, account.Id, account.CollectionName(), &found); err != nil {
		t.Fatalf("FindOne error = %v", err)
	}
	if found.Username != account.Username {
		t.Fatalf("found username = %q, want %q", found.Username, account.Username)
	}

	var accounts []CAccount
	if err := component.Query(ctx, bson.M{"username": account.Username}, account.CollectionName(), &accounts); err != nil {
		t.Fatalf("Query error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("Query count = %d, want 1", len(accounts))
	}

	account.PasswordHash = "hash2"
	if err := component.Save(ctx, account.Id, account, account.CollectionName()); err != nil {
		t.Fatalf("Save error = %v", err)
	}

	value, err := component.Increment(ctx, "account_id", 1, 100)
	if err != nil {
		t.Fatalf("Increment error = %v", err)
	}
	if value != 101 {
		t.Fatalf("Increment value = %d, want 101", value)
	}

	deleted, err := component.Remove(ctx, account.Id, account.CollectionName())
	if err != nil {
		t.Fatalf("Remove error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}
