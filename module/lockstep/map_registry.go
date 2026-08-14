package lockstep

import (
	"strings"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

var (
	mapRegistryMu sync.RWMutex
	mapScenes     = make(map[int64]*ecs.Scene)
)

type mapSceneRegistryMarker struct {
	ecs.BaseComponent
	scene *ecs.Scene
}

func (m *mapSceneRegistryMarker) Type() string { return "MapSceneRegistryMarker" }

func (m *mapSceneRegistryMarker) OnDestroy() {
	if m == nil {
		return
	}
	UnregisterMapScene(m.scene)
}

// RegisterMapScene 注册可用地图场景。
func RegisterMapScene(scene *ecs.Scene) {
	if scene == nil || scene.IsDisposed() || scene.SceneType() != ecs.SceneTypeMap {
		return
	}
	mapRegistryMu.Lock()
	mapScenes[scene.InstanceID()] = scene
	mapRegistryMu.Unlock()
	if component, ok := scene.GetComponent("MapSceneRegistryMarker"); !ok || component == nil {
		scene.AddComponent(&mapSceneRegistryMarker{scene: scene})
	}
}

// UnregisterMapScene 删除地图场景引用。
func UnregisterMapScene(scene *ecs.Scene) {
	if scene == nil {
		return
	}
	mapRegistryMu.Lock()
	delete(mapScenes, scene.InstanceID())
	mapRegistryMu.Unlock()
}

// ResolveMapScene 返回全局唯一可用地图场景。
func ResolveMapScene() (*ecs.Scene, error) {
	return resolveMapScene(func(*ecs.Scene) bool { return true })
}

// ResolveMapSceneForZone 按运行时 Zone 选择唯一地图场景。
//
// 该函数只使用显式 Zone 约束；同一 Zone 存在多个地图时仍返回
// ErrMapSceneAmbiguous，不按 Go map 遍历顺序猜测目标。
func ResolveMapSceneForZone(zone int) (*ecs.Scene, error) {
	if zone <= 0 {
		return nil, ErrMapSceneZoneRequired
	}
	return resolveMapScene(func(scene *ecs.Scene) bool {
		return scene.Zone() == zone
	})
}

// ResolveDefaultMapSceneForZone 按默认出生地图规则选择地图。
//
// 约定名为 Home 的地图优先作为默认地图；没有 Home 时，只有 Zone
// 内恰好一张地图才允许自动选择，避免在多地图部署中随机路由。
func ResolveDefaultMapSceneForZone(zone int) (*ecs.Scene, error) {
	if zone <= 0 {
		return nil, ErrMapSceneZoneRequired
	}
	if scene, err := ResolveMapSceneByName(zone, "Home"); err == nil {
		return scene, nil
	} else if err != ErrMapSceneMissing {
		return nil, err
	}
	return ResolveMapSceneForZone(zone)
}

// ResolveMapSceneByName 按 Zone 和场景名称选择唯一地图场景。
func ResolveMapSceneByName(zone int, name string) (*ecs.Scene, error) {
	if zone <= 0 {
		return nil, ErrMapSceneZoneRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrMapSceneNameRequired
	}
	return resolveMapScene(func(scene *ecs.Scene) bool {
		return scene.Zone() == zone && strings.EqualFold(scene.Name(), name)
	})
}

func resolveMapScene(matches func(*ecs.Scene) bool) (*ecs.Scene, error) {
	mapRegistryMu.Lock()
	defer mapRegistryMu.Unlock()
	var result *ecs.Scene
	for instanceID, scene := range mapScenes {
		if scene == nil || scene.IsDisposed() || scene.SceneType() != ecs.SceneTypeMap {
			delete(mapScenes, instanceID)
			continue
		}
		if matches != nil && !matches(scene) {
			continue
		}
		if result != nil {
			return nil, ErrMapSceneAmbiguous
		}
		result = scene
	}
	if result == nil {
		return nil, ErrMapSceneMissing
	}
	return result, nil
}

// PickMapScene 返回一个可用地图场景或明确选择错误。
func PickMapScene() (*ecs.Scene, error) {
	return ResolveMapScene()
}
