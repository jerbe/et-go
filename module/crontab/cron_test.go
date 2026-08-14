package crontab

import (
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	schedule, err := parseCron("*/5 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fixed := time.Date(2026, 3, 21, 0, 10, 0, 0, time.UTC)
	if !schedule.Match(fixed) {
		t.Fatalf("schedule should match time %v", fixed)
	}
	off := time.Date(2026, 3, 21, 0, 11, 0, 0, time.UTC)
	if schedule.Match(off) {
		t.Fatalf("schedule should not match minute %v", off)
	}

	cases := []struct {
		expression string
		when       time.Time
	}{
		{"0 * * * *", time.Date(2026, 3, 21, 8, 0, 0, 0, time.UTC)},
		{"0 9 * * 1", time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)},
		{"0 0 1 * *", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
		{"30 2 15 6,12 *", time.Date(2026, 6, 15, 2, 30, 0, 0, time.UTC)},
		{"0 9 * * 7", time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		schedule, err := parseCron(tc.expression)
		if err != nil {
			t.Fatalf("parseCron(%q) err = %v", tc.expression, err)
		}
		if !schedule.Match(tc.when) {
			t.Fatalf("schedule %q should match %v", tc.expression, tc.when)
		}
	}
}

func TestParseCronInvalid(t *testing.T) {
	if _, err := parseCron("*/5 0-25 * * *"); err == nil {
		t.Fatal("expected error for invalid hour range")
	}
	if _, err := parseCron("* * *"); err == nil {
		t.Fatal("expected error for wrong field count")
	}
	if _, err := parseCron("* */0 * * *"); err == nil {
		t.Fatal("expected error for zero step")
	}
}
