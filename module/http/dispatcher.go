package http

import (
	"log/slog"
	nethttp "net/http"
	"sync"

	"github.com/jerbe/et-go/engine/ecs"
)

// IHttpHandler 表示 HTTP 处理器接口。
type IHttpHandler interface {
	Handle(scene *ecs.Scene, req *nethttp.Request, resp nethttp.ResponseWriter) error
}

// HttpDispatcher 管理 path 到 handler 的映射。
type HttpDispatcher struct {
	mu       sync.RWMutex
	handlers map[string]IHttpHandler
}

// Register 注册处理器。
func (d *HttpDispatcher) Register(path string, handler IHttpHandler) {
	if path == "" || handler == nil {
		return
	}
	d.mu.Lock()
	if d.handlers == nil {
		d.handlers = make(map[string]IHttpHandler)
	}
	d.handlers[path] = handler
	d.mu.Unlock()
}

// Dispatch 将请求分发到指定处理器。
func (d *HttpDispatcher) Dispatch(scene *ecs.Scene, w nethttp.ResponseWriter, r *nethttp.Request) {
	if d == nil {
		WriteInternalServerError(w)
		return
	}
	if r == nil || r.URL == nil {
		WriteError(w, nethttp.StatusOK, nethttp.StatusBadRequest, "400 Bad Request")
		return
	}

	d.mu.RLock()
	handler := d.handlers[r.URL.Path]
	d.mu.RUnlock()
	if handler == nil {
		WriteNotFound(w)
		return
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("HTTP handler panic", "path", r.URL.Path, "panic", recovered)
			WriteInternalServerError(w)
		}
	}()

	if err := handler.Handle(scene, r, w); err != nil {
		WriteInternalServerError(w)
	}
}
