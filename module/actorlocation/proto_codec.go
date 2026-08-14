package actorlocation

import (
	"fmt"

	"github.com/jerbe/et-go/engine/actor"
	actorlocationpb "github.com/jerbe/et-go/proto/actorlocationpb"
	gproto "google.golang.org/protobuf/proto"
)

func marshalAddRequest(msg *ObjectAddRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectAddRequest{
		RpcId:   int32(msg.RpcID),
		Type:    int32(msg.Type),
		Key:     msg.Key,
		ActorId: toWireActorID(msg.ActorID),
	})
}

func unmarshalAddRequest(data []byte) (*ObjectAddRequest, error) {
	wire := &actorlocationpb.ObjectAddRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectAddRequest{
		RpcID:   uint32(wire.GetRpcId()),
		Type:    LocationType(wire.GetType()),
		Key:     wire.GetKey(),
		ActorID: fromWireActorID(wire.GetActorId()),
	}, nil
}

func marshalAddResponse(msg *ObjectRemoveResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectAddResponse{
		RpcId:   int32(msg.RpcID),
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalAddResponse(data []byte) (*ObjectRemoveResponse, error) {
	wire := &actorlocationpb.ObjectAddResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectRemoveResponse{
		RpcID:   uint32(wire.GetRpcId()),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func marshalGetRequest(msg *ObjectGetRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectGetRequest{
		RpcId: int32(msg.RpcID),
		Type:  int32(msg.Type),
		Key:   msg.Key,
	})
}

func unmarshalGetRequest(data []byte) (*ObjectGetRequest, error) {
	wire := &actorlocationpb.ObjectGetRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectGetRequest{
		RpcID: uint32(wire.GetRpcId()),
		Type:  LocationType(wire.GetType()),
		Key:   wire.GetKey(),
	}, nil
}

func marshalGetResponse(msg *ObjectGetResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectGetResponse{
		RpcId:   int32(msg.RpcID),
		Error:   msg.Error,
		Message: msg.Message,
		ActorId: toWireActorID(msg.ActorID),
	})
}

func unmarshalGetResponse(data []byte) (*ObjectGetResponse, error) {
	wire := &actorlocationpb.ObjectGetResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectGetResponse{
		RpcID:   uint32(wire.GetRpcId()),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
		ActorID: fromWireActorID(wire.GetActorId()),
	}, nil
}

func marshalLockRequest(msg *ObjectLockRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectLockRequest{
		RpcId:   int32(msg.RpcID),
		Type:    int32(msg.Type),
		Key:     msg.Key,
		ActorId: toWireActorID(msg.ActorID),
		Time:    int32(msg.Time),
	})
}

func unmarshalLockRequest(data []byte) (*ObjectLockRequest, error) {
	wire := &actorlocationpb.ObjectLockRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectLockRequest{
		RpcID:   uint32(wire.GetRpcId()),
		Type:    LocationType(wire.GetType()),
		Key:     wire.GetKey(),
		ActorID: fromWireActorID(wire.GetActorId()),
		Time:    int(wire.GetTime()),
	}, nil
}

func marshalLockResponse(msg *ObjectRemoveResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectLockResponse{
		RpcId:   int32(msg.RpcID),
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalLockResponse(data []byte) (*ObjectRemoveResponse, error) {
	wire := &actorlocationpb.ObjectLockResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectRemoveResponse{
		RpcID:   uint32(wire.GetRpcId()),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func marshalUnlockRequest(msg *ObjectUnlockRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectUnLockRequest{
		RpcId:      int32(msg.RpcID),
		Type:       int32(msg.Type),
		Key:        msg.Key,
		OldActorId: toWireActorID(msg.OldActorID),
		NewActorId: toWireActorID(msg.NewActorID),
	})
}

func unmarshalUnlockRequest(data []byte) (*ObjectUnlockRequest, error) {
	wire := &actorlocationpb.ObjectUnLockRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectUnlockRequest{
		RpcID:      uint32(wire.GetRpcId()),
		Type:       LocationType(wire.GetType()),
		Key:        wire.GetKey(),
		OldActorID: fromWireActorID(wire.GetOldActorId()),
		NewActorID: fromWireActorID(wire.GetNewActorId()),
	}, nil
}

func marshalUnlockResponse(msg *ObjectRemoveResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectUnLockResponse{
		RpcId:   int32(msg.RpcID),
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalUnlockResponse(data []byte) (*ObjectRemoveResponse, error) {
	wire := &actorlocationpb.ObjectUnLockResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectRemoveResponse{
		RpcID:   uint32(wire.GetRpcId()),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func marshalRemoveRequest(msg *ObjectRemoveRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectRemoveRequest{
		RpcId: int32(msg.RpcID),
		Type:  int32(msg.Type),
		Key:   msg.Key,
	})
}

func unmarshalRemoveRequest(data []byte) (*ObjectRemoveRequest, error) {
	wire := &actorlocationpb.ObjectRemoveRequest{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectRemoveRequest{
		RpcID: uint32(wire.GetRpcId()),
		Type:  LocationType(wire.GetType()),
		Key:   wire.GetKey(),
	}, nil
}

func marshalRemoveResponse(msg *ObjectRemoveResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&actorlocationpb.ObjectRemoveResponse{
		RpcId:   int32(msg.RpcID),
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalRemoveResponse(data []byte) (*ObjectRemoveResponse, error) {
	wire := &actorlocationpb.ObjectRemoveResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ObjectRemoveResponse{
		RpcID:   uint32(wire.GetRpcId()),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func marshalRequest(msgID uint16, req any) ([]byte, error) {
	switch msgID {
	case MsgObjectAddRequest:
		msg, ok := req.(*ObjectAddRequest)
		if !ok {
			return nil, fmt.Errorf("%w: msg=%d type=%T", ErrMessageTypeInvalid, msgID, req)
		}
		return marshalAddRequest(msg)
	case MsgObjectGetRequest:
		msg, ok := req.(*ObjectGetRequest)
		if !ok {
			return nil, fmt.Errorf("%w: msg=%d type=%T", ErrMessageTypeInvalid, msgID, req)
		}
		return marshalGetRequest(msg)
	case MsgObjectLockRequest:
		msg, ok := req.(*ObjectLockRequest)
		if !ok {
			return nil, fmt.Errorf("%w: msg=%d type=%T", ErrMessageTypeInvalid, msgID, req)
		}
		return marshalLockRequest(msg)
	case MsgObjectUnlockRequest:
		msg, ok := req.(*ObjectUnlockRequest)
		if !ok {
			return nil, fmt.Errorf("%w: msg=%d type=%T", ErrMessageTypeInvalid, msgID, req)
		}
		return marshalUnlockRequest(msg)
	case MsgObjectRemoveRequest:
		msg, ok := req.(*ObjectRemoveRequest)
		if !ok {
			return nil, fmt.Errorf("%w: msg=%d type=%T", ErrMessageTypeInvalid, msgID, req)
		}
		return marshalRemoveRequest(msg)
	default:
		return nil, fmt.Errorf("actorlocation: unsupported request msg %d", msgID)
	}
}

func unmarshalCommonResponse(msgID uint16, data []byte) (*ObjectRemoveResponse, error) {
	switch msgID {
	case MsgObjectAddRequest:
		return unmarshalAddResponse(data)
	case MsgObjectLockRequest:
		return unmarshalLockResponse(data)
	case MsgObjectUnlockRequest:
		return unmarshalUnlockResponse(data)
	case MsgObjectRemoveRequest:
		return unmarshalRemoveResponse(data)
	default:
		return nil, fmt.Errorf("actorlocation: unsupported common response msg %d", msgID)
	}
}

func toWireActorID(actorID actor.ActorID) *actorlocationpb.ActorId {
	return &actorlocationpb.ActorId{
		ProcessId:  int32(actorID.ProcessID),
		FiberId:    actorID.FiberID,
		InstanceId: actorID.InstanceID,
	}
}

func fromWireActorID(actorID *actorlocationpb.ActorId) actor.ActorID {
	if actorID == nil {
		return actor.ActorID{}
	}
	return actor.ActorID{
		ProcessID:  int(actorID.GetProcessId()),
		FiberID:    actorID.GetFiberId(),
		InstanceID: actorID.GetInstanceId(),
	}
}
