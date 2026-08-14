package config

import "sync"

var (
	globalMu  sync.RWMutex
	globalCfg *Config
)

// SetGlobal 设置全局配置快照。
func SetGlobal(cfg *Config) {
	globalMu.Lock()
	globalCfg = cfg
	globalMu.Unlock()
}

// GetGlobal 返回全局配置快照。
func GetGlobal() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalCfg
}
