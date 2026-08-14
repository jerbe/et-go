package actor

import (
	"sort"
	"strings"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// SceneRef 描述一个运行中的场景引用。
type SceneRef struct {
	SceneType ecs.SceneType
	Zone      int
	SceneID   int64
	Name      string
	ActorID   ActorID
	Scene     *ecs.Scene
}

var (
	sceneRegistryMu sync.RWMutex
	sceneRegistry   = map[int64]SceneRef{}
)

// SceneActorID 返回场景根实体对应的 ActorID。
func SceneActorID(scene *ecs.Scene) ActorID {
	if scene == nil {
		return ActorID{}
	}
	if fiberRef, ok := scene.Fiber().(interface {
		ID() int64
		ProcessID() int
	}); ok {
		return ActorID{
			ProcessID:  fiberRef.ProcessID(),
			FiberID:    fiberRef.ID(),
			InstanceID: scene.InstanceID(),
		}
	}
	return ActorID{}
}

// UpdateSceneRegistry 注册或刷新场景引用。
func UpdateSceneRegistry(scene *ecs.Scene) {
	if scene == nil || scene.IsDisposed() {
		return
	}
	actorID := SceneActorID(scene)
	if !actorID.IsValid() {
		return
	}

	sceneRegistryMu.Lock()
	sceneRegistry[scene.InstanceID()] = SceneRef{
		SceneType: scene.SceneType(),
		Zone:      scene.Zone(),
		SceneID:   scene.ID(),
		Name:      scene.Name(),
		ActorID:   actorID,
		Scene:     scene,
	}
	sceneRegistryMu.Unlock()
}

// RemoveSceneRegistry 删除场景引用。
func RemoveSceneRegistry(scene *ecs.Scene) {
	if scene == nil {
		return
	}
	sceneRegistryMu.Lock()
	delete(sceneRegistry, scene.InstanceID())
	sceneRegistryMu.Unlock()
}

// ResolveSceneActor 返回唯一匹配场景的 ActorID。
//
// name 为空时必须恰好匹配一个场景；多个候选不会再按排序结果
// 静默选择第一个，调用方必须显式提供名称或处理歧义。
func ResolveSceneActor(zone int, sceneType ecs.SceneType, name string) (ActorID, bool) {
	refs := ResolveSceneActors(zone, sceneType)
	if len(refs) == 0 {
		return ActorID{}, false
	}
	if name == "" {
		if len(refs) != 1 {
			return ActorID{}, false
		}
		return refs[0].ActorID, true
	}
	lowerName := strings.ToLower(strings.TrimSpace(name))
	var match *SceneRef
	for _, ref := range refs {
		if strings.ToLower(strings.TrimSpace(ref.Name)) == lowerName {
			if match != nil {
				return ActorID{}, false
			}
			current := ref
			match = &current
		}
	}
	if match != nil {
		return match.ActorID, true
	}
	return ActorID{}, false
}

// ResolveSceneActors 返回匹配条件的场景列表。
func ResolveSceneActors(zone int, sceneType ecs.SceneType) []SceneRef {
	sceneRegistryMu.Lock()
	defer sceneRegistryMu.Unlock()

	refs := make([]SceneRef, 0)
	for instanceID, ref := range sceneRegistry {
		if ref.Scene == nil || ref.Scene.IsDisposed() {
			delete(sceneRegistry, instanceID)
			continue
		}
		if zone > 0 && ref.Zone != zone {
			continue
		}
		if sceneType != ecs.SceneTypeNone && ref.SceneType != sceneType {
			continue
		}
		if !ref.ActorID.IsValid() {
			continue
		}
		refs = append(refs, ref)
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].SceneID != refs[j].SceneID {
			if refs[i].SceneID == 0 {
				return false
			}
			if refs[j].SceneID == 0 {
				return true
			}
			return refs[i].SceneID < refs[j].SceneID
		}
		if refs[i].Name != refs[j].Name {
			return refs[i].Name < refs[j].Name
		}
		if refs[i].ActorID.ProcessID != refs[j].ActorID.ProcessID {
			return refs[i].ActorID.ProcessID < refs[j].ActorID.ProcessID
		}
		return refs[i].ActorID.FiberID < refs[j].ActorID.FiberID
	})
	return refs
}
