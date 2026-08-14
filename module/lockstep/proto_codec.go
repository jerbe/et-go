package lockstep

import (
	"fmt"

	"github.com/jerbe/et-go/engine/actor"
	etmath "github.com/jerbe/et-go/engine/math"
	locksteppb "github.com/jerbe/et-go/proto/locksteppb"
	mapouterpb "github.com/jerbe/et-go/proto/mapouterpb"
	gproto "google.golang.org/protobuf/proto"
)

func marshalC2GMatch(msg *C2GMatch) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.C2G_Match{RpcId: msg.RpcId, PlayerId: msg.PlayerId})
}

func unmarshalC2GMatch(data []byte) (*C2GMatch, error) {
	wire := &locksteppb.C2G_Match{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &C2GMatch{RpcId: wire.GetRpcId(), PlayerId: wire.GetPlayerId()}, nil
}

func marshalG2CMatch(msg *G2CMatch) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.G2C_Match{RpcId: msg.RpcId, Error: msg.Error, Message: msg.Message})
}

func unmarshalG2CMatch(data []byte) (*G2CMatch, error) {
	wire := &locksteppb.G2C_Match{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2CMatch{RpcId: wire.GetRpcId(), Error: wire.GetError(), Message: wire.GetMessage()}, nil
}

func marshalMatch2GNotifyMatchSuccess(msg *Match2GNotifyMatchSuccess) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Match2G_NotifyMatchSuccess{
		PlayerId:    msg.PlayerId,
		MapActorId:  msg.MapActorId,
		RoomActorId: msg.RoomActorId,
		MapActor:    toWireActorID(msg.MapActor),
		RoomActor:   toWireActorID(msg.RoomActor),
	})
}

func unmarshalMatch2GNotifyMatchSuccess(data []byte) (*Match2GNotifyMatchSuccess, error) {
	wire := &locksteppb.Match2G_NotifyMatchSuccess{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Match2GNotifyMatchSuccess{
		PlayerId:    wire.GetPlayerId(),
		MapActorId:  wire.GetMapActorId(),
		RoomActorId: wire.GetRoomActorId(),
		MapActor:    fromWireActorID(wire.GetMapActor()),
		RoomActor:   fromWireActorID(wire.GetRoomActor()),
	}, nil
}

func marshalG2CNotifyMatchSuccess(msg *Match2GNotifyMatchSuccess) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.G2C_NotifyMatchSuccess{
		MapActorId:  msg.MapActorId,
		RoomActorId: msg.RoomActorId,
		MapActor:    toWireActorID(msg.MapActor),
		RoomActor:   toWireActorID(msg.RoomActor),
	})
}

func marshalG2MatchMatch(msg *G2MatchMatch) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.G2Match_Match{RpcId: msg.RpcId, PlayerId: msg.PlayerId})
}

func unmarshalG2MatchMatch(data []byte) (*G2MatchMatch, error) {
	wire := &locksteppb.G2Match_Match{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &G2MatchMatch{RpcId: wire.GetRpcId(), PlayerId: wire.GetPlayerId()}, nil
}

func marshalMatch2MapGetRoom(msg *Match2MapGetRoom) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Match2Map_GetRoom{RpcId: msg.RpcId, PlayerIds: append([]int64(nil), msg.PlayerIds...)})
}

func unmarshalMatch2MapGetRoom(data []byte) (*Match2MapGetRoom, error) {
	wire := &locksteppb.Match2Map_GetRoom{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Match2MapGetRoom{RpcId: wire.GetRpcId(), PlayerIds: append([]int64(nil), wire.GetPlayerIds()...)}, nil
}

func marshalMap2MatchGetRoom(msg *Map2MatchGetRoom) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Map2Match_GetRoom{
		RpcId:       msg.RpcId,
		MapActorId:  msg.MapActorId,
		RoomActorId: msg.RoomActorId,
		MapActor:    toWireActorID(msg.MapActor),
		RoomActor:   toWireActorID(msg.RoomActor),
	})
}

