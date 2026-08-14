package db

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/jerbe/et-go/config"
)

func TestCollectionsMetadata(t *testing.T) {
	account := &CAccount{Id: 1}
	if account.CollectionName() != "account" || account.GetID() != 1 {
		t.Fatal("CAccount metadata mismatch")
	}

	profile := &CPlayerProfile{Id: 2}
	if profile.CollectionName() != "player_profile" || profile.GetID() != 2 {
		t.Fatal("CPlayerProfile metadata mismatch")
	}

	hero := &CHero{Id: 3}
	if hero.CollectionName() != "hero" || hero.GetID() != 3 {
		t.Fatal("CHero metadata mismatch")
	}
}

func TestExtractID(t *testing.T) {
	id, err := extractID(&CAccount{Id: 99})
	if err != nil || id != 99 {
		t.Fatalf("extractID err=%v id=%d", err, id)
	}

	if _, err := extractID(&CIncrement{Key: "x"}); err == nil {
		t.Fatal("extractID should fail for entity without int64 id")
	}
}

func TestSaveUpdateFieldsExcludesID(t *testing.T) {
	fields, err := saveUpdateFields(&CAccount{
		Id:           99,
		Username:     "user",
		PasswordHash: "hash",
	})
	if err != nil {
		t.Fatalf("saveUpdateFields error = %v", err)
	}
	if _, ok := fields["_id"]; ok {
		t.Fatal("save update fields must not contain _id")
	}
	if fields["username"] != "user" || fields["password_hash"] != "hash" {
		t.Fatalf("unexpected save fields: %#v", fields)
	}
}

func TestDBLockIDHandlesMinimumInt64(t *testing.T) {
	got := dbLockID(-1 << 63)
	if got < 0 || got >= DBTaskCount {
		t.Fatalf("dbLockID(min int64) = %d, want [0,%d)", got, DBTaskCount)
	}
}

func TestSaveRejectsIDMismatch(t *testing.T) {
	component := NewDBComponent(nil, nil)
	err := component.Save(context.Background(), 1, &CAccount{Id: 2}, (&CAccount{}).CollectionName())
	if !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("Save id mismatch error = %v, want %v", err, ErrInvalidEntity)
	}
}

func TestDBManagerComponentCaching(t *testing.T) {
	manager := &DBManagerComponent{}
	manager.Awake()

	manager.SetConfig(&config.Config{
		Zones: []config.StartZoneConfig{
			{ID: 1, DBAddr: "mongodb://example", DBName: "zone1"},
		},
	})

	var called atomic.Int32
	manager.SetClientFactory(func(ctx context.Context, uri string, dbName string) (*Client, error) {
		_ = ctx
		_ = uri
		_ = dbName
		called.Add(1)
		return &Client{}, nil
	})

	first, err := manager.GetZoneDB(1)
	if err != nil {
		t.Fatalf("GetZoneDB first error = %v", err)
	}
	second, err := manager.GetZoneDB(1)
	if err != nil {
		t.Fatalf("GetZoneDB second error = %v", err)
	}
	if first != second {
		t.Fatal("GetZoneDB should return cached instance")
	}
	if called.Load() != 1 {
		t.Fatalf("clientFactory called %d times, want 1", called.Load())
	}
}

func TestDBManagerRejectsUseAfterDestroy(t *testing.T) {
	manager := &DBManagerComponent{}
	manager.Awake()
	manager.OnDestroy()

	if _, err := manager.GetZoneDB(1); !errors.Is(err, ErrDBManagerClosed) {
		t.Fatalf("GetZoneDB error = %v, want %v", err, ErrDBManagerClosed)
	}
}
