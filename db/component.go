package db

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// DBTaskCount 表示数据库写入分桶数量。
	DBTaskCount int64 = 32
)

// DBComponent 封装单个 MongoDB 数据库的 CRUD 操作。
type DBComponent struct {
	ecs.BaseComponent
	client *Client
	lock   *coroutinelock.Lock
	lockMu sync.Mutex
}

// NewDBComponent 创建数据库组件。
func NewDBComponent(client *Client, lock *coroutinelock.Lock) *DBComponent {
	return &DBComponent{
		client: client,
		lock:   lock,
	}
}

// Type 返回组件类型名称。
func (db *DBComponent) Type() string { return "DBComponent" }

// SetClient 设置底层 Client。
func (db *DBComponent) SetClient(client *Client) {
	if db == nil {
		return
	}
	db.client = client
}

// SetLock 设置协程锁。
func (db *DBComponent) SetLock(lock *coroutinelock.Lock) {
	if db == nil {
		return
	}
	db.lockMu.Lock()
	db.lock = lock
	db.lockMu.Unlock()
}

// Client 返回底层 Client。
func (db *DBComponent) Client() *Client {
	if db == nil {
		return nil
	}
	return db.client
}

// FindOne 按 `_id` 查询单条文档。
func (db *DBComponent) FindOne(ctx context.Context, id int64, collection string, result any) error {
	if ctx == nil {
		return ErrContextRequired
	}
	col, err := db.getCollection(collection)
	if err != nil {
		return err
	}
	if err := col.FindOne(ctx, bson.M{"_id": id}).Decode(result); err != nil {
		if err == mongo.ErrNoDocuments {
			return ErrDocumentNotFound
		}
		return err
	}
	return nil
}

// Query 按条件查询多条文档。
func (db *DBComponent) Query(ctx context.Context, filter bson.M, collection string, results any) error {
	if ctx == nil {
		return ErrContextRequired
	}
	col, err := db.getCollection(collection)
	if err != nil {
		return err
	}
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	return cursor.All(ctx, results)
}

// Insert 插入单条文档。
func (db *DBComponent) Insert(ctx context.Context, entity any, collection string) error {
	if ctx == nil {
		return ErrContextRequired
	}
	id, err := extractID(entity)
	if err != nil {
		return err
	}
	release, err := db.acquireWriteLock(ctx, id)
	if err != nil {
		return err
	}
	defer release()

	col, err := db.getCollection(collection)
	if err != nil {
		return err
	}
	_, err = col.InsertOne(ctx, entity)
	return err
}

// Save 执行 Upsert 操作。
func (db *DBComponent) Save(ctx context.Context, id int64, entity any, collection string) error {
	if ctx == nil {
		return ErrContextRequired
	}
	entityID, err := extractID(entity)
	if err != nil {
		return err
	}
	if entityID != id {
		return fmt.Errorf("%w: save id=%d entity id=%d", ErrInvalidEntity, id, entityID)
	}
	fields, err := saveUpdateFields(entity)
	if err != nil {
		return err
	}
	release, err := db.acquireWriteLock(ctx, id)
	if err != nil {
		return err
	}
	defer release()

	col, err := db.getCollection(collection)
	if err != nil {
		return err
	}
	_, err = col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": fields},
		options.Update().SetUpsert(true),
	)
	return err
}

// Remove 按 `_id` 删除文档。
func (db *DBComponent) Remove(ctx context.Context, id int64, collection string) (int64, error) {
	if ctx == nil {
		return 0, ErrContextRequired
	}
	release, err := db.acquireWriteLock(ctx, id)
	if err != nil {
		return 0, err
	}
	defer release()

	col, err := db.getCollection(collection)
	if err != nil {
		return 0, err
	}
	result, err := col.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return 0, err
	}
	return result.DeletedCount, nil
}

// Increment 原子自增 increment 集合中的指定 key。
func (db *DBComponent) Increment(ctx context.Context, key string, inc int64, defaultVal int64) (int64, error) {
	if ctx == nil {
		return 0, ErrContextRequired
	}
	col, err := db.getCollection("increment")
	if err != nil {
		return 0, err
	}

	if _, err := col.UpdateOne(
		ctx,
		bson.M{"_id": key},
		bson.M{"$setOnInsert": bson.M{"value": defaultVal}},
		options.Update().SetUpsert(true),
	); err != nil {
		return 0, err
	}

	opts := options.FindOneAndUpdate().
		SetReturnDocument(options.After)

	var result CIncrement
	if err := col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": key},
		bson.M{"$inc": bson.M{"value": inc}},
		opts,
	).Decode(&result); err != nil {
		return 0, err
	}
	return result.Value, nil
}

func (db *DBComponent) getCollection(name string) (*mongo.Collection, error) {
	if db == nil || db.client == nil || db.client.database == nil {
		return nil, ErrCollectionNotFound
	}
	if name == "" {
		return nil, ErrCollectionNotFound
	}
	return db.client.Collection(name), nil
}

func (db *DBComponent) acquireWriteLock(ctx context.Context, id int64) (func(), error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if db == nil {
		return nil, ErrCollectionNotFound
	}
	db.lockMu.Lock()
	if db.lock == nil {
		db.lock = coroutinelock.New()
	}
	lock := db.lock
	db.lockMu.Unlock()
	return lock.Acquire(ctx, coroutinelock.LockTypeDB, dbLockID(id))
}

func dbLockID(id int64) int64 {
	unsigned := uint64(id)
	if id < 0 {
		unsigned = uint64(-(id + 1)) + 1
	}
	return int64(unsigned % uint64(DBTaskCount))
}

func saveUpdateFields(entity any) (bson.M, error) {
	data, err := bson.Marshal(entity)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal save entity: %v", ErrInvalidEntity, err)
	}
	fields := make(bson.M)
	if err := bson.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("%w: decode save entity: %v", ErrInvalidEntity, err)
	}
	delete(fields, "_id")
	return fields, nil
}

func extractID(entity any) (int64, error) {
	if entity == nil {
		return 0, ErrInvalidEntity
	}
	if provider, ok := entity.(interface{ GetID() int64 }); ok {
		if id := provider.GetID(); id != 0 {
			return id, nil
		}
	}

	value := reflect.ValueOf(entity)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, ErrInvalidEntity
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, ErrInvalidEntity
	}

	field := value.FieldByName("Id")
	if !field.IsValid() || !field.CanInterface() {
		return 0, ErrInvalidEntity
	}
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		id := field.Int()
		if id == 0 {
			return 0, fmt.Errorf("%w: zero id", ErrInvalidEntity)
		}
		return id, nil
	default:
		return 0, ErrInvalidEntity
	}
}
