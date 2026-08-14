package statesync

import (
	etmath "github.com/jerbe/et-go/engine/math"
	etproto "github.com/jerbe/et-go/proto"
	mapouterpb "github.com/jerbe/et-go/proto/mapouterpb"
	gproto "google.golang.org/protobuf/proto"
)

func marshalEnterMap(msg *EnterMap) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.C2M_EnterMap{RpcId: msg.RpcID})
}

func unmarshalEnterMap(data []byte) (*EnterMap, error) {
	wire := &mapouterpb.C2M_EnterMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &EnterMap{RpcID: wire.GetRpcId()}, nil
}

func marshalEnterMapResponse(msg *EnterMapResponse) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_EnterMap{
		RpcId:   msg.RpcID,
		Error:   msg.Error,
		Message: msg.Message,
	})
}

func unmarshalEnterMapResponse(data []byte) (*EnterMapResponse, error) {
	wire := &mapouterpb.M2C_EnterMap{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &EnterMapResponse{
		RpcID:   wire.GetRpcId(),
		Error:   wire.GetError(),
		Message: wire.GetMessage(),
	}, nil
}

func MarshalStartSceneChange(msg *StartSceneChange) ([]byte, error) {
	return marshalStartSceneChange(msg)
}

func marshalStartSceneChange(msg *StartSceneChange) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_StartSceneChange{
		SceneInstanceId: msg.SceneInstanceId,
		SceneName:       msg.SceneName,
	})
}

func unmarshalStartSceneChange(data []byte) (*StartSceneChange, error) {
	wire := &mapouterpb.M2C_StartSceneChange{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &StartSceneChange{
		SceneInstanceId: wire.GetSceneInstanceId(),
		SceneName:       wire.GetSceneName(),
	}, nil
}

func MarshalCreateMyUnit(msg *CreateMyUnit) ([]byte, error) {
	return marshalCreateMyUnit(msg)
}

func marshalCreateMyUnit(msg *CreateMyUnit) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_CreateMyUnit{Unit: toWireUnitInfo(msg.Unit)})
}

func unmarshalCreateMyUnit(data []byte) (*CreateMyUnit, error) {
	wire := &mapouterpb.M2C_CreateMyUnit{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &CreateMyUnit{Unit: fromWireUnitInfo(wire.GetUnit())}, nil
}

func marshalCreateUnits(msg *CreateUnits) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	units := make([]*mapouterpb.UnitInfo, 0, len(msg.Units))
	for _, unitInfo := range msg.Units {
		units = append(units, toWireUnitInfo(unitInfo))
	}
	return gproto.Marshal(&mapouterpb.M2C_CreateUnits{Units: units})
}

func unmarshalCreateUnits(data []byte) (*CreateUnits, error) {
	wire := &mapouterpb.M2C_CreateUnits{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	units := make([]*etproto.UnitInfo, 0, len(wire.GetUnits()))
	for _, unitInfo := range wire.GetUnits() {
		units = append(units, fromWireUnitInfo(unitInfo))
	}
	return &CreateUnits{Units: units}, nil
}

func marshalRemoveUnits(msg *RemoveUnits) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_RemoveUnits{Units: msg.Units})
}

func unmarshalRemoveUnits(data []byte) (*RemoveUnits, error) {
	wire := &mapouterpb.M2C_RemoveUnits{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &RemoveUnits{Units: append([]int64(nil), wire.GetUnits()...)}, nil
}

func marshalPathfindingResultReq(msg *PathfindingResultReq) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.C2M_PathfindingResult{
		RpcId:    msg.RpcID,
		Position: toWireFloat3(msg.Position),
	})
}

func unmarshalPathfindingResultReq(data []byte) (*PathfindingResultReq, error) {
	wire := &mapouterpb.C2M_PathfindingResult{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &PathfindingResultReq{
		RpcID:    wire.GetRpcId(),
		Position: fromWireFloat3(wire.GetPosition()),
	}, nil
}

func marshalStopReq(msg *StopReq) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.C2M_Stop{RpcId: msg.RpcID})
}

func unmarshalStopReq(data []byte) (*StopReq, error) {
	wire := &mapouterpb.C2M_Stop{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &StopReq{RpcID: wire.GetRpcId()}, nil
}

func marshalPathfindingResult(msg *PathfindingResult) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	points := make([]*mapouterpb.Float3, 0, len(msg.Points))
	for _, point := range msg.Points {
		points = append(points, toWireFloat3(point))
	}
	return gproto.Marshal(&mapouterpb.M2C_PathfindingResult{
		Id:       msg.Id,
		Position: toWireFloat3(msg.Position),
		Points:   points,
	})
}

