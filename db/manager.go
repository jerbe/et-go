package db

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
)

// DBManagerComponent 管理多个 zone 的数据库组件。
type DBManagerComponent struct {
	ecs.BaseComponent
	mu        sync.RWMutex
	zoneDbs   map[int]*DBComponent
	lock      *coroutinelock.Lock
	cfg       *config.Config
	configDir string
	closed    bool

	clientFactory func(ctx context.Context, uri string, dbName string) (*Client, error)
}

// Type 返回组件类型名称。
func (m *DBManagerComponent) Type() string { return "DBManagerComponent" }

// Awake 初始化内部状态。
func (m *DBManagerComponent) Awake() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	if m.zoneDbs == nil {
		m.zoneDbs = make(map[int]*DBComponent)
	}
	if m.lock == nil {
		m.lock = coroutinelock.New()
	}
	if m.clientFactory == nil {
		m.clientFactory = New
	}
}

// OnDestroy 关闭所有数据库连接。
func (m *DBManagerComponent) OnDestroy() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	zoneDbs := m.zoneDbs
	m.zoneDbs = nil
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for zone, dbComponent := range zoneDbs {
		if dbComponent == nil || dbComponent.client == nil {
			continue
		}
		if err := dbComponent.client.Close(ctx); err != nil {
			slog.Warn("关闭 DB 连接失败", "zone", zone, "err", err)
		}
	}
}

// SetConfig 设置配置对象。
func (m *DBManagerComponent) SetConfig(cfg *config.Config) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.cfg = cfg
	}
}

// SetConfigDir 设置配置目录。
func (m *DBManagerComponent) SetConfigDir(dir string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.configDir = dir
	}
}

// SetLock 设置共享协程锁。
func (m *DBManagerComponent) SetLock(lock *coroutinelock.Lock) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.lock = lock
	}
}

// SetClientFactory 设置 Client 构造函数，便于测试替换。
func (m *DBManagerComponent) SetClientFactory(factory func(ctx context.Context, uri string, dbName string) (*Client, error)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.clientFactory = factory
	}
}

// GetZoneDB 获取指定 zone 的数据库组件。
func (m *DBManagerComponent) GetZoneDB(zone int) (*DBComponent, error) {
	if m == nil {
		return nil, fmt.Errorf("db: manager is nil")
	}
	m.Awake()
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrDBManagerClosed
	}
	if dbComponent, ok := m.zoneDbs[zone]; ok {
		m.mu.RUnlock()
		return dbComponent, nil
	}
	cfg := m.cfg
	configDir := m.configDir
	factory := m.clientFactory
	lock := m.lock
	m.mu.RUnlock()

	if cfg == nil {
		if configDir == "" {
			return nil, fmt.Errorf("db: config not initialized")
		}
		loaded, err := config.Load(configDir)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, ErrDBManagerClosed
		}
		if m.cfg == nil {
			m.cfg = loaded
		}
		cfg = m.cfg
		factory = m.clientFactory
		lock = m.lock
		m.mu.Unlock()
	}

	var zoneCfg *config.StartZoneConfig
	for index := range cfg.Zones {
		if cfg.Zones[index].ID == zone {
			zoneCfg = &cfg.Zones[index]
			break
		}
	}
	if zoneCfg == nil {
		return nil, fmt.Errorf("db: zone config not found: %d", zone)
	}

	if factory == nil {
		factory = New
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := factory(ctx, zoneCfg.DBAddr, zoneCfg.DBName)
	if err != nil {
		return nil, fmt.Errorf("db: create client for zone %d failed: %w", zone, err)
	}

	dbComponent := NewDBComponent(client, lock)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		if closeErr := client.Close(ctx); closeErr != nil {
			slog.Warn("关闭 DB 连接失败", "zone", zone, "err", closeErr)
		}
		return nil, ErrDBManagerClosed
	}
	if existing, ok := m.zoneDbs[zone]; ok {
		m.mu.Unlock()
		if closeErr := client.Close(ctx); closeErr != nil {
			slog.Warn("关闭重复 DB 连接失败", "zone", zone, "err", closeErr)
		}
		return existing, nil
	}
	m.zoneDbs[zone] = dbComponent
	m.mu.Unlock()
	return dbComponent, nil
}
