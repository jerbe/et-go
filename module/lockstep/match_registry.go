package lockstep

import (
	"strings"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

var (
	matchRegistryMu sync.RWMutex
	matchScenes     = make(map[int64]*ecs.Scene)
)

type matchSceneRegistryMarker struct {
	ecs.BaseComponent
	scene *ecs.Scene
}

func (m *matchSceneRegistryMarker) Type() string { return "MatchSceneRegistryMarker" }

func (m *matchSceneRegistryMarker) OnDestroy() {
	if m == nil {
		return
	}
	UnregisterMatchScene(m.scene)
}

// RegisterMatchScene 注册可用匹配场景。
func RegisterMatchScene(scene *ecs.Scene) {
	if scene == nil || scene.IsDisposed() || scene.SceneType() != ecs.SceneTypeMatch {
		return
	}
	matchRegistryMu.Lock()
	matchScenes[scene.InstanceID()] = scene
	matchRegistryMu.Unlock()
	if component, ok := scene.GetComponent("MatchSceneRegistryMarker"); !ok || component == nil {
		scene.AddComponent(&matchSceneRegistryMarker{scene: scene})
	}
}

// UnregisterMatchScene 删除匹配场景引用。
func UnregisterMatchScene(scene *ecs.Scene) {
	if scene == nil {
		return
	}
	matchRegistryMu.Lock()
	delete(matchScenes, scene.InstanceID())
	matchRegistryMu.Unlock()
}

// ResolveMatchScene 返回全局唯一可用匹配场景。
func ResolveMatchScene() (*ecs.Scene, error) {
	return resolveMatchScene(func(*ecs.Scene) bool { return true })
}

// ResolveMatchSceneForZone 按运行时 Zone 选择唯一匹配场景。
func ResolveMatchSceneForZone(zone int) (*ecs.Scene, error) {
	if zone <= 0 {
		return nil, ErrMatchSceneZoneRequired
	}
	return resolveMatchScene(func(scene *ecs.Scene) bool {
		return scene.Zone() == zone
	})
}

// ResolveMatchSceneByName 按 Zone 和场景名称选择唯一匹配场景。
func ResolveMatchSceneByName(zone int, name string) (*ecs.Scene, error) {
	if zone <= 0 {
		return nil, ErrMatchSceneZoneRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrMatchSceneNameRequired
	}
	return resolveMatchScene(func(scene *ecs.Scene) bool {
		return scene.Zone() == zone && strings.EqualFold(scene.Name(), name)
	})
}

func resolveMatchScene(matches func(*ecs.Scene) bool) (*ecs.Scene, error) {
	matchRegistryMu.Lock()
	defer matchRegistryMu.Unlock()
	var result *ecs.Scene
	for instanceID, scene := range matchScenes {
		if scene == nil || scene.IsDisposed() || scene.SceneType() != ecs.SceneTypeMatch {
			delete(matchScenes, instanceID)
			continue
		}
		if matches != nil && !matches(scene) {
			continue
		}
		if result != nil {
			return nil, ErrMatchSceneAmbiguous
		}
		result = scene
	}
	if result == nil {
		return nil, ErrMatchSceneMissing
	}
	return result, nil
}

// PickMatchScene 返回一个可用匹配场景或明确选择错误。
func PickMatchScene() (*ecs.Scene, error) {
	return ResolveMatchScene()
}
