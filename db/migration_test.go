package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidateMigrations(t *testing.T) {
	valid := []Migration{
		{Version: 2, Name: "second", Up: func(context.Context, *Client) error { return nil }},
		{Version: 1, Name: "first", Up: func(context.Context, *Client) error { return nil }},
	}
	if err := ValidateMigrations(valid); err != nil {
		t.Fatalf("ValidateMigrations valid error = %v", err)
	}

	cases := [][]Migration{
		{{Version: 0, Name: "invalid", Up: func(context.Context, *Client) error { return nil }}},
		{{Version: 1, Name: "", Up: func(context.Context, *Client) error { return nil }}},
		{{Version: 1, Name: "missing-up"}},
		{
			{Version: 1, Name: "one", Up: func(context.Context, *Client) error { return nil }},
			{Version: 1, Name: "duplicate", Up: func(context.Context, *Client) error { return nil }},
		},
	}
	for _, migrations := range cases {
		if err := ValidateMigrations(migrations); !errors.Is(err, ErrMigrationInvalid) {
			t.Fatalf("ValidateMigrations(%#v) = %v, want ErrMigrationInvalid", migrations, err)
		}
	}
}

func TestRunMigrationsRequiresExplicitDependencies(t *testing.T) {
	migrations := []Migration{{Version: 1, Name: "one", Up: func(context.Context, *Client) error { return nil }}}
	if err := RunMigrations(nil, nil, migrations); !errors.Is(err, ErrMigrationContextRequired) {
		t.Fatalf("RunMigrations nil context = %v, want ErrMigrationContextRequired", err)
	}
	if err := RunMigrations(context.Background(), nil, migrations); !errors.Is(err, ErrMigrationClientRequired) {
		t.Fatalf("RunMigrations nil client = %v, want ErrMigrationClientRequired", err)
	}
}

func TestNormalizeMigrationOptions(t *testing.T) {
	options, err := normalizeMigrationOptions(MigrationOptions{
		Owner:         " owner ",
		LeaseDuration: time.Second,
		PollInterval:  100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("normalizeMigrationOptions error = %v", err)
	}
	if options.Owner != "owner" {
		t.Fatalf("owner = %q, want owner", options.Owner)
	}
	if options.LeaseDuration != time.Second || options.PollInterval != 100*time.Millisecond {
		t.Fatalf("normalized timing = %+v", options)
	}

	if _, err := normalizeMigrationOptions(MigrationOptions{
		LeaseDuration: time.Second,
		PollInterval:  time.Second,
	}); !errors.Is(err, ErrMigrationOptionsInvalid) {
		t.Fatalf("invalid timing error = %v, want ErrMigrationOptionsInvalid", err)
	}
}

func TestNormalizeMigrationOptionsGeneratesOwner(t *testing.T) {
	options, err := normalizeMigrationOptions(MigrationOptions{})
	if err != nil {
		t.Fatalf("normalize default options error = %v", err)
	}
	if options.Owner == "" {
		t.Fatal("generated migration owner is empty")
	}
	if options.LeaseDuration <= 0 || options.PollInterval <= 0 {
		t.Fatalf("generated timing is invalid: %+v", options)
	}
}

func TestAllMigrationsDefinitionIsValid(t *testing.T) {
	if err := ValidateMigrations(migrationsForTest()); err != nil {
		t.Fatalf("migration definition invalid: %v", err)
	}
}

func migrationsForTest() []Migration {
	return []Migration{
		{Version: 1, Name: "test", Up: func(context.Context, *Client) error { return nil }},
	}
}
