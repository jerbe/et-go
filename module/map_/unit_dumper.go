package map_

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/timer"
	"github.com/jerbe/et-go/module/unit"
	"go.mongodb.org/mongo-driver/bson"
)

const defaultDumpInterval = 5 * time.Minute
const dumpSaveTimeout = 5 * time.Second
const dumpDrainTimeout = 6 * time.Second
const defaultDumpMaxRetries = 2
const defaultDumpRetryBackoff = 100 * time.Millisecond
const defaultDumpMaxDeadLetters = 1024
const unitDumpQueueCollection = "unit_dump_queue"

type unitDumpStore interface {
	Insert(ctx context.Context, entity any, collection string) error
	Query(ctx context.Context, filter bson.M, collection string, results any) error
	Save(ctx context.Context, id int64, entity any, collection string) error
	Remove(ctx context.Context, id int64, collection string) (int64, error)
}

// UnitDumperComponent 定时保存可持久化组件。
//
// 组件先把不可变快照写入 durable queue，再写业务集合，成功后删除 queue
// 记录。进程重启或响应丢失时，下一次 dump 会重放未删除的 queue 记录。
// 多组件一致性事务和跨组件崩溃恢复仍需要更高层数据库协议，不能由单个
// queue 记录伪造。
//
// TODO(persistence): 定义 Unit 聚合快照的版本、跨集合提交边界和恢复扫描器；
// 当前各组件可能属于不同 MongoDB collection，自动拼接或顺序写入都会留下
// 可见的部分状态，因此继续保持单组件 durable queue 语义。
type UnitDumperComponent struct {
	ecs.BaseComponent
	Interval       time.Duration
	MaxRetries     int
	RetryBackoff   time.Duration
	MaxDeadLetters int
	timerID        int64
	mu             sync.Mutex
	stopping       bool
	dumping        bool
	saveWG         sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	deadLetters    []UnitDumperDeadLetter
}

type persistedEntitySnapshot struct {
	id         int64
	collection string
	data       []byte
}

type unitDumpTask struct {
	Id         int64     `bson:"_id"`
	TargetID   int64     `bson:"target_id"`
	Collection string    `bson:"collection"`
	Data       []byte    `bson:"data"`
	CreatedAt  time.Time `bson:"created_at"`
	Attempts   int       `bson:"attempts"`
	LastError  string    `bson:"last_error,omitempty"`
}

func (t *unitDumpTask) GetID() int64 {
	if t == nil {
		return 0
	}
	return t.Id
}

func (e persistedEntitySnapshot) GetID() int64 {
	return e.id
}

func (e persistedEntitySnapshot) CollectionName() string {
	return e.collection
}

func (e persistedEntitySnapshot) MarshalBSON() ([]byte, error) {
	return append([]byte(nil), e.data...), nil
}

// UnitDumperDeadLetter 表示重试耗尽后保留的不可变失败快照。
type UnitDumperDeadLetter struct {
	ID         int64
	Collection string
	Data       []byte
	Attempts   int
	Error      string
	CreatedAt  time.Time
}

// Type 返回组件名称。
func (c *UnitDumperComponent) Type() string { return "UnitDumperComponent" }

// Awake 注册定时存储。
func (c *UnitDumperComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.stopping || c.timerID != 0 {
		c.mu.Unlock()
		return
	}
	if c.Interval <= 0 {
		c.Interval = defaultDumpInterval
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultDumpMaxRetries
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = defaultDumpRetryBackoff
	}
	if c.MaxDeadLetters <= 0 {
		c.MaxDeadLetters = defaultDumpMaxDeadLetters
	}
	if c.ctx == nil {
		parent := context.Background()
		if scene := entityScene(c.GetEntity()); scene != nil {
			if provider, ok := scene.Fiber().(interface{ Context() context.Context }); ok {
				if provided := provider.Context(); provided != nil {
					parent = provided
				}
			}
		}
		c.ctx, c.cancel = context.WithCancel(parent)
	}
	interval := c.Interval
	c.mu.Unlock()
	entity := c.GetEntity()
	if entity == nil {
		slog.Error("UnitDumper entity missing")
		return
	}
	component, ok := entity.GetComponent("TimerComponent")
	if !ok {
		return
	}
	timerComponent, ok := component.(*timer.TimerComponent)
	if !ok {
		return
	}
	timerID := timerComponent.AddRepeatingTimer(interval, c.dump)
	c.mu.Lock()
	stopping := c.stopping
	if !stopping {
		c.timerID = timerID
	}
	c.mu.Unlock()
	if stopping {
		timerComponent.RemoveTimer(timerID)
		return
	}
	if timerID == 0 {
		slog.Error("UnitDumper timer registration failed", "interval", interval)
	}
}

