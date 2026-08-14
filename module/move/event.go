package move

import "github.com/jerbe/et-go/engine/event"

const (
	// EventMoveStart 表示开始移动事件。
	EventMoveStart event.EventID = "move.Start"
	// EventMoveStop 表示停止移动事件。
	EventMoveStop event.EventID = "move.Stop"
)

// MoveStart 表示移动开始事件。
type MoveStart struct {
	Unit any
}

// MoveStop 表示移动停止事件。
type MoveStop struct {
	Unit any
}
