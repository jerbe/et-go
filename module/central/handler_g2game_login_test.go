package central

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/db"
)

func TestUniquePlayerProfileRejectsDuplicates(t *testing.T) {
	_, err := uniquePlayerProfile([]db.CPlayerProfile{
		{Id: 1, AccountId: 7, ZoneId: 1},
		{Id: 2, AccountId: 7, ZoneId: 1},
	}, 7, 1)
	if !errors.Is(err, ErrPlayerProfileDuplicate) {
		t.Fatalf("duplicate profile error = %v, want %v", err, ErrPlayerProfileDuplicate)
	}
}

func TestUniquePlayerProfileReturnsCopy(t *testing.T) {
	profiles := []db.CPlayerProfile{{Id: 1, AccountId: 7, ZoneId: 1}}
	profile, err := uniquePlayerProfile(profiles, 7, 1)
	if err != nil {
		t.Fatalf("unique profile error = %v", err)
	}
	if profile == nil || profile.Id != 1 {
		t.Fatalf("profile = %+v", profile)
	}
	profiles[0].Id = 9
	if profile.Id != 1 {
		t.Fatalf("profile should be copied, got id %d", profile.Id)
	}
}
