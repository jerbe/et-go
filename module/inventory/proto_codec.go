package inventory

import (
	inventorypb "github.com/jerbe/et-go/proto/inventorypb"
	gproto "google.golang.org/protobuf/proto"
)

func marshalGetBagInfoReq(msg *C2MGetBagInfo) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.C2M_GetBagInfo{
		RpcId:  msg.RpcId,
		UnitId: msg.UnitId,
	})
}

func unmarshalGetBagInfoReq(data []byte) (*C2MGetBagInfo, error) {
	wire := &inventorypb.C2M_GetBagInfo{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2MGetBagInfo{RpcId: wire.GetRpcId(), UnitId: wire.GetUnitId()}, nil
}

func marshalGetBagInfoResp(msg *M2CGetBagInfo) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	items := make([]*inventorypb.ItemInfo, 0, len(msg.Items))
	for _, item := range msg.Items {
		items = append(items, toWireItemInfo(item))
	}
	return gproto.Marshal(&inventorypb.M2C_GetBagInfo{
		RpcId:       msg.RpcId,
		Error:       msg.Error,
		Message:     msg.Message,
		MaxCapacity: msg.MaxCapacity,
		Items:       items,
	})
}

func unmarshalGetBagInfoResp(data []byte) (*M2CGetBagInfo, error) {
	wire := &inventorypb.M2C_GetBagInfo{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	items := make([]ItemInfo, 0, len(wire.GetItems()))
	for _, item := range wire.GetItems() {
		items = append(items, fromWireItemInfo(item))
	}
	return &M2CGetBagInfo{
		RpcId:       wire.GetRpcId(),
		Error:       wire.GetError(),
		Message:     wire.GetMessage(),
		MaxCapacity: wire.GetMaxCapacity(),
		Items:       items,
	}, nil
}

func marshalBagOperationReq(msg *C2MBagOperation) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.C2M_BagOperation{
		RpcId:      msg.RpcId,
		UnitId:     msg.UnitId,
		OpType:     msg.OpType,
		ItemId:     msg.ItemId,
		ConfigId:   msg.ConfigId,
		Count:      msg.Count,
		SourceSlot: msg.SourceSlot,
		TargetSlot: msg.TargetSlot,
	})
}

func unmarshalBagOperationReq(data []byte) (*C2MBagOperation, error) {
	wire := &inventorypb.C2M_BagOperation{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2MBagOperation{
		RpcId:      wire.GetRpcId(),
		UnitId:     wire.GetUnitId(),
		OpType:     wire.GetOpType(),
		ItemId:     wire.GetItemId(),
		ConfigId:   wire.GetConfigId(),
		Count:      wire.GetCount(),
		SourceSlot: wire.GetSourceSlot(),
		TargetSlot: wire.GetTargetSlot(),
	}, nil
}

func marshalBagOperationResp(msg *M2CBagOperation) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.M2C_BagOperation{
		RpcId:   msg.RpcId,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalBagOperationResp(data []byte) (*M2CBagOperation, error) {
	wire := &inventorypb.M2C_BagOperation{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2CBagOperation{RpcId: wire.GetRpcId(), Error: wire.GetError(), Message: wire.GetMessage()}, nil
}

func marshalGetWarehouseInfoReq(msg *C2MGetWarehouseInfo) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.C2M_GetWarehouseInfo{
		RpcId:  msg.RpcId,
		UnitId: msg.UnitId,
	})
}

func unmarshalGetWarehouseInfoReq(data []byte) (*C2MGetWarehouseInfo, error) {
	wire := &inventorypb.C2M_GetWarehouseInfo{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2MGetWarehouseInfo{RpcId: wire.GetRpcId(), UnitId: wire.GetUnitId()}, nil
}

func marshalGetWarehouseInfoResp(msg *M2CGetWarehouseInfo) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	items := make([]*inventorypb.ItemInfo, 0, len(msg.Items))
	for _, item := range msg.Items {
		items = append(items, toWireItemInfo(item))
	}
	return gproto.Marshal(&inventorypb.M2C_GetWarehouseInfo{
		RpcId:       msg.RpcId,
		Error:       msg.Error,
		Message:     msg.Message,
		MaxCapacity: msg.MaxCapacity,
		Items:       items,
	})
}

func unmarshalGetWarehouseInfoResp(data []byte) (*M2CGetWarehouseInfo, error) {
	wire := &inventorypb.M2C_GetWarehouseInfo{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	items := make([]ItemInfo, 0, len(wire.GetItems()))
	for _, item := range wire.GetItems() {
		items = append(items, fromWireItemInfo(item))
	}
	return &M2CGetWarehouseInfo{
		RpcId:       wire.GetRpcId(),
		Error:       wire.GetError(),
		Message:     wire.GetMessage(),
		MaxCapacity: wire.GetMaxCapacity(),
		Items:       items,
	}, nil
}

func marshalWarehouseOperationReq(msg *C2MWarehouseOperation) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.C2M_WarehouseOperation{
		RpcId:      msg.RpcId,
		UnitId:     msg.UnitId,
		OpType:     msg.OpType,
		ItemId:     msg.ItemId,
		Count:      msg.Count,
		SourceSlot: msg.SourceSlot,
		TargetSlot: msg.TargetSlot,
	})
}

func unmarshalWarehouseOperationReq(data []byte) (*C2MWarehouseOperation, error) {
	wire := &inventorypb.C2M_WarehouseOperation{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2MWarehouseOperation{
		RpcId:      wire.GetRpcId(),
		UnitId:     wire.GetUnitId(),
		OpType:     wire.GetOpType(),
		ItemId:     wire.GetItemId(),
		Count:      wire.GetCount(),
		SourceSlot: wire.GetSourceSlot(),
		TargetSlot: wire.GetTargetSlot(),
	}, nil
}

func marshalWarehouseOperationResp(msg *M2CWarehouseOperation) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.M2C_WarehouseOperation{
		RpcId:   msg.RpcId,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalWarehouseOperationResp(data []byte) (*M2CWarehouseOperation, error) {
	wire := &inventorypb.M2C_WarehouseOperation{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2CWarehouseOperation{RpcId: wire.GetRpcId(), Error: wire.GetError(), Message: wire.GetMessage()}, nil
}

func marshalItemChange(msg *M2CItemChange) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&inventorypb.M2C_ItemChange{
		ChangeType: msg.ChangeType,
		Container:  msg.Container,
		Item:       toWireItemInfo(msg.Item),
	})
}

func unmarshalItemChange(data []byte) (*M2CItemChange, error) {
	wire := &inventorypb.M2C_ItemChange{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2CItemChange{
		ChangeType: wire.GetChangeType(),
		Container:  wire.GetContainer(),
		Item:       fromWireItemInfo(wire.GetItem()),
	}, nil
}

func toWireItemInfo(item ItemInfo) *inventorypb.ItemInfo {
	return &inventorypb.ItemInfo{
		ItemId:    item.ItemId,
		ConfigId:  item.ConfigId,
		Count:     item.Count,
		SlotIndex: item.SlotIndex,
		UniqueId:  item.UniqueId,
	}
}

func fromWireItemInfo(item *inventorypb.ItemInfo) ItemInfo {
	if item == nil {
		return ItemInfo{}
	}
	return ItemInfo{
		ItemId:    item.GetItemId(),
		ConfigId:  item.GetConfigId(),
		Count:     item.GetCount(),
		SlotIndex: item.GetSlotIndex(),
		UniqueId:  item.GetUniqueId(),
	}
}
