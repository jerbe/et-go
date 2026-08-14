package fiber

import "sync"

var (
	defaultManagerMu sync.RWMutex
	defaultManager   *Manager
)

// SetDefaultManager 设置默认 FiberManager。
func SetDefaultManager(manager *Manager) {
	defaultManagerMu.Lock()
	defaultManager = manager
	defaultManagerMu.Unlock()
}

// DefaultManager 返回默认 FiberManager。
func DefaultManager() *Manager {
	defaultManagerMu.RLock()
	defer defaultManagerMu.RUnlock()
	return defaultManager
}

func clearDefaultManager(manager *Manager) {
	defaultManagerMu.Lock()
	if defaultManager == manager {
		defaultManager = nil
	}
	defaultManagerMu.Unlock()
}
