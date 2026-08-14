package move

import (
	"time"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/event"
	etmath "github.com/jerbe/et-go/engine/math"
)

// UnitAccessor 表示 MoveComponent 依赖的最小单位接口。
type UnitAccessor interface {
	Position() etmath.Vector3
	SetPosition(pos etmath.Vector3)
	Rotation() etmath.Quaternion
	SetRotation(rot etmath.Quaternion)
}

// MoveComponent 管理单位的路径移动。
type MoveComponent struct {
	ecs.BaseComponent
	Speed            float32
	StartTime        int64
	BeginTime        int64
	StartPos         etmath.Vector3
	NeedTime         int64
	Targets          []etmath.Vector3
	N                int
	TurnTime         int
	IsTurnHorizontal bool
	From             etmath.Quaternion
	To               etmath.Quaternion
	moveDone         chan error
	isMoving         bool
	unit             UnitAccessor
	eventBus         *event.Bus
	nowFunc          func() time.Time
	closed           bool
}

// NewMoveComponent 创建 MoveComponent。
func NewMoveComponent() *MoveComponent {
	return &MoveComponent{
		From:    etmath.QuaternionIdentity,
		To:      etmath.QuaternionIdentity,
		nowFunc: time.Now,
	}
}

// Type 返回组件类型名。
func (c *MoveComponent) Type() string { return "MoveComponent" }

// Awake 初始化默认状态。
func (c *MoveComponent) Awake() {
	if c == nil || c.closed {
		return
	}
	if c.nowFunc == nil {
		c.nowFunc = time.Now
	}
	if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
		c.eventBus = entity.Scene().EventBus()
		if registrar, ok := entity.Scene().Fiber().(interface{ RegisterUpdate(ecs.UpdateSystem) }); ok {
			registrar.RegisterUpdate(c)
		}
	}
}

// OnDestroy 清理注册关系。
func (c *MoveComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.Stop(false)
	c.closed = true
	if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
		if registrar, ok := entity.Scene().Fiber().(interface{ UnregisterUpdate(ecs.UpdateSystem) }); ok {
			registrar.UnregisterUpdate(c)
		}
	}
	c.eventBus = nil
	c.unit = nil
}

// Bind 绑定单位访问器。
func (c *MoveComponent) Bind(unit UnitAccessor) {
	if c == nil || c.closed {
		return
	}
	c.unit = unit
	if entity := c.GetEntity(); entity != nil && entity.Scene() != nil {
		c.eventBus = entity.Scene().EventBus()
	}
}

// SetNowFunc 设置时间函数，便于测试驱动。
func (c *MoveComponent) SetNowFunc(now func() time.Time) {
	if c == nil || c.closed {
		return
	}
	if now == nil {
		c.nowFunc = time.Now
		return
	}
	c.nowFunc = now
}

// MoveToAsync 开始沿路径移动，并等待完成。
func (c *MoveComponent) MoveToAsync(targets []etmath.Vector3, speed float32, turnTime int) error {
	if c == nil {
		return ErrInvalidPath
	}
	done, err := c.StartMove(targets, speed, turnTime)
	if err != nil {
		return err
	}
	return <-done
}

// StartMove 同步初始化移动状态，并返回完成通知 channel。
func (c *MoveComponent) StartMove(targets []etmath.Vector3, speed float32, turnTime int) (<-chan error, error) {
	if c == nil {
		return nil, ErrInvalidPath
	}
	if c.closed {
		return nil, ErrMoveCanceled
	}
	if err := c.beginMove(targets, speed, turnTime); err != nil {
		return nil, err
	}
	return c.moveDone, nil
}

func (c *MoveComponent) beginMove(targets []etmath.Vector3, speed float32, turnTime int) error {
	if c == nil {
		return ErrInvalidPath
	}
	if c.closed {
		return ErrMoveCanceled
	}
	if len(targets) < 2 {
		return ErrInvalidPath
	}
	if speed <= 0 {
		return ErrInvalidSpeed
	}

	c.Stop(false)

	c.Targets = append([]etmath.Vector3(nil), targets...)
	c.Speed = speed
	c.TurnTime = turnTime
	c.N = 1
	c.StartPos = targets[0]
	c.StartTime = c.currentTimeMillis()
	c.BeginTime = c.StartTime
	c.isMoving = true
	c.moveDone = make(chan error, 1)

	if c.unit != nil {
		c.unit.SetPosition(c.StartPos)
		c.From = c.unit.Rotation()
	} else {
		c.From = etmath.QuaternionIdentity
	}
	c.prepareSegment(c.StartPos, c.Targets[c.N], c.From)
	c.publishMoveStart()
	return nil
}

