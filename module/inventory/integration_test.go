package inventory

import (
	"context"
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/unit"
)

type stubInventorySender struct {
	payloads [][]byte
}

func (s *stubInventorySender) Send(_ int64, _ uint16, payload []byte) error {
	s.payloads = append(s.payloads, payload)
	return nil
}

func (s *stubInventorySender) Call(_ context.Context, _ int64, _ uint16, _ []byte) ([]byte, error) {
	return nil, nil
}

type stubInventorySenderComponent struct {
	ecs.BaseComponent
	sender *stubInventorySender
}

func (c *stubInventorySenderComponent) Type() string { return "MessageLocationSenderComponent" }
func (c *stubInventorySenderComponent) Get(_ int) interface {
	Send(key int64, msgID uint16, payload []byte) error
	Call(ctx context.Context, key int64, msgID uint16, payload []byte) ([]byte, error)
} {
	return c.sender
}

func TestInventoryHandlers(t *testing.T) {
	RegisterItemConfigType(5001, ItemTypeConsumable)
	RegisterItemConfigType(5002, ItemTypeWeapon)
	scene := ecs.NewScene(ecs.SceneTypeMap, 1, "map")
	scene.AddComponent(&unit.UnitComponent{})
	scene.AddComponent(&stubInventorySenderComponent{sender: &stubInventorySender{}})
	u, err := unit.CreatePlayer(scene, 1)
	if err != nil {
		t.Fatalf("CreatePlayer error = %v", err)
	}
	u.AddComponent(&BagComponent{MaxCapacity: 5})
	u.AddComponent(&WarehouseComponent{})
	bagComponent, _ := u.GetComponent("BagComponent")
	bag := bagComponent.(*BagComponent)
	_, _ = bag.TryAddItem(5002, 1)

	mailbox := actor.NewMailBox(actor.ActorID{ProcessID: 1, FiberID: 1, InstanceID: scene.InstanceID()}, actor.MailBoxTypeUnOrderedMessage)
	scene.AddComponent(mailbox)
	RegisterMapHandlers(scene, mailbox)

	payload, err := marshalBagOperationReq(&C2MBagOperation{RpcId: 1, UnitId: u.ID(), OpType: 4})
	if err != nil {
		t.Fatalf("marshal sort bag op err = %v", err)
	}
	if _, err := mailbox.Dispatch(MsgC2MBagOperation, payload); err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}

	payload, err = marshalBagOperationReq(&C2MBagOperation{RpcId: 2, UnitId: u.ID(), OpType: 3, SourceSlot: 0, TargetSlot: 3})
	if err != nil {
		t.Fatalf("marshal swap bag op err = %v", err)
	}
	if _, err := mailbox.Dispatch(MsgC2MBagOperation, payload); err != nil {
		t.Fatalf("Dispatch swap err = %v", err)
	}
}
