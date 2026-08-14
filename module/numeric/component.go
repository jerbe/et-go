package numeric

import (
	stdmath "math"

	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
)

// NumericComponent 数值属性组件。
type NumericComponent struct {
	ecs.BaseComponent
	NumericDic     map[int]int64
	watcherManager *WatcherManager
	closed         bool
}

// Component 保留旧名称兼容。
type Component = NumericComponent

// NewComponent 创建数值组件。
func NewComponent() *NumericComponent {
	return &NumericComponent{
		NumericDic:     make(map[int]int64),
		watcherManager: DefaultWatcherManager,
	}
}

// NewNumericComponent 创建数值组件。
func NewNumericComponent() *NumericComponent {
	return NewComponent()
}

// Type 返回组件类型名。
func (c *NumericComponent) Type() string { return "NumericComponent" }

// Awake 初始化内部状态。
func (c *NumericComponent) Awake() {
	if c == nil || c.closed {
		return
	}
	if c.NumericDic == nil {
		c.NumericDic = make(map[int]int64)
	}
	if c.watcherManager == nil {
		c.watcherManager = DefaultWatcherManager
	}
}

// OnDestroy 清理内部状态。
func (c *NumericComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.closed = true
	c.NumericDic = nil
}

// Get 获取原始 int64 数值。
func (c *NumericComponent) Get(numType int) int64 {
	if c == nil || c.NumericDic == nil {
		return 0
	}
	return c.NumericDic[numType]
}

// GetAsFloat 获取浮点值。
func (c *NumericComponent) GetAsFloat(numType int) float64 {
	return float64(c.Get(numType)) / FloatMultiplier
}

// GetAsInt 获取 int32 值。
func (c *NumericComponent) GetAsInt(numType int) int32 {
	return int32(c.Get(numType))
}

// SetFloat 设置浮点值。
func (c *NumericComponent) SetFloat(numType int, value float64) {
	c.Set(numType, int64(stdmath.Round(value*FloatMultiplier)))
}

// Set 设置原始值。
func (c *NumericComponent) Set(numType int, value int64) {
	if c == nil || c.closed {
		return
	}
	c.Awake()

	old := c.NumericDic[numType]
	if old == value {
		return
	}
	c.NumericDic[numType] = value

	if numType >= NumericTypeMax {
		c.recalculate(numType / 10)
		return
	}
	c.publishChange(numType, old, value)
}

// Serialize 序列化数值字典。
func (c *NumericComponent) Serialize() ([]byte, error) {
	if c == nil || c.closed {
		return nil, ErrNumericClosed
	}
	c.Awake()
	return bson.Marshal(c.NumericDic)
}

// Deserialize 反序列化数值字典。
func (c *NumericComponent) Deserialize(data []byte) error {
	if c == nil || c.closed {
		return ErrNumericClosed
	}
	c.Awake()
	if len(data) == 0 {
		c.NumericDic = make(map[int]int64)
		return nil
	}
	if err := bson.Unmarshal(data, &c.NumericDic); err != nil {
		return err
	}
	c.recalculateAll()
	return nil
}

// Transfer 导出迁移数据。
func (c *NumericComponent) Transfer() ([]byte, error) {
	return c.Serialize()
}

// OnTransferIn 导入迁移数据。
func (c *NumericComponent) OnTransferIn(data []byte) error {
	return c.Deserialize(data)
}

// GetAllFinal 返回所有最终属性的拷贝。
func (c *NumericComponent) GetAllFinal() map[int]int64 {
	if c == nil || c.closed {
		return nil
	}
	c.Awake()
	result := make(map[int]int64)
	for numType, value := range c.NumericDic {
		if numType < NumericTypeMax {
			result[numType] = value
		}
	}
	return result
}

func (c *NumericComponent) recalculate(finalType int) {
	base := c.NumericDic[finalType*10+SubBase]
	add := c.NumericDic[finalType*10+SubAdd]
	pct := c.NumericDic[finalType*10+SubPct]
	finalAdd := c.NumericDic[finalType*10+SubFinalAdd]
	finalPct := c.NumericDic[finalType*10+SubFinalPct]

	result := (((base+add)*(100+pct))/100 + finalAdd) * (100 + finalPct) / 100
	old := c.NumericDic[finalType]
	c.NumericDic[finalType] = result
	if old != result {
		c.publishChange(finalType, old, result)
	}
}

func (c *NumericComponent) recalculateAll() {
	if c == nil || c.NumericDic == nil {
		return
	}

	finalTypes := make(map[int]struct{})
	for numType := range c.NumericDic {
		if numType >= NumericTypeMax {
			finalTypes[numType/10] = struct{}{}
		}
	}
	for finalType := range finalTypes {
		c.recalculate(finalType)
	}
}

func (c *NumericComponent) publishChange(numType int, old, new_ int64) {
	if c == nil || numType >= NumericTypeMax || old == new_ {
		return
	}

	unit := any(c.GetEntity())
	if c.watcherManager != nil {
		c.watcherManager.Notify(unit, numType, old, new_)
	}

	entity := c.GetEntity()
	if entity == nil {
		return
	}
	scene := entity.Scene()
	if scene == nil || scene.EventBus() == nil {
		return
	}
	scene.EventBus().Publish(EventNumericChange, &NumericChange{
		Unit:     unit,
		Type:     numType,
		OldValue: old,
		NewValue: new_,
	})
}
