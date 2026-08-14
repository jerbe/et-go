package crontab

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

// CrontabTask 描述一个具体的定时任务。
type CrontabTask struct {
	ecs.Entity

	Name           string
	CronExpression string
	InvokeType     int
	IsRunning      bool
	LastRunTime    *time.Time

	schedule *cronSchedule
}

// CrontabComponent 管理一组 CrontabTask。
type CrontabComponent struct {
	ecs.BaseComponent

	tasks       map[string]*CrontabTask
	lastMinute  int
	nowFunc     func() time.Time
	closed      bool
	registered  bool
	initialized bool
}

// Type 返回组件名称。
func (c *CrontabComponent) Type() string { return "CrontabComponent" }

// Awake 初始化内部状态。
func (c *CrontabComponent) Awake() {
	if c == nil || c.closed {
		return
	}
	if c.tasks == nil {
		c.tasks = make(map[string]*CrontabTask)
	}
	if !c.initialized {
		c.lastMinute = -1
		c.initialized = true
	}
	if c.registered {
		return
	}
	entity := c.GetEntity()
	if entity == nil || entity.Scene() == nil {
		return
	}
	if registrar, ok := entity.Scene().Fiber().(interface {
		RegisterUpdate(ecs.UpdateSystem)
	}); ok && registrar != nil {
		registrar.RegisterUpdate(c)
		c.registered = true
	}
}

// OnDestroy 清理任务列表。
func (c *CrontabComponent) OnDestroy() {
	if c == nil || c.closed {
		return
	}
	c.closed = true
	registered := c.registered
	c.registered = false
	for name, task := range c.tasks {
		if task != nil {
			task.IsRunning = false
		}
		delete(c.tasks, name)
	}
	c.tasks = nil
	if registered {
		if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
			if registrar, ok := entity.Scene().Fiber().(interface {
				UnregisterUpdate(ecs.UpdateSystem)
			}); ok && registrar != nil {
				registrar.UnregisterUpdate(c)
			}
		}
	}
}

// AddTask 注册新的 CrontabTask。
func (c *CrontabComponent) AddTask(task *CrontabTask) error {
	if c == nil || c.closed {
		return ErrComponentClosed
	}
	if task == nil || task.Name == "" {
		return ErrInvalidCronExpression
	}
	if c.tasks == nil {
		c.Awake()
	}
	if _, ok := c.tasks[task.Name]; ok {
		return ErrTaskAlreadyExists
	}
	schedule, err := parseCron(task.CronExpression)
	if err != nil {
		return err
	}
	task.schedule = schedule
	c.tasks[task.Name] = task
	return nil
}

// RemoveTask 删除任务。
func (c *CrontabComponent) RemoveTask(name string) error {
	if c == nil || c.closed {
		return ErrComponentClosed
	}
	if c.tasks == nil {
		return ErrTaskNotFound
	}
	if _, ok := c.tasks[name]; !ok {
		return ErrTaskNotFound
	}
	delete(c.tasks, name)
	return nil
}

// GetTask 查询任务。
func (c *CrontabComponent) GetTask(name string) (*CrontabTask, bool) {
	if c == nil || c.closed || c.tasks == nil {
		return nil, false
	}
	task, ok := c.tasks[name]
	return task, ok
}

// SetNowFunc 用于测试注入时间源。
func (c *CrontabComponent) SetNowFunc(fn func() time.Time) {
	if c == nil || c.closed {
		return
	}
	c.nowFunc = fn
}

func (c *CrontabComponent) now() time.Time {
	if c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

// Update 实现 ecs.UpdateSystem 接口。
func (c *CrontabComponent) Update() {
	if c == nil || c.closed {
		return
	}
	now := c.now()
	minute := now.Minute()
	if c.lastMinute == minute {
		return
	}
	c.lastMinute = minute
	scene := c.scene()
	for _, task := range c.tasks {
		if task == nil || task.schedule == nil {
			continue
		}
		if task.IsRunning {
			slog.Warn("定时任务跳过：仍在执行", "task", task.Name)
			continue
		}
		if !task.schedule.Match(now) {
			continue
		}
		handler := getHandler(task.InvokeType)
		if handler == nil {
			slog.Error("定时任务缺少 handler", "task", task.Name, "invokeType", task.InvokeType)
			continue
		}
		task.IsRunning = true
		start := now
		err := safeInvoke(handler, scene, task)
		task.IsRunning = false
		task.LastRunTime = &start
		if err != nil {
			slog.Error("定时任务执行失败", "task", task.Name, "error", err)
			continue
		}
		slog.Info("定时任务执行成功", "task", task.Name)
	}
}

func (c *CrontabComponent) scene() *ecs.Scene {
	if c == nil || c.GetEntity() == nil {
		return nil
	}
	return c.GetEntity().Scene()
}

func safeInvoke(handler ICrontabHandler, scene *ecs.Scene, task *CrontabTask) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("task %s panic: %v", task.Name, r)
			slog.Error("定时任务 panic", "task", task.Name, "error", r)
		}
	}()
	return handler.Handle(scene, task)
}