func unmarshalMap2MatchGetRoom(data []byte) (*Map2MatchGetRoom, error) {
	wire := &locksteppb.Map2Match_GetRoom{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Map2MatchGetRoom{
		RpcId:       wire.GetRpcId(),
		MapActorId:  wire.GetMapActorId(),
		RoomActorId: wire.GetRoomActorId(),
		MapActor:    fromWireActorID(wire.GetMapActor()),
		RoomActor:   fromWireActorID(wire.GetRoomActor()),
	}, nil
}

func marshalMatch2MapCancelRoom(msg *Match2MapCancelRoom) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Match2Map_CancelRoom{
		RpcId:     msg.RpcId,
		RoomActor: toWireActorID(msg.RoomActor),
		PlayerIds: append([]int64(nil), msg.PlayerIds...),
	})
}

func unmarshalMatch2MapCancelRoom(data []byte) (*Match2MapCancelRoom, error) {
	wire := &locksteppb.Match2Map_CancelRoom{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Match2MapCancelRoom{
		RpcId:     wire.GetRpcId(),
		RoomActor: fromWireActorID(wire.GetRoomActor()),
		PlayerIds: append([]int64(nil), wire.GetPlayerIds()...),
	}, nil
}

func marshalMap2MatchCancelRoom(msg *Map2MatchCancelRoom) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Map2Match_CancelRoom{
		RpcId:     msg.RpcId,
		Cancelled: msg.Cancelled,
		Message:   msg.Message,
	})
}

func unmarshalMap2MatchCancelRoom(data []byte) (*Map2MatchCancelRoom, error) {
	wire := &locksteppb.Map2Match_CancelRoom{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Map2MatchCancelRoom{
		RpcId:     wire.GetRpcId(),
		Cancelled: wire.GetCancelled(),
		Message:   wire.GetMessage(),
	}, nil
}

func marshalMatch2GCancelMatchSuccess(msg *Match2GCancelMatchSuccess) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Match2G_CancelMatchSuccess{
		PlayerId:  msg.PlayerId,
		MapActor:  toWireActorID(msg.MapActor),
		RoomActor: toWireActorID(msg.RoomActor),
	})
}

func unmarshalMatch2GCancelMatchSuccess(data []byte) (*Match2GCancelMatchSuccess, error) {
	wire := &locksteppb.Match2G_CancelMatchSuccess{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Match2GCancelMatchSuccess{
		PlayerId:  wire.GetPlayerId(),
		MapActor:  fromWireActorID(wire.GetMapActor()),
		RoomActor: fromWireActorID(wire.GetRoomActor()),
	}, nil
}

func marshalRoom2CStart(msg *Room2CStart) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	unitInfos := make([]*locksteppb.LockStepUnitInfo, 0, len(msg.UnitInfos))
	for _, item := range msg.UnitInfos {
		unitInfos = append(unitInfos, toWireLockStepUnitInfo(item))
	}
	return gproto.Marshal(&locksteppb.Room2C_Start{RpcId: msg.RpcId, StartTime: msg.StartTime, UnitInfos: unitInfos})
}

func unmarshalRoom2CStart(data []byte) (*Room2CStart, error) {
	wire := &locksteppb.Room2C_Start{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	unitInfos := make([]*LockStepUnitInfo, 0, len(wire.GetUnitInfos()))
	for _, item := range wire.GetUnitInfos() {
		unitInfos = append(unitInfos, fromWireLockStepUnitInfo(item))
	}
	return &Room2CStart{RpcId: wire.GetRpcId(), StartTime: wire.GetStartTime(), UnitInfos: unitInfos}, nil
}

func marshalRoom2CAdjustUpdateTime(msg *Room2CAdjustUpdateTime) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Room2C_AdjustUpdateTime{RpcId: msg.RpcId, DiffTime: msg.DiffTime})
}