// OnDestroy 取消定时器。
func (c *UnitDumperComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.stopping = true
	timerID := c.timerID
	c.timerID = 0
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	entity := c.GetEntity()
	if entity != nil {
		if component, ok := entity.GetComponent("TimerComponent"); ok {
			if timerComponent, ok := component.(*timer.TimerComponent); ok {
				timerComponent.RemoveTimer(timerID)
			}
		}
	}

	done := make(chan struct{})
	go func() {
		c.saveWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(dumpDrainTimeout):
		slog.Error("UnitDumper save drain timeout")
	}
}

func (c *UnitDumperComponent) dump() {
	if c == nil {
		return
	}
	var batchWG sync.WaitGroup
	c.mu.Lock()
	stopping := c.stopping
	if !stopping && !c.dumping {
		c.dumping = true
	} else {
		stopping = true
	}
	c.mu.Unlock()
	if stopping {
		return
	}
	defer func() {
		go func() {
			batchWG.Wait()
			c.mu.Lock()
			c.dumping = false
			c.mu.Unlock()
		}()
	}()
	entity := c.GetEntity()
	if entity == nil {
		slog.Error("UnitDumper entity missing")
		return
	}
	scene := entity.Scene()
	if scene == nil {
		slog.Error("UnitDumper scene missing")
		return
	}
	sceneName := scene.Name()

	dbManagerRaw, ok := scene.GetComponent("DBManagerComponent")
	if !ok || dbManagerRaw == nil {
		slog.Error("UnitDumper DBManagerComponent missing", "scene", sceneName)
		return
	}
	dbManager, ok := dbManagerRaw.(*db.DBManagerComponent)
	if !ok {
		slog.Error("UnitDumper DBManagerComponent type invalid", "scene", sceneName)
		return
	}
	zoneDB, err := dbManager.GetZoneDB(scene.Zone())
	if err != nil {
		slog.Error("UnitDumper resolve zone DB failed", "scene", sceneName, "zone", scene.Zone(), "err", err)
		return
	}
	if err := c.recoverPending(c.persistenceContext(), zoneDB); err != nil {
		slog.Error("UnitDumper recover durable queue failed", "scene", sceneName, "zone", scene.Zone(), "err", err)
		return
	}

	unitComponentRaw, ok := scene.GetComponent("UnitComponent")
	if !ok || unitComponentRaw == nil {
		slog.Error("UnitDumper UnitComponent missing", "scene", sceneName)
		return
	}
	unitComponent, ok := unitComponentRaw.(*unit.UnitComponent)
	if !ok {
		slog.Error("UnitDumper UnitComponent type invalid", "scene", sceneName)
		return
	}

	for _, u := range unitComponent.GetAll() {
		for _, component := range u.Components() {
			dbEntity, ok := component.(db.IDBEntityCollection)
			if !ok {
				continue
			}
			entityID := dbEntity.GetID()
			collection := dbEntity.CollectionName()
			if entityID <= 0 || collection == "" {
				slog.Error(
					"UnitDumper persist entity metadata invalid",
					"scene", sceneName,
					"collection", collection,
					"id", entityID,
				)
				continue
			}
			data, err := bson.Marshal(dbEntity)
			if err != nil {
				slog.Error(
					"UnitDumper serialize entity failed",
					"scene", sceneName,
					"collection", collection,
					"id", entityID,
					"err", err,
				)
				continue
			}
			snapshot := persistedEntitySnapshot{
				id:         entityID,
				collection: collection,
				data:       data,
			}
			c.mu.Lock()
			if c.stopping {
				c.mu.Unlock()
				return
			}
			batchWG.Add(1)
			c.saveWG.Add(1)
			c.mu.Unlock()
			go func(entity persistedEntitySnapshot) {
				defer batchWG.Done()
				defer c.saveWG.Done()
				if err := c.saveSnapshot(c.persistenceContext(), zoneDB, entity); err != nil {
					if !errors.Is(err, context.Canceled) {
						slog.Error(
							"UnitDumper persist entity failed",
							"scene", sceneName,
							"collection", entity.CollectionName(),
							"id", entity.GetID(),
							"err", err,
						)
						c.addDeadLetter(entity, err)
					}
				}
			}(snapshot)
		}
	}
}

