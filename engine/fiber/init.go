package fiber

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// FiberInitHandler 定义 Fiber 初始化函数签名。
type FiberInitHandler func(f *Fiber) error

var (
	initMu       sync.RWMutex
	initHandlers = make(map[ecs.SceneType]FiberInitHandler)
)

// RegisterFiberInit 注册指定场景类型的 Fiber 初始化函数。
func RegisterFiberInit(sceneType ecs.SceneType, handler FiberInitHandler) {
	initMu.Lock()
	defer initMu.Unlock()
	if handler == nil {
		delete(initHandlers, sceneType)
		return
	}
	initHandlers[sceneType] = handler
}

func getFiberInit(sceneType ecs.SceneType) (FiberInitHandler, bool) {
	initMu.RLock()
	defer initMu.RUnlock()
	handler, ok := initHandlers[sceneType]
	return handler, ok
}