func unmarshalRoom2CAdjustUpdateTime(data []byte) (*Room2CAdjustUpdateTime, error) {
	wire := &locksteppb.Room2C_AdjustUpdateTime{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Room2CAdjustUpdateTime{RpcId: wire.GetRpcId(), DiffTime: wire.GetDiffTime()}, nil
}

func marshalRoom2CReconnect(msg *Room2CReconnect) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	unitInfos := make([]*locksteppb.LockStepUnitInfo, 0, len(msg.UnitInfos))
	for _, item := range msg.UnitInfos {
		unitInfos = append(unitInfos, toWireLockStepUnitInfo(item))
	}
	frameInputs := make([]*locksteppb.OneFrameInputs, 0, len(msg.FrameInputs))
	for _, item := range msg.FrameInputs {
		frameInputs = append(frameInputs, toWireOneFrameInputs(item))
	}
	return gproto.Marshal(&locksteppb.Room2C_Reconnect{
		RpcId:         msg.RpcId,
		StartTime:     msg.StartTime,
		UnitInfos:     unitInfos,
		Frame:         int32(msg.Frame),
		SnapshotFrame: int32(msg.SnapshotFrame),
		Snapshot:      append([]byte(nil), msg.Snapshot...),
		FrameInputs:   frameInputs,
	})
}

func unmarshalRoom2CReconnect(data []byte) (*Room2CReconnect, error) {
	wire := &locksteppb.Room2C_Reconnect{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	unitInfos := make([]*LockStepUnitInfo, 0, len(wire.GetUnitInfos()))
	for _, item := range wire.GetUnitInfos() {
		unitInfos = append(unitInfos, fromWireLockStepUnitInfo(item))
	}
	frameInputs := make([]*OneFrameInputs, 0, len(wire.GetFrameInputs()))
	for _, item := range wire.GetFrameInputs() {
		frameInputs = append(frameInputs, fromWireOneFrameInputs(item))
	}
	return &Room2CReconnect{
		RpcId:         wire.GetRpcId(),
		StartTime:     wire.GetStartTime(),
		UnitInfos:     unitInfos,
		Frame:         int(wire.GetFrame()),
		SnapshotFrame: int(wire.GetSnapshotFrame()),
		Snapshot:      append([]byte(nil), wire.GetSnapshot()...),
		FrameInputs:   frameInputs,
	}, nil
}

func marshalRoom2CCheckHashFail(msg *Room2CCheckHashFail) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Room2C_CheckHashFail{
		RpcId:         msg.RpcId,
		Frame:         int32(msg.Frame),
		SnapshotFrame: int32(msg.SnapshotFrame),
		Snapshot:      append([]byte(nil), msg.Snapshot...),
	})
}

func unmarshalRoom2CCheckHashFail(data []byte) (*Room2CCheckHashFail, error) {
	wire := &locksteppb.Room2C_CheckHashFail{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Room2CCheckHashFail{
		RpcId:         wire.GetRpcId(),
		Frame:         int(wire.GetFrame()),
		SnapshotFrame: int(wire.GetSnapshotFrame()),
		Snapshot:      append([]byte(nil), wire.GetSnapshot()...),
	}, nil
}

func marshalFrameMessageRequest(msg *FrameMessageRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.FrameMessage{
		RpcId:    msg.RpcId,
		PlayerId: msg.PlayerId,
		Frame:    int32(msg.Frame),
		Input:    toWireLSInput(msg.Input),
	})
}

func unmarshalFrameMessageRequest(data []byte) (*FrameMessageRequest, error) {
	wire := &locksteppb.FrameMessage{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &FrameMessageRequest{
		RpcId:    wire.GetRpcId(),
		PlayerId: wire.GetPlayerId(),
		Frame:    int(wire.GetFrame()),
		Input:    fromWireLSInput(wire.GetInput()),
	}, nil
}

func marshalFrameMessageResponse(msg *FrameMessageResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.FrameMessageResponse{RpcId: msg.RpcId, Accepted: msg.Accepted})
}

func unmarshalFrameMessageResponse(data []byte) (*FrameMessageResponse, error) {
	wire := &locksteppb.FrameMessageResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &FrameMessageResponse{RpcId: wire.GetRpcId(), Accepted: wire.GetAccepted()}, nil
}

func marshalChangeSceneFinish(msg *ChangeSceneFinish) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.C2Room_ChangeSceneFinish{RpcId: msg.RpcId, PlayerId: msg.PlayerId})
}

func unmarshalChangeSceneFinish(data []byte) (*ChangeSceneFinish, error) {
	wire := &locksteppb.C2Room_ChangeSceneFinish{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ChangeSceneFinish{RpcId: wire.GetRpcId(), PlayerId: wire.GetPlayerId()}, nil
}

func marshalCheckHashRequest(msg *CheckHashRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.C2Room_CheckHash{RpcId: msg.RpcId, PlayerId: msg.PlayerId, Frame: int32(msg.Frame), Hash: msg.Hash})
}

