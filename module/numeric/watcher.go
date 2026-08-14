package numeric

import (
	"log/slog"
	"runtime/debug"
	"sync"
)

// INumericWatcher 数值变化观察者接口。
type INumericWatcher interface {
	Run(unit any, numType int, old, new_ int64)
}

// WatcherManager 管理按属性类型注册的观察者。
type WatcherManager struct {
	mu       sync.RWMutex
	watchers map[int][]INumericWatcher
}

// DefaultWatcherManager 是默认观察者注册表。
var DefaultWatcherManager = NewWatcherManager()

// NewWatcherManager 创建观察者管理器。
func NewWatcherManager() *WatcherManager {
	return &WatcherManager{
		watchers: make(map[int][]INumericWatcher),
	}
}

// Register 注册指定属性的观察者。
func (m *WatcherManager) Register(numType int, watcher INumericWatcher) {
	if m == nil || watcher == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchers[numType] = append(m.watchers[numType], watcher)
}

// Notify 通知指定属性的所有观察者。
func (m *WatcherManager) Notify(unit any, numType int, old, new_ int64) {
	if m == nil {
		return
	}
	m.mu.RLock()
	watchers := append([]INumericWatcher(nil), m.watchers[numType]...)
	m.mu.RUnlock()

	for _, watcher := range watchers {
		if watcher != nil {
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						slog.Error(
							"numeric watcher panic",
							"num_type", numType,
							"panic", recovered,
							"stack", string(debug.Stack()),
						)
					}
				}()
				watcher.Run(unit, numType, old, new_)
			}()
		}
	}
}

// RegisterWatcher 注册到默认观察者管理器。
func RegisterWatcher(numType int, watcher INumericWatcher) {
	DefaultWatcherManager.Register(numType, watcher)
}