func unmarshalPathfindingResult(data []byte) (*PathfindingResult, error) {
	wire := &mapouterpb.M2C_PathfindingResult{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	points := make([]etmath.Vector3, 0, len(wire.GetPoints()))
	for _, point := range wire.GetPoints() {
		points = append(points, fromWireFloat3(point))
	}
	return &PathfindingResult{
		Id:       wire.GetId(),
		Position: fromWireFloat3(wire.GetPosition()),
		Points:   points,
	}, nil
}

func marshalStop(msg *Stop) ([]byte, error) {
	if msg == nil {
		return nil, ErrMessageNil
	}
	return gproto.Marshal(&mapouterpb.M2C_Stop{
		Error:    msg.Error,
		Id:       msg.Id,
		Position: toWireFloat3(msg.Position),
		Rotation: toWireQuaternion(msg.Rotation),
	})
}

func unmarshalStop(data []byte) (*Stop, error) {
	wire := &mapouterpb.M2C_Stop{}
	if err := gproto.Unmarshal(data, wire); err != nil {
		return nil, err
	}
	return &Stop{
		Error:    wire.GetError(),
		Id:       wire.GetId(),
		Position: fromWireFloat3(wire.GetPosition()),
		Rotation: fromWireQuaternion(wire.GetRotation()),
	}, nil
}

func marshalMessage(msg any) (uint16, []byte, error) {
	switch typed := msg.(type) {
	case *CreateUnits:
		payload, err := marshalCreateUnits(typed)
		return MsgCreateUnits, payload, err
	case *CreateMyUnit:
		payload, err := marshalCreateMyUnit(typed)
		return MsgCreateMyUnit, payload, err
	case *RemoveUnits:
		payload, err := marshalRemoveUnits(typed)
		return MsgRemoveUnits, payload, err
	case *StartSceneChange:
		payload, err := marshalStartSceneChange(typed)
		return MsgStartSceneChange, payload, err
	case *PathfindingResult:
		payload, err := marshalPathfindingResult(typed)
		return MsgPathfindingResult, payload, err
	case *Stop:
		payload, err := marshalStop(typed)
		return MsgStop, payload, err
	default:
		return 0, nil, ErrUnsupportedMessage
	}
}

func toWireUnitInfo(info *etproto.UnitInfo) *mapouterpb.UnitInfo {
	if info == nil {
		return nil
	}
	kv := make(map[int32]int64, len(info.KV))
	for key, value := range info.KV {
		kv[key] = value
	}
	return &mapouterpb.UnitInfo{
		UnitId:   info.UnitId,
		ConfigId: info.ConfigId,
		Type:     info.Type,
		Position: toWireFloat3(info.Position),
		Forward:  toWireFloat3(info.Forward),
		KV:       kv,
		MoveInfo: toWireMoveInfo(info.MoveInfo),
	}
}

func fromWireUnitInfo(info *mapouterpb.UnitInfo) *etproto.UnitInfo {
	if info == nil {
		return nil
	}
	kv := make(map[int32]int64, len(info.GetKV()))
	for key, value := range info.GetKV() {
		kv[key] = value
	}
	return &etproto.UnitInfo{
		UnitId:   info.GetUnitId(),
		ConfigId: info.GetConfigId(),
		Type:     info.GetType(),
		Position: fromWireFloat3(info.GetPosition()),
		Forward:  fromWireFloat3(info.GetForward()),
		KV:       kv,
		MoveInfo: fromWireMoveInfo(info.GetMoveInfo()),
	}
}

func toWireMoveInfo(info *etproto.MoveInfo) *mapouterpb.MoveInfo {
	if info == nil {
		return nil
	}
	points := make([]*mapouterpb.Float3, 0, len(info.Points))
	for _, point := range info.Points {
		points = append(points, toWireFloat3(point))
	}
	return &mapouterpb.MoveInfo{
		Points:    points,
		Rotation:  toWireQuaternion(info.Rotation),
		TurnSpeed: info.TurnSpeed,
	}
}

func fromWireMoveInfo(info *mapouterpb.MoveInfo) *etproto.MoveInfo {
	if info == nil {
		return nil
	}
	points := make([]etmath.Vector3, 0, len(info.GetPoints()))
	for _, point := range info.GetPoints() {
		points = append(points, fromWireFloat3(point))
	}
	return &etproto.MoveInfo{
		Points:    points,
		Rotation:  fromWireQuaternion(info.GetRotation()),
		TurnSpeed: info.GetTurnSpeed(),
	}
}

func toWireFloat3(v etmath.Vector3) *mapouterpb.Float3 {
	return &mapouterpb.Float3{X: v.X, Y: v.Y, Z: v.Z}
}

func fromWireFloat3(v *mapouterpb.Float3) etmath.Vector3 {
	if v == nil {
		return etmath.Vector3{}
	}
	return etmath.Vector3{X: v.GetX(), Y: v.GetY(), Z: v.GetZ()}
}

func toWireQuaternion(v etmath.Quaternion) *mapouterpb.Quaternion {
	return &mapouterpb.Quaternion{X: v.X, Y: v.Y, Z: v.Z, W: v.W}
}

func fromWireQuaternion(v *mapouterpb.Quaternion) etmath.Quaternion {
	if v == nil {
		return etmath.Quaternion{}
	}
	return etmath.Quaternion{X: v.GetX(), Y: v.GetY(), Z: v.GetZ(), W: v.GetW()}
}