func unmarshalCheckHashRequest(data []byte) (*CheckHashRequest, error) {
	wire := &locksteppb.C2Room_CheckHash{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &CheckHashRequest{RpcId: wire.GetRpcId(), PlayerId: wire.GetPlayerId(), Frame: int(wire.GetFrame()), Hash: wire.GetHash()}, nil
}

func marshalCheckHashResponse(msg *CheckHashResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.CheckHashResponse{
		RpcId:         msg.RpcId,
		Frame:         int32(msg.Frame),
		SnapshotFrame: int32(msg.SnapshotFrame),
		Snapshot:      append([]byte(nil), msg.Snapshot...),
	})
}

func unmarshalCheckHashResponse(data []byte) (*CheckHashResponse, error) {
	wire := &locksteppb.CheckHashResponse{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &CheckHashResponse{
		RpcId:         wire.GetRpcId(),
		Frame:         int(wire.GetFrame()),
		SnapshotFrame: int(wire.GetSnapshotFrame()),
		Snapshot:      append([]byte(nil), wire.GetSnapshot()...),
	}, nil
}

func marshalReconnectRequest(msg *ReconnectRequest) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.G2Room_Reconnect{RpcId: msg.RpcId, PlayerId: msg.PlayerId})
}

func unmarshalReconnectRequest(data []byte) (*ReconnectRequest, error) {
	wire := &locksteppb.G2Room_Reconnect{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &ReconnectRequest{RpcId: wire.GetRpcId(), PlayerId: wire.GetPlayerId()}, nil
}

func marshalRoomDispose(msg *Room2MNotifyRoomDispose) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.Room2M_NotifyRoomDispose{
		RoomActorId: msg.RoomActorId,
		PlayerIds:   append([]int64(nil), msg.PlayerIds...),
		DisposeAt:   msg.DisposeAt,
		RoomActor:   toWireActorID(msg.RoomActor),
	})
}

func unmarshalRoomDispose(data []byte) (*Room2MNotifyRoomDispose, error) {
	wire := &locksteppb.Room2M_NotifyRoomDispose{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Room2MNotifyRoomDispose{
		RoomActorId: wire.GetRoomActorId(),
		PlayerIds:   append([]int64(nil), wire.GetPlayerIds()...),
		DisposeAt:   wire.GetDisposeAt(),
		RoomActor:   fromWireActorID(wire.GetRoomActor()),
	}, nil
}

func marshalPlayerOffline(msg *M2RoomPlayerOffline) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&locksteppb.M2Room_PlayerOffline{PlayerId: msg.PlayerId})
}

func unmarshalPlayerOffline(data []byte) (*M2RoomPlayerOffline, error) {
	wire := &locksteppb.M2Room_PlayerOffline{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &M2RoomPlayerOffline{PlayerId: wire.GetPlayerId()}, nil
}

func marshalOneFrameInputs(msg *OneFrameInputs) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(toWireOneFrameInputs(msg))
}

