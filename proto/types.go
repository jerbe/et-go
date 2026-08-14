package proto

import etmath "github.com/jerbe/et-go/engine/math"

// MoveInfo 表示单位当前移动状态。
type MoveInfo struct {
	Points    []etmath.Vector3
	Rotation  etmath.Quaternion
	TurnSpeed int32
}

// UnitInfo 表示同步给客户端的单位信息。
type UnitInfo struct {
	UnitId   int64
	ConfigId int32
	Type     int32
	Position etmath.Vector3
	Forward  etmath.Vector3
	KV       map[int32]int64
	MoveInfo *MoveInfo
}