func (c *UnitDumperComponent) saveSnapshot(parent context.Context, store unitDumpStore, entity persistedEntitySnapshot) error {
	if store == nil {
		return db.ErrCollectionNotFound
	}
	if entity.GetID() <= 0 || entity.CollectionName() == "" || len(entity.data) == 0 {
		return fmt.Errorf("map_: durable dump snapshot invalid")
	}
	if parent == nil {
		parent = context.Background()
	}
	c.mu.Lock()
	attempts := c.MaxRetries + 1
	backoff := c.RetryBackoff
	c.mu.Unlock()
	taskID, err := newUnitDumpTaskID()
	if err != nil {
		return fmt.Errorf("map_: create durable dump task: %w", err)
	}
	task := &unitDumpTask{
		Id:         taskID,
		TargetID:   entity.GetID(),
		Collection: entity.CollectionName(),
		Data:       append([]byte(nil), entity.data...),
		CreatedAt:  time.Now(),
	}
	if err := saveWithRetry(parent, func(ctx context.Context) error {
		return store.Insert(ctx, task, unitDumpQueueCollection)
	}, attempts, backoff); err != nil {
		return fmt.Errorf("map_: enqueue durable dump task: %w", err)
	}
	return c.flushDumpTask(parent, store, task, attempts, backoff)
}

func (c *UnitDumperComponent) recoverPending(parent context.Context, store unitDumpStore) error {
	if store == nil {
		return db.ErrCollectionNotFound
	}
	if parent == nil {
		parent = context.Background()
	}
	var tasks []unitDumpTask
	ctx, cancel := context.WithTimeout(parent, dumpSaveTimeout)
	err := store.Query(ctx, bson.M{}, unitDumpQueueCollection, &tasks)
	cancel()
	if err != nil {
		return fmt.Errorf("map_: query durable dump queue: %w", err)
	}
	c.mu.Lock()
	attempts := c.MaxRetries + 1
	backoff := c.RetryBackoff
	c.mu.Unlock()
	for index := range tasks {
		task := tasks[index]
		if err := c.flushDumpTask(parent, store, &task, attempts, backoff); err != nil {
			snapshot := persistedEntitySnapshot{
				id:         task.TargetID,
				collection: task.Collection,
				data:       append([]byte(nil), task.Data...),
			}
			if !errors.Is(err, context.Canceled) {
				c.addDeadLetter(snapshot, err)
			}
		}
	}
	return nil
}

