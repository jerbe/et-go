package inventory

import (
	"errors"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/unit"
)

var errInventoryNotifyDependency = errors.New("inventory: message location sender missing")
var errInventoryItemInvalid = errors.New("inventory: item info is empty")

// NotifyItemChange 发送单条物品变更通知。
func NotifyItemChange(u *unit.Unit, changeType int32, container int32, item *Item) error {
	if u == nil || u.Scene() == nil || item == nil {
		return errInventoryNotifyDependency
	}
	component, ok := u.Scene().GetComponent("MessageLocationSenderComponent")
	if !ok || component == nil {
		return errInventoryNotifyDependency
	}
	senderComponent, ok := component.(*actorlocation.MessageLocationSenderComponent)
	if !ok || senderComponent == nil {
		return errInventoryNotifyDependency
	}
	sender := senderComponent.Get(int(actorlocation.LocationTypeGateSession))
	if sender == nil {
		return errInventoryNotifyDependency
	}
	payload, err := marshalItemChange(&M2CItemChange{
		ChangeType: changeType,
		Container:  container,
		Item:       toItemInfo(item),
	})
	if err != nil {
		return err
	}
	return sender.Send(u.ID(), MsgM2CItemChange, payload)
}

// NotifyItemChanges 发送多条物品变更通知。
func NotifyItemChanges(u *unit.Unit, changeType int32, container int32, items []*Item) error {
	for _, item := range items {
		if err := NotifyItemChange(u, changeType, container, item); err != nil {
			return err
		}
	}
	return nil
}

// NotifyItemChangeInfos 发送结构化变更通知。
func NotifyItemChangeInfos(u *unit.Unit, changes []ItemChangeInfo) error {
	for _, change := range changes {
		item := change.Item
		if item == (ItemInfo{}) {
			return errInventoryItemInvalid
		}
		if err := NotifyItemChange(u, change.ChangeType, change.Container, &Item{
			Entity:    *ecs.NewEntity(),
			ConfigId:  item.ConfigId,
			Count:     item.Count,
			SlotIndex: item.SlotIndex,
			UniqueId:  item.UniqueId,
		}); err != nil {
			return err
		}
	}
	return nil
}

func toItemInfo(item *Item) ItemInfo {
	if item == nil {
		return ItemInfo{}
	}
	return ItemInfo{
		ItemId:    item.ID(),
		ConfigId:  item.ConfigId,
		Count:     item.Count,
		SlotIndex: item.SlotIndex,
		UniqueId:  item.UniqueId,
	}
}
