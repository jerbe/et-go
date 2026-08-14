package numeric

import "github.com/jerbe/et-go/engine/event"

// EventNumericChange 表示最终属性变化事件。
const EventNumericChange event.EventID = "numeric.Change"

// NumericChange 表示数值变化通知。
type NumericChange struct {
	Unit     any
	Type     int
	OldValue int64
	NewValue int64
}
