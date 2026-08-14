package ecs

import "strings"

// EntityStatus 表示实体的状态标志位。
type EntityStatus uint8

const (
	// StatusNone 表示无状态。
	StatusNone EntityStatus = 0
	// StatusIsFromPool 表示实体来自对象池。
	StatusIsFromPool EntityStatus = 1
	// StatusIsRegister 表示实体已注册到 Scene。
	StatusIsRegister EntityStatus = 1 << 1
	// StatusIsComponent 表示实体作为组件附加。
	StatusIsComponent EntityStatus = 1 << 2
	// StatusIsNew 表示实体为新建状态。
	StatusIsNew EntityStatus = 1 << 3
	// StatusIsSerializeWithParent 表示实体随父实体一起序列化。
	StatusIsSerializeWithParent EntityStatus = 1 << 4
)

// Set 返回设置指定标志位后的状态。
func (s EntityStatus) Set(flag EntityStatus) EntityStatus {
	return s | flag
}

// Has 返回是否包含指定标志位。
func (s EntityStatus) Has(flag EntityStatus) bool {
	return s&flag == flag
}

// Clear 返回清除指定标志位后的状态。
func (s EntityStatus) Clear(flag EntityStatus) EntityStatus {
	return s &^ flag
}

// String 返回状态的可读字符串。
func (s EntityStatus) String() string {
	if s == StatusNone {
		return "StatusNone"
	}

	parts := make([]string, 0, 5)
	if s.Has(StatusIsFromPool) {
		parts = append(parts, "StatusIsFromPool")
	}
	if s.Has(StatusIsRegister) {
		parts = append(parts, "StatusIsRegister")
	}
	if s.Has(StatusIsComponent) {
		parts = append(parts, "StatusIsComponent")
	}
	if s.Has(StatusIsNew) {
		parts = append(parts, "StatusIsNew")
	}
	if s.Has(StatusIsSerializeWithParent) {
		parts = append(parts, "StatusIsSerializeWithParent")
	}
	if len(parts) == 0 {
		return "StatusNone"
	}
	return strings.Join(parts, "|")
}
