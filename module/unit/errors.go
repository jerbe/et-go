package unit

import "errors"

var (
	// ErrSceneMissing 表示单位创建缺少 Scene。
	ErrSceneMissing = errors.New("unit: scene missing")
	// ErrUnitComponentMissing 表示地图没有挂载 UnitComponent。
	ErrUnitComponentMissing = errors.New("unit: unit component missing")
	// ErrLocationProxyMissing 表示地图没有挂载 LocationProxyComponent。
	ErrLocationProxyMissing = errors.New("unit: location proxy missing")
	// ErrAOIManagerMissing 表示地图没有挂载 AOIManagerComponent。
	ErrAOIManagerMissing = errors.New("unit: aoi manager missing")
	// ErrInvalidUnitID 表示单位 ID 非法。
	ErrInvalidUnitID = errors.New("unit: invalid unit id")
	// ErrUnitComponentClosed 表示单位注册表已经关闭。
	ErrUnitComponentClosed = errors.New("unit: unit component closed")
	// ErrUnitAlreadyExists 表示单位 ID 已经被其他单位占用。
	ErrUnitAlreadyExists = errors.New("unit: unit already exists")
	// ErrLocationRegistration 表示单位 Location 注册失败。
	ErrLocationRegistration = errors.New("unit: location registration failed")
)