// Update 执行当前位置和旋转插值。
func (c *MoveComponent) Update() {
	if c == nil || c.closed {
		return
	}
	if !c.isMoving || c.unit == nil {
		return
	}

	now := c.currentTimeMillis()
	elapsed := now - c.BeginTime
	if c.NeedTime > 0 && elapsed < c.NeedTime {
		t := float32(elapsed) / float32(c.NeedTime)
		c.unit.SetPosition(etmath.Lerp(c.StartPos, c.Targets[c.N], t))
		if c.TurnTime > 0 {
			rt := t
			if c.NeedTime > 0 {
				rt = float32(elapsed) / float32(c.TurnTime)
			}
			if rt > 1 {
				rt = 1
			}
			c.unit.SetRotation(etmath.Slerp(c.From, c.To, rt))
		} else {
			c.unit.SetRotation(c.To)
		}
		return
	}

	c.unit.SetPosition(c.Targets[c.N])
	c.unit.SetRotation(c.To)
	c.N++
	if c.N >= len(c.Targets) {
		c.moveFinish(nil, true)
		return
	}

	currentPos := c.unit.Position()
	currentRot := c.unit.Rotation()
	c.prepareSegment(currentPos, c.Targets[c.N], currentRot)
}

// ChangeSpeed 在移动中调整速度。
func (c *MoveComponent) ChangeSpeed(speed float32) bool {
	if c == nil || c.closed {
		return false
	}
	if !c.isMoving || c.unit == nil || speed <= 0 || c.N >= len(c.Targets) {
		return false
	}

	now := c.currentTimeMillis()
	elapsed := now - c.BeginTime
	currentPos := c.currentPosition(elapsed)
	c.unit.SetPosition(currentPos)
	c.StartPos = currentPos
	c.BeginTime = now
	c.Speed = speed
	c.NeedTime = segmentNeedTime(currentPos, c.Targets[c.N], speed)
	return true
}

// Stop 停止当前移动。
func (c *MoveComponent) Stop(ret bool) {
	if c == nil {
		return
	}
	if !c.isMoving {
		return
	}
	c.moveFinish(stopError(ret), ret)
}

// IsArrived 返回是否已结束移动。
func (c *MoveComponent) IsArrived() bool {
	return c == nil || len(c.Targets) == 0 || c.N >= len(c.Targets)
}

// FlashTo 立即传送到目标位置。
func (c *MoveComponent) FlashTo(target etmath.Vector3) bool {
	if c == nil || c.closed {
		return false
	}
	c.Stop(false)
	if c.unit == nil {
		return false
	}
	c.unit.SetPosition(target)
	return true
}

// RemainingTargets 返回剩余路径点。
func (c *MoveComponent) RemainingTargets() []etmath.Vector3 {
	if c == nil || c.N >= len(c.Targets) {
		return nil
	}
	return append([]etmath.Vector3(nil), c.Targets[c.N:]...)
}

func (c *MoveComponent) publishMoveStart() {
	if c == nil {
		return
	}
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(EventMoveStart, &MoveStart{Unit: c.unit})
}

func (c *MoveComponent) publishMoveStop() {
	if c == nil {
		return
	}
	if c.eventBus == nil {
		return
	}
	c.eventBus.Publish(EventMoveStop, &MoveStop{Unit: c.unit})
}

func (c *MoveComponent) moveFinish(err error, publishStop bool) {
	if c == nil {
		return
	}
	if !c.isMoving {
		return
	}
	c.isMoving = false
	c.Targets = nil
	c.N = 0
	if publishStop {
		c.publishMoveStop()
	}
	if c.moveDone != nil {
		c.moveDone <- err
		close(c.moveDone)
		c.moveDone = nil
	}
}

func (c *MoveComponent) prepareSegment(start, target etmath.Vector3, currentRot etmath.Quaternion) {
	if c == nil {
		return
	}
	c.StartPos = start
	c.BeginTime = c.currentTimeMillis()
	c.NeedTime = segmentNeedTime(start, target, c.Speed)
	c.From = currentRot
	dir := target.Sub(start)
	if c.IsTurnHorizontal {
		dir.Y = 0
	}
	c.To = etmath.LookRotation(dir)
}

func (c *MoveComponent) currentTimeMillis() int64 {
	if c == nil {
		return 0
	}
	if c.nowFunc == nil {
		c.nowFunc = time.Now
	}
	return c.nowFunc().UnixMilli()
}

func (c *MoveComponent) currentPosition(elapsed int64) etmath.Vector3 {
	if c.N >= len(c.Targets) || c.NeedTime <= 0 {
		if c.N > 0 && c.N-1 < len(c.Targets) {
			return c.Targets[c.N-1]
		}
		return c.StartPos
	}
	t := float32(elapsed) / float32(c.NeedTime)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return etmath.Lerp(c.StartPos, c.Targets[c.N], t)
}

func segmentNeedTime(start, target etmath.Vector3, speed float32) int64 {
	if speed <= 0 {
		return 0
	}
	distance := start.Distance(target)
	if distance == 0 {
		return 0
	}
	return int64(float64(distance/speed) * 1000)
}

func stopError(ret bool) error {
	if ret {
		return nil
	}
	return ErrMoveCanceled
}
