package move

import "errors"

var (
	// ErrInvalidPath 表示路径点列表无效。
	ErrInvalidPath = errors.New("move: invalid path")
	// ErrInvalidSpeed 表示移动速度无效。
	ErrInvalidSpeed = errors.New("move: invalid speed")
	// ErrMoveCanceled 表示移动被取消。
	ErrMoveCanceled = errors.New("move: move canceled")
	// ErrFinderMissing 表示未配置导航网格实现。
	ErrFinderMissing = errors.New("move: path finder missing")
	// ErrPathfindingFailed 表示导航网格未返回有效路径。
	ErrPathfindingFailed = errors.New("move: pathfinding failed")
	// ErrFinderFactoryMissing 表示配置了导航资源但没有注册 Finder 工厂。
	ErrFinderFactoryMissing = errors.New("move: path finder factory missing")
	// ErrFinderFactoryFailed 表示 Finder 工厂无法加载导航资源。
	ErrFinderFactoryFailed = errors.New("move: path finder factory failed")
)
