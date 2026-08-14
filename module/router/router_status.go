package router

// RouterStatus 表示 RouterNode 的状态。
type RouterStatus int

const (
	RouterStatusSync RouterStatus = 0
	RouterStatusMsg  RouterStatus = 1
)

func (s RouterStatus) String() string {
	switch s {
	case RouterStatusSync:
		return "Sync"
	case RouterStatusMsg:
		return "Msg"
	default:
		return "Unknown"
	}
}
