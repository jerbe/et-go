package crontab

import (
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// ICrontabHandler 定义定时任务执行器接口。
type ICrontabHandler interface {
	Handle(scene *ecs.Scene, task *CrontabTask) error
}

var (
	handlerMu sync.RWMutex
	handlers  = make(map[int]ICrontabHandler)
)

// RegisterCrontabHandler 注册对应 invokeType 的处理器，后注册者覆盖前者。
func RegisterCrontabHandler(invokeType int, handler ICrontabHandler) {
	handlerMu.Lock()
	if handler == nil {
		delete(handlers, invokeType)
	} else {
		handlers[invokeType] = handler
	}
	handlerMu.Unlock()
}

func getHandler(invokeType int) ICrontabHandler {
	handlerMu.RLock()
	defer handlerMu.RUnlock()
	return handlers[invokeType]
}
