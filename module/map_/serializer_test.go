package map_

import (
	"errors"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/numeric"
	"github.com/jerbe/et-go/module/unit"
	"go.mongodb.org/mongo-driver/bson"
)

func TestSerializeUnitRoundTrip(t *testing.T) {
	original := unit.NewUnit(1001, unit.UnitTypePlayer)
	original.SetID(10086)
	original.SetPosition(etmath.Vector3{X: 1, Y: 2, Z: 3})
	original.SetRotation(etmath.LookRotation(etmath.Vector3{X: 1, Y: 0, Z: 0}))

	data, err := SerializeUnit(original)
	if err != nil {
		t.Fatalf("SerializeUnit error = %v", err)
	}
	clone, err := DeserializeUnit(data)
	if err != nil {
		t.Fatalf("DeserializeUnit error = %v", err)
	}
	if clone.ID() != original.ID() || clone.ConfigId != original.ConfigId || clone.UnitType != original.UnitType {
		t.Fatalf("clone mismatch: %+v", clone)
	}
	if clone.Position() != original.Position() {
		t.Fatalf("clone position = %v, want %v", clone.Position(), original.Position())
	}
	if etmath.QuaternionDistance(clone.Rotation(), original.Rotation()) > 1e-5 {
		t.Fatalf("clone rotation = %v, want %v", clone.Rotation(), original.Rotation())
	}
}

func TestDeserializeUnitRejectsUnknownIdentity(t *testing.T) {
	data, err := bson.Marshal(unitSnapshot{
		ID:       10086,
		ConfigID: 0,
		UnitType: int32(unit.UnitTypePlayer),
	})
	if err != nil {
		t.Fatalf("marshal invalid unit snapshot error = %v", err)
	}
	if _, err := DeserializeUnit(data); !errors.Is(err, ErrTransferUnitInvalid) {
		t.Fatalf("DeserializeUnit error = %v, want %v", err, ErrTransferUnitInvalid)
	}

	data, err = bson.Marshal(unitSnapshot{
		ID:       10086,
		ConfigID: 1001,
		UnitType: 9999,
	})
	if err != nil {
		t.Fatalf("marshal unknown unit snapshot error = %v", err)
	}
	if _, err := DeserializeUnit(data); !errors.Is(err, ErrTransferUnitInvalid) {
		t.Fatalf("DeserializeUnit unknown type error = %v, want %v", err, ErrTransferUnitInvalid)
	}
}

func TestSerializeTransferComponentsRoundTrip(t *testing.T) {
	u := unit.NewUnit(1001, unit.UnitTypePlayer)
	component := numeric.NewNumericComponent()
	u.AddComponent(component)
	component.SetFloat(numeric.Speed, 6.0)
	component.Set(numeric.AOI, 15000)

	data, err := SerializeTransferComponents(u)
	if err != nil {
		t.Fatalf("SerializeTransferComponents error = %v", err)
	}
	components, err := DeserializeComponents(data)
	if err != nil {
		t.Fatalf("DeserializeComponents error = %v", err)
	}
	if len(components) != 1 {
		t.Fatalf("components len = %d, want 1", len(components))
	}
	numericClone, ok := components[0].(*numeric.NumericComponent)
	if !ok {
		t.Fatalf("component type = %T, want *numeric.NumericComponent", components[0])
	}
	if numericClone.Get(numeric.Speed) != 60000 || numericClone.Get(numeric.AOI) != 15000 {
		t.Fatalf("clone values mismatch: speed=%d aoi=%d", numericClone.Get(numeric.Speed), numericClone.Get(numeric.AOI))
	}
}

func TestRegisterTransferComponentRejectsInvalidDefinition(t *testing.T) {
	if err := RegisterTransferComponent("", nil); !errors.Is(err, ErrTransferComponentInvalid) {
		t.Fatalf("invalid registration error = %v, want %v", err, ErrTransferComponentInvalid)
	}
}

func TestDeserializeComponentsRejectsInvalidFactory(t *testing.T) {
	const typeName = "InvalidTransferComponent"
	if err := RegisterTransferComponent(typeName, func() ecs.Component { return nil }); err != nil {
		t.Fatalf("RegisterTransferComponent error = %v", err)
	}
	raw, err := bson.Marshal(componentEnvelope{Type: typeName})
	if err != nil {
		t.Fatalf("marshal envelope error = %v", err)
	}
	if _, err := DeserializeComponents([][]byte{raw}); !errors.Is(err, ErrTransferComponentInvalid) {
		t.Fatalf("invalid factory error = %v, want %v", err, ErrTransferComponentInvalid)
	}
}
