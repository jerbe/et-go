package fiber

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/internal/log"
)

// Manager 管理所有 Fiber 的生命周期。
// 对应 ET 框架的 FiberManager。
type Manager struct {
	ctx    context.Context
	cancel context.CancelFunc
	world  *ecs.World
	logger *log.Logger

	mu     sync.RWMutex
	fibers map[int64]*Fiber
	closed bool
}

const stopAllTimeout = 5 * time.Second

// NewManager 创建 FiberManager
func NewManager(ctx context.Context, world *ecs.World, logger *log.Logger) *Manager {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	manager := &Manager{
		ctx:    ctx,
		cancel: cancel,
		world:  world,
		logger: logger,
		fibers: make(map[int64]*Fiber),
	}
	SetDefaultManager(manager)
	return manager
}

// Create 创建并启动一个新 Fiber
func (m *Manager) Create(sceneType ecs.SceneType, zone int, processID int, handler MessageHandler) *Fiber {
	return m.create(sceneType, zone, processID, 0, "", nil, handler)
}

// CreateWithSetup 创建 Fiber，并在启动 loop 前同步执行 setup。
//
// setup 适用于动态 Fiber 的运行时依赖注入。它执行完成后 Fiber 才会
// 注册到 Manager 并启动 goroutine，调用方不需要与初始化/销毁并发访问
// 根 Scene。
func (m *Manager) CreateWithSetup(sceneType ecs.SceneType, zone int, processID int, setup func(*Fiber) error, handler MessageHandler) *Fiber {
	return m.create(sceneType, zone, processID, 0, "", setup, handler)
}

// CreateConfigured 按启动配置创建 Fiber。
// Scene ID 和 Name 会在业务 FiberInitHandler 执行前写入根 Scene，
// 这样初始化阶段的网络地址解析和 Actor registry 不会使用临时默认值。
func (m *Manager) CreateConfigured(sceneType ecs.SceneType, zone int, processID int, sceneID int64, sceneName string, handler MessageHandler) *Fiber {
	if sceneID <= 0 || sceneName == "" {
		m.logError("Fiber 配置无效", "type", sceneType.String(), "zone", zone, "scene_id", sceneID, "scene_name", sceneName)
		return nil
	}
	return m.create(sceneType, zone, processID, sceneID, sceneName, nil, handler)
}

func (m *Manager) create(sceneType ecs.SceneType, zone int, processID int, sceneID int64, sceneName string, setup func(*Fiber) error, handler MessageHandler) *Fiber {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	closed := m.closed
	m.mu.RUnlock()
	if closed {
		m.logWarn("拒绝创建 Fiber：Manager 已关闭", "type", sceneType.String(), "zone", zone)
		return nil
	}

	f := New(m.ctx, sceneType, zone, processID)
	f.manager = m
	if sceneID > 0 {
		f.Root().SetID(sceneID)
	}
	if sceneName != "" {
		f.Root().SetName(sceneName)
	}

	if initHandler, ok := getFiberInit(sceneType); ok && initHandler != nil {
		if err := initHandler(f); err != nil {
			m.logError("Fiber 初始化失败", "type", sceneType.String(), "zone", zone, "err", err)
			m.discardFiber(f)
			return nil
		}
	}
	if setup != nil {
		if err := setup(f); err != nil {
			m.logError("Fiber 依赖注入失败", "type", sceneType.String(), "zone", zone, "err", err)
			m.discardFiber(f)
			return nil
		}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.discardFiber(f)
		m.logWarn("拒绝注册 Fiber：Manager 已关闭", "id", f.ID())
		return nil
	}
	m.fibers[f.ID()] = f
	m.mu.Unlock()

	m.logInfo("Fiber 创建", "id", f.ID(), "type", sceneType.String(), "zone", zone)
	f.Run(handler)
	return f
}

func (m *Manager) discardFiber(f *Fiber) {
	if f == nil {
		return
	}
	f.RequestStop()
	if f.started.Load() {
		if !f.Wait(time.Second) {
			m.logWarn("丢弃 Fiber 停止超时", "id", f.ID(), "wait", time.Second.String())
		}
		return
	}
	f.dispose(nil)
	f.closeDone()
}

// Get 根据 ID 获取 Fiber
func (m *Manager) Get(id int64) (*Fiber, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.fibers[id]
	return f, ok
}

// Remove 移除并停止 Fiber
func (m *Manager) Remove(id int64) {
	m.mu.Lock()
	f, ok := m.fibers[id]
	if ok {
		delete(m.fibers, id)
	}
	m.mu.Unlock()

	if ok && f != nil {
		f.RequestStop()
		if f.Wait(time.Second) {
			m.logInfo("Fiber 移除", "id", id)
		} else {
			m.logWarn("Fiber 移除等待超时", "id", id, "wait", time.Second.String())
		}
	}
}

// StopAll 停止所有 Fiber
func (m *Manager) StopAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		clearDefaultManager(m)
		return
	}
	m.closed = true
	m.mu.Unlock()

	m.cancel()

	m.mu.Lock()
	fibers := make([]*Fiber, 0, len(m.fibers))
	for _, f := range m.fibers {
		if f != nil {
			fibers = append(fibers, f)
		}
	}
	m.mu.Unlock()

	for _, f := range fibers {
		f.RequestStop()
	}

	deadline := time.Now().Add(stopAllTimeout)
	for _, f := range fibers {
		remain := time.Until(deadline)
		if remain <= 0 {
			m.logWarn("StopAll 等待超时", "timeout", stopAllTimeout.String())
			break
		}
		if !f.Wait(remain) {
			m.logWarn("Fiber 停止超时", "id", f.ID(), "wait", remain.String())
		}
	}

	m.mu.Lock()
	m.fibers = make(map[int64]*Fiber)
	m.mu.Unlock()
	clearDefaultManager(m)
}

// Count 返回当前 Fiber 数量
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.fibers)
}

func (m *Manager) logInfo(message string, args ...any) {
	if m.logger != nil {
		m.logger.Info(message, args...)
		return
	}
	slog.Info(message, args...)
}

func (m *Manager) logWarn(message string, args ...any) {
	if m.logger != nil {
		m.logger.Warn(message, args...)
		return
	}
	slog.Warn(message, args...)
}

func (m *Manager) logError(message string, args ...any) {
	if m.logger != nil {
		m.logger.Error(message, args...)
		return
	}
	slog.Error(message, args...)
}
