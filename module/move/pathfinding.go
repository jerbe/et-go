package move

import (
	"fmt"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
	etmath "github.com/jerbe/et-go/engine/math"
)

const (
	// NavExtentX 是导航搜索范围 X。
	NavExtentX = 15.0
	// NavExtentY 是导航搜索范围 Y。
	NavExtentY = 10.0
	// NavExtentZ 是导航搜索范围 Z。
	NavExtentZ = 15.0
	// NavMaxPolys 是最大多边形数量。
	NavMaxPolys = 256
)

// Finder 抽象导航网格寻路器。
type Finder interface {
	FindPath(start, target, extents etmath.Vector3, maxPolys int) ([]etmath.Vector3, error)
}

// FinderFactory 从地图名和导航网格资源创建真实 Finder。
//
// Go 核心不内置 DotRecast 实现，部署层必须显式注册兼容工厂；没有工厂时
// 启动或寻路会返回明确错误，绝不生成直线或空路径。
// TODO(map): 固定生产 navmesh 格式、坐标系和 Finder 工厂实现。
type FinderFactory func(mapName, navMeshFile string) (Finder, error)

var (
	finderFactoryMu sync.RWMutex
	finderFactory   FinderFactory
)

// RegisterFinderFactory 注册部署层导航网格工厂。
func RegisterFinderFactory(factory FinderFactory) {
	finderFactoryMu.Lock()
	finderFactory = factory
	finderFactoryMu.Unlock()
}

// NewFinder 根据配置创建真实导航 Finder。
func NewFinder(mapName, navMeshFile string) (Finder, error) {
	finderFactoryMu.RLock()
	factory := finderFactory
	finderFactoryMu.RUnlock()
	if factory == nil {
		return nil, ErrFinderFactoryMissing
	}
	finder, err := factory(mapName, navMeshFile)
	if err != nil {
		return nil, fmt.Errorf("%w: map=%q navmesh=%q: %v", ErrFinderFactoryFailed, mapName, navMeshFile, err)
	}
	if finder == nil {
		return nil, fmt.Errorf("%w: map=%q navmesh=%q returned nil finder", ErrFinderFactoryFailed, mapName, navMeshFile)
	}
	return finder, nil
}

// PathfindingComponent 导航寻路组件。
type PathfindingComponent struct {
	ecs.BaseComponent
	MapName  string
	extents  etmath.Vector3
	maxPolys int
	finder   Finder
}

// Type 返回组件名称。
func (c *PathfindingComponent) Type() string { return "PathfindingComponent" }

// Awake 初始化默认参数。
func (c *PathfindingComponent) Awake() {
	if c == nil {
		return
	}
	if c.extents == etmath.Vector3Zero {
		c.extents = etmath.Vector3{X: NavExtentX, Y: NavExtentY, Z: NavExtentZ}
	}
	if c.maxPolys == 0 {
		c.maxPolys = NavMaxPolys
	}
}

// SetFinder 设置导航求解器。
func (c *PathfindingComponent) SetFinder(finder Finder) {
	if c == nil {
		return
	}
	c.finder = finder
}

// Finder 返回当前注入的导航实现。
func (c *PathfindingComponent) Finder() Finder {
	if c == nil {
		return nil
	}
	return c.finder
}

// NewPathfindingComponentForScene 创建带场景 Finder 的寻路组件。
func NewPathfindingComponentForScene(scene *ecs.Scene, mapName string) *PathfindingComponent {
	component := &PathfindingComponent{MapName: mapName}
	if scene == nil {
		return component
	}
	if provider, ok := scene.GetComponent("MapUnitManagerComponent"); ok && provider != nil {
		if finderProvider, ok := provider.(interface{ PathfindingFinder() Finder }); ok {
			component.SetFinder(finderProvider.PathfindingFinder())
		}
	}
	return component
}

// Find 计算路径点列表。
func (c *PathfindingComponent) Find(start, target etmath.Vector3) ([]etmath.Vector3, error) {
	if c == nil {
		return nil, ErrFinderMissing
	}
	c.Awake()
	convertedStart := etmath.Vector3{X: -start.X, Y: start.Y, Z: start.Z}
	convertedTarget := etmath.Vector3{X: -target.X, Y: target.Y, Z: target.Z}
	if c.finder == nil {
		return nil, ErrFinderMissing
	}
	path, err := c.finder.FindPath(convertedStart, convertedTarget, c.extents, c.maxPolys)
	if err != nil {
		return nil, err
	}
	if len(path) < 2 {
		return nil, ErrPathfindingFailed
	}
	result := make([]etmath.Vector3, 0, len(path))
	for _, point := range path {
		result = append(result, etmath.Vector3{X: -point.X, Y: point.Y, Z: point.Z})
	}
	return result, nil
}
