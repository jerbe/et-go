package map_

import (
	"github.com/jerbe/et-go/engine/actor"
	mapinnerpb "github.com/jerbe/et-go/proto/mapinnerpb"
	mapouterpb "github.com/jerbe/et-go/proto/mapouterpb"
	gproto "google.golang.org/protobuf/proto"
)

func marshalTransferMap(msg *C2MTransferMap) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.C2M_TransferMap{RpcId: msg.RpcID})
}

func unmarshalTransferMap(data []byte) (*C2MTransferMap, error) {
	wire := &mapouterpb.C2M_TransferMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2MTransferMap{RpcID: wire.GetRpcId()}, nil
}

func marshalTransferMapResponse(msg *M2CTransferMap) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_TransferMap{
		RpcId:   msg.RpcID,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalTransferMapResponse(data []byte) (*M2CTransferMap, error) {
	wire := &mapouterpb.M2C_TransferMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2CTransferMap{
		RpcID:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func marshalUnitTransferRequest(msg *M2MUnitTransferRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapinnerpb.M2M_UnitTransferRequest{
		RpcId:      msg.RpcID,
		OldActorId: toWireActorID(msg.OldActorID),
		Unit:       append([]byte(nil), msg.Unit...),
		Entitys:    cloneBytesSlices(msg.Entitys),
	})
}

func unmarshalUnitTransferRequest(data []byte) (*M2MUnitTransferRequest, error) {
	wire := &mapinnerpb.M2M_UnitTransferRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2MUnitTransferRequest{
		RpcID:      wire.GetRpcId(),
		OldActorID: fromWireActorID(wire.GetOldActorId()),
		Unit:       append([]byte(nil), wire.GetUnit()...),
		Entitys:    cloneBytesSlices(wire.GetEntitys()),
	}, nil
}

func marshalUnitTransferResponse(msg *M2MUnitTransferResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapinnerpb.M2M_UnitTransferResponse{
		RpcId:   msg.RpcID,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalUnitTransferResponse(data []byte) (*M2MUnitTransferResponse, error) {
	wire := &mapinnerpb.M2M_UnitTransferResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2MUnitTransferResponse{
		RpcID:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func toWireActorID(id actor.ActorID) *mapinnerpb.ActorId {
	return &mapinnerpb.ActorId{
		ProcessId:  int32(id.ProcessID),
		FiberId:    id.FiberID,
		InstanceId: id.InstanceID,
	}
}

func fromWireActorID(id *mapinnerpb.ActorId) actor.ActorID {
	if id == nil {
		return actor.ActorID{}
	}
	return actor.ActorID{
		ProcessID:  int(id.GetProcessId()),
		FiberID:    id.GetFiberId(),
		InstanceID: id.GetInstanceId(),
	}
}

func cloneBytesSlices(src [][]byte) [][]byte {
	if len(src) == 0 {
		return nil
	}
	cloned := make([][]byte, 0, len(src))
	for _, item := range src {
		cloned = append(cloned, append([]byte(nil), item...))
	}
	return cloned
}
