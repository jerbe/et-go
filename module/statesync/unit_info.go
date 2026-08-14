package statesync

import (
	etmath "github.com/jerbe/et-go/engine/math"
	"github.com/jerbe/et-go/module/unit"
	"github.com/jerbe/et-go/proto"
)

const (
	// MsgC2MEnterMap 表示进入地图请求。
	MsgC2MEnterMap uint16 = 2501
	// MsgM2CEnterMap 表示进入地图响应。
	MsgM2CEnterMap uint16 = 2502
	// MsgCreateUnits 表示批量创建单位消息。
	MsgCreateUnits uint16 = 3005
	// MsgCreateMyUnit 表示创建自身单位消息。
	MsgCreateMyUnit uint16 = 3006
	// MsgRemoveUnits 表示批量移除单位消息。
	MsgRemoveUnits uint16 = 3008
	// MsgStartSceneChange 表示场景切换通知。
	MsgStartSceneChange uint16 = 3007
	// MsgPathfindingResult 表示广播路径消息。
	MsgPathfindingResult uint16 = 3011
	// MsgStop 表示广播停止消息。
	MsgStop uint16 = 3012
	// MsgC2MPathfindingResult 表示客户端寻路请求。
	MsgC2MPathfindingResult uint16 = 3009
	// MsgC2MStop 表示客户端停止请求。
	MsgC2MStop uint16 = 3010
	// MsgC2MTransferMap 表示客户端切图请求。
	MsgC2MTransferMap uint16 = 3021
	// MsgM2CTransferMap 表示客户端切图响应。
	MsgM2CTransferMap uint16 = 3022
)

// UnitInfo 对齐客户端单位同步结构。
type UnitInfo = proto.UnitInfo

// MoveInfo 对齐客户端移动同步结构。
type MoveInfo = proto.MoveInfo

// StartSceneChange 用于通知客户端切换地图。
type StartSceneChange struct {
	SceneInstanceId int64
	SceneName       string
}

// CreateMyUnit 包含当前玩家 Unit 信息。
type CreateMyUnit struct {
	Unit *proto.UnitInfo
}

// CreateUnits 表示批量创建单位消息。
type CreateUnits struct {
	Units []*proto.UnitInfo
}

// RemoveUnits 表示批量移除单位消息。
type RemoveUnits struct {
	Units []int64
}

// PathfindingResultReq 表示客户端寻路请求。
type PathfindingResultReq struct {
	RpcID    uint32
	Position etmath.Vector3
}

// StopReq 表示客户端请求停止移动。
type StopReq struct {
	RpcID uint32
}

// PathfindingResult 表示服务器广播路径结果。
type PathfindingResult struct {
	Id       int64
	Position etmath.Vector3
	Points   []etmath.Vector3
}

// Stop 表示服务器广播停止结果。
type Stop struct {
	Error    int32
	Id       int64
	Position etmath.Vector3
	Rotation etmath.Quaternion
}

// EnterMap 表示进入地图请求。
type EnterMap struct {
	RpcID uint32
}

// EnterMapResponse 表示进入地图响应。
type EnterMapResponse struct {
	RpcID   uint32
	Error   int32
	Message string
}

// NewStartSceneChange 构建 SceneChange 消息。
func NewStartSceneChange(sceneID int64, name string) *StartSceneChange {
	return &StartSceneChange{SceneInstanceId: sceneID, SceneName: name}
}

// NewCreateMyUnit 构建 CreateMyUnit 消息。
func NewCreateMyUnit(u *unit.Unit) *CreateMyUnit {
	return &CreateMyUnit{Unit: CreateUnitInfo(u)}
}

// CreateUnitInfo 包装 unit.CreateUnitInfo，方便其他模块引用。
func CreateUnitInfo(u *unit.Unit) *proto.UnitInfo {
	if u == nil {
		return nil
	}
	return unit.CreateUnitInfo(u)
}