func unmarshalOneFrameInputs(data []byte) (*OneFrameInputs, error) {
	wire := &locksteppb.OneFrameInputs{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return fromWireOneFrameInputs(wire), nil
}

func marshalByMessage(msg any) (uint16, []byte, error) {
	switch m := msg.(type) {
	case *Room2CStart:
		payload, err := marshalRoom2CStart(m)
		return MsgRoom2CStart, payload, err
	case *Room2CAdjustUpdateTime:
		payload, err := marshalRoom2CAdjustUpdateTime(m)
		return MsgRoom2CAdjustUpdateTime, payload, err
	case *Room2CReconnect:
		payload, err := marshalRoom2CReconnect(m)
		return MsgRoom2CReconnect, payload, err
	case *Room2CCheckHashFail:
		payload, err := marshalRoom2CCheckHashFail(m)
		return MsgRoom2CCheckHashFail, payload, err
	case *Room2MNotifyRoomDispose:
		payload, err := marshalRoomDispose(m)
		return MsgRoom2MNotifyRoomDispose, payload, err
	case *OneFrameInputs:
		payload, err := marshalOneFrameInputs(m)
		return MsgOneFrameInputs, payload, err
	case *Match2GNotifyMatchSuccess:
		payload, err := marshalMatch2GNotifyMatchSuccess(m)
		return MsgMatch2GNotifyMatchSuccess, payload, err
	case *Match2GCancelMatchSuccess:
		payload, err := marshalMatch2GCancelMatchSuccess(m)
		return MsgMatch2GCancelMatchSuccess, payload, err
	default:
		return 0, nil, fmt.Errorf("lockstep: unsupported message type %T", msg)
	}
}

func toWireLockStepUnitInfo(info *LockStepUnitInfo) *locksteppb.LockStepUnitInfo {
	if info == nil {
		return nil
	}
	return &locksteppb.LockStepUnitInfo{
		PlayerId: info.PlayerId,
		Position: &mapouterpb.Float3{X: info.Position.X, Y: info.Position.Y, Z: info.Position.Z},
		Rotation: &mapouterpb.Quaternion{
			X: info.Rotation.X,
			Y: info.Rotation.Y,
			Z: info.Rotation.Z,
			W: info.Rotation.W,
		},
	}
}

func fromWireLockStepUnitInfo(info *locksteppb.LockStepUnitInfo) *LockStepUnitInfo {
	if info == nil {
		return nil
	}
	position := info.GetPosition()
	rotation := info.GetRotation()
	result := &LockStepUnitInfo{PlayerId: info.GetPlayerId(), Rotation: etmath.QuaternionIdentity}
	if position != nil {
		result.Position = etmath.Vector3{X: position.GetX(), Y: position.GetY(), Z: position.GetZ()}
	}
	if rotation != nil {
		result.Rotation = etmath.Quaternion{
			X: rotation.GetX(),
			Y: rotation.GetY(),
			Z: rotation.GetZ(),
			W: rotation.GetW(),
		}
	}
	return result
}

func toWireTSVector2(v TSVector2) *locksteppb.TSVector2 {
	return &locksteppb.TSVector2{X: v.X, Y: v.Y}
}

func fromWireTSVector2(v *locksteppb.TSVector2) TSVector2 {
	if v == nil {
		return TSVector2{}
	}
	return TSVector2{X: v.GetX(), Y: v.GetY()}
}

func toWireLSInput(input *LSInput) *locksteppb.LSInput {
	if input == nil {
		return nil
	}
	return &locksteppb.LSInput{V: toWireTSVector2(input.V), Button: input.Button}
}

func fromWireLSInput(input *locksteppb.LSInput) *LSInput {
	if input == nil {
		return nil
	}
	return &LSInput{V: fromWireTSVector2(input.GetV()), Button: input.GetButton()}
}

func toWireActorID(id actor.ActorID) *locksteppb.ActorId {
	return &locksteppb.ActorId{
		ProcessId:  int32(id.ProcessID),
		FiberId:    id.FiberID,
		InstanceId: id.InstanceID,
	}
}

func fromWireActorID(id *locksteppb.ActorId) actor.ActorID {
	if id == nil {
		return actor.ActorID{}
	}
	return actor.ActorID{
		ProcessID:  int(id.GetProcessId()),
		FiberID:    id.GetFiberId(),
		InstanceID: id.GetInstanceId(),
	}
}

func toWireOneFrameInputs(inputs *OneFrameInputs) *locksteppb.OneFrameInputs {
	if inputs == nil {
		return &locksteppb.OneFrameInputs{}
	}
	wireInputs := make(map[int64]*locksteppb.LSInput, len(inputs.Inputs))
	for playerID, input := range inputs.Inputs {
		wireInputs[playerID] = toWireLSInput(input)
	}
	return &locksteppb.OneFrameInputs{Inputs: wireInputs}
}

func fromWireOneFrameInputs(inputs *locksteppb.OneFrameInputs) *OneFrameInputs {
	result := NewOneFrameInputs()
	if inputs == nil {
		return result
	}
	for playerID, input := range inputs.GetInputs() {
		result.Inputs[playerID] = fromWireLSInput(input)
	}
	return result
}
