package map_

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
)

func TestMapUnitManagerRejectsInvalidTarget(t *testing.T) {
	manager := &MapUnitManagerComponent{}
	if err := manager.SetTarget("", actor.ActorID{}); !errors.Is(err, ErrMapTargetInvalid) {
		t.Fatalf("SetTarget invalid input = %v, want %v", err, ErrMapTargetInvalid)
	}
}
