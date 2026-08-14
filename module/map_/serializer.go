package map_

import (
	"fmt"
	"strings"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/numeric"
	"github.com/jerbe/et-go/module/unit"
	"go.mongodb.org/mongo-driver/bson"
)

type unitSnapshot struct {
	ID       int64             `bson:"id"`
	ConfigID int32             `bson:"config_id"`
	UnitType int32             `bson:"unit_type"`
	Position etmath.Vector3     `bson:"position"`
	Rotation etmath.Quaternion `bson:"rotation"`
}

type componentEnvelope struct {
	Type string `bson:"type"`
	Data []byte `bson:"data"`
}

var (
	componentFactoryMu sync.RWMutex
	componentFactories = map[string]func() ecs.Component{}
)

func init() {
	RegisterTransferComponent("NumericComponent", func() ecs.Component {
		return numeric.NewNumericComponent()
	})
	RegisterTransferComponent("BagComponent", func() ecs.Component {
		return &inventory.BagComponent{}
	})
	RegisterTransferComponent("WarehouseComponent", func() ecs.Component {
		return &inventory.WarehouseComponent{}
	})
}

// RegisterTransferComponent 注册组件反序列化工厂。
func RegisterTransferComponent(typeName string, factory func() ecs.Component) error {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" || factory == nil {
		return ErrTransferComponentInvalid
	}
	componentFactoryMu.Lock()
	defer componentFactoryMu.Unlock()
	componentFactories[typeName] = factory
	return nil
}

// SerializeUnit 将 Unit 序列化为 BSON。
func SerializeUnit(u *unit.Unit) ([]byte, error) {
	if u == nil {
		return nil, ErrTransferUnitMissing
	}
	snapshot := unitSnapshot{
		ID:       u.ID(),
		ConfigID: u.ConfigId,
		UnitType: int32(u.UnitType),
		Position: u.Position(),
		Rotation: u.Rotation(),
	}
	if err := validateUnitSnapshot(snapshot); err != nil {
		return nil, err
	}
	return bson.Marshal(snapshot)
}

// DeserializeUnit 从 BSON 恢复 Unit。
func DeserializeUnit(data []byte) (*unit.Unit, error) {
	var snapshot unitSnapshot
	if err := bson.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	if err := validateUnitSnapshot(snapshot); err != nil {
		return nil, err
	}
	result := unit.NewUnit(snapshot.ConfigID, unit.UnitType(snapshot.UnitType))
	result.SetID(snapshot.ID)
	result.SetPosition(snapshot.Position)
	result.SetRotation(snapshot.Rotation)
	return result, nil
}

func validateUnitSnapshot(snapshot unitSnapshot) error {
	if snapshot.ID <= 0 || snapshot.ConfigID <= 0 {
		return ErrTransferUnitInvalid
	}
	switch unit.UnitType(snapshot.UnitType) {
	case unit.UnitTypePlayer, unit.UnitTypeMonster, unit.UnitTypeNPC:
		return nil
	default:
		return ErrTransferUnitInvalid
	}
}

// SerializeTransferComponents 序列化所有可转移组件。
func SerializeTransferComponents(u *unit.Unit) ([][]byte, error) {
	if u == nil {
		return nil, ErrTransferUnitMissing
	}
	components := u.GetTransferComponents()
	result := make([][]byte, 0, len(components))
	for _, component := range components {
		if component == nil {
			continue
		}
		transferComponent, ok := component.(ecs.TransferSystem)
		if !ok {
			continue
		}
		data, err := transferComponent.Transfer()
		if err != nil {
			return nil, err
		}
		wrapped, err := bson.Marshal(componentEnvelope{
			Type: component.Type(),
			Data: data,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, wrapped)
	}
	return result, nil
}

// DeserializeComponents 恢复可转移组件。
func DeserializeComponents(entityBytes [][]byte) ([]ecs.Component, error) {
	result := make([]ecs.Component, 0, len(entityBytes))
	seen := make(map[string]struct{}, len(entityBytes))
	for _, raw := range entityBytes {
		var envelope componentEnvelope
		if err := bson.Unmarshal(raw, &envelope); err != nil {
			return nil, err
		}
		envelope.Type = strings.TrimSpace(envelope.Type)
		if envelope.Type == "" {
			return nil, fmt.Errorf("%w: empty type", ErrTransferComponentInvalid)
		}
		if _, exists := seen[envelope.Type]; exists {
			return nil, fmt.Errorf("%w: %s", ErrTransferComponentDuplicate, envelope.Type)
		}
		seen[envelope.Type] = struct{}{}
		componentFactoryMu.RLock()
		factory, ok := componentFactories[envelope.Type]
		componentFactoryMu.RUnlock()
		if !ok || factory == nil {
			return nil, fmt.Errorf("%w: %s", ErrTransferComponentUnsupported, envelope.Type)
		}

		component := factory()
		if component == nil {
			return nil, fmt.Errorf("%w: factory returned nil for %s", ErrTransferComponentInvalid, envelope.Type)
		}
		if component.Type() != envelope.Type {
			return nil, fmt.Errorf("%w: factory type=%q envelope type=%q", ErrTransferComponentInvalid, component.Type(), envelope.Type)
		}
		if transferComponent, ok := component.(ecs.TransferSystem); ok {
			if err := transferComponent.OnTransferIn(envelope.Data); err != nil {
				return nil, err
			}
		} else if deserializeComponent, ok := component.(ecs.DeserializeSystem); ok {
			if err := deserializeComponent.Deserialize(envelope.Data); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("%w: %s has no deserializer", ErrTransferComponentInvalid, envelope.Type)
		}
		result = append(result, component)
	}
	return result, nil
}