func (c *UnitDumperComponent) flushDumpTask(
	parent context.Context,
	store unitDumpStore,
	task *unitDumpTask,
	attempts int,
	backoff time.Duration,
) error {
	if store == nil || task == nil || task.Id <= 0 || task.TargetID <= 0 ||
		task.Collection == "" || len(task.Data) == 0 {
		return fmt.Errorf("map_: durable dump task invalid")
	}
	entity := persistedEntitySnapshot{
		id:         task.TargetID,
		collection: task.Collection,
		data:       append([]byte(nil), task.Data...),
	}
	err := saveWithRetry(parent, func(ctx context.Context) error {
		return store.Save(ctx, entity.GetID(), entity, entity.CollectionName())
	}, attempts, backoff)
	if err != nil {
		task.Attempts++
		task.LastError = err.Error()
		metadataErr := saveWithRetry(parent, func(ctx context.Context) error {
			return store.Save(ctx, task.Id, task, unitDumpQueueCollection)
		}, attempts, backoff)
		if metadataErr != nil {
			return errors.Join(
				fmt.Errorf("map_: flush durable dump task %d: %w", task.Id, err),
				fmt.Errorf("map_: persist durable dump error metadata: %w", metadataErr),
			)
		}
		return fmt.Errorf("map_: flush durable dump task %d: %w", task.Id, err)
	}
	removeErr := saveWithRetry(parent, func(ctx context.Context) error {
		_, err := store.Remove(ctx, task.Id, unitDumpQueueCollection)
		return err
	}, attempts, backoff)
	if removeErr != nil {
		task.Attempts++
		task.LastError = removeErr.Error()
		metadataErr := saveWithRetry(parent, func(ctx context.Context) error {
			return store.Save(ctx, task.Id, task, unitDumpQueueCollection)
		}, attempts, backoff)
		if metadataErr != nil {
			return errors.Join(
				fmt.Errorf("map_: remove durable dump task %d: %w", task.Id, removeErr),
				fmt.Errorf("map_: persist durable dump removal error metadata: %w", metadataErr),
			)
		}
		return fmt.Errorf("map_: remove durable dump task %d: %w", task.Id, removeErr)
	}
	return nil
}

func newUnitDumpTaskID() (int64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	id := int64(binary.BigEndian.Uint64(raw[:]) & uint64(^uint64(0)>>1))
	if id <= 0 {
		return 0, errors.New("map_: generated durable dump task id is zero")
	}
	return id, nil
}

func (c *UnitDumperComponent) addDeadLetter(entity persistedEntitySnapshot, err error) {
	if c == nil || err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.MaxDeadLetters <= 0 {
		c.MaxDeadLetters = defaultDumpMaxDeadLetters
	}
	c.deadLetters = append(c.deadLetters, UnitDumperDeadLetter{
		ID:         entity.GetID(),
		Collection: entity.CollectionName(),
		Data:       append([]byte(nil), entity.data...),
		Attempts:   c.MaxRetries + 1,
		Error:      err.Error(),
		CreatedAt:  time.Now(),
	})
	if len(c.deadLetters) > c.MaxDeadLetters {
		c.deadLetters = append([]UnitDumperDeadLetter(nil), c.deadLetters[len(c.deadLetters)-c.MaxDeadLetters:]...)
	}
}

// DeadLetters 返回当前进程内的失败快照副本。
func (c *UnitDumperComponent) DeadLetters() []UnitDumperDeadLetter {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]UnitDumperDeadLetter, len(c.deadLetters))
	for index, item := range c.deadLetters {
		result[index] = item
		result[index].Data = append([]byte(nil), item.Data...)
	}
	return result
}

func (c *UnitDumperComponent) persistenceContext() context.Context {
	if c == nil {
		return context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func saveWithRetry(parent context.Context, save func(context.Context) error, attempts int, backoff time.Duration) error {
	if save == nil {
		return errors.New("map_: persistence save function missing")
	}
	if parent == nil {
		parent = context.Background()
	}
	if attempts <= 0 {
		attempts = 1
	}
	if backoff < 0 {
		backoff = 0
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := parent.Err(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(parent, dumpSaveTimeout)
		err := save(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == attempts-1 {
			break
		}
		if backoff == 0 {
			continue
		}
		timer := time.NewTimer(backoff)
		select {
		case <-parent.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return parent.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func entityScene(entity *ecs.Entity) *ecs.Scene {
	if entity == nil {
		return nil
	}
	return entity.Scene()
}
