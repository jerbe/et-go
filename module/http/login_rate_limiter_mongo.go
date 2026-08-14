package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultLoginRateLimitCollection = "login_rate_limit_buckets"

// MongoLoginRateLimitStore 定义基于 MongoDB 原子 bucket 的跨 Process 限流。
type MongoLoginRateLimitStore struct {
	component  *db.DBComponent
	collection string
}

type loginRateLimitBucket struct {
	ID        string    `bson:"_id"`
	Count     int       `bson:"count"`
	ExpiresAt time.Time `bson:"expires_at"`
}

// NewMongoLoginRateLimitStore 创建显式 MongoDB 限流存储。
func NewMongoLoginRateLimitStore(component *db.DBComponent) (*MongoLoginRateLimitStore, error) {
	if component == nil || component.Client() == nil {
		return nil, ErrLoginRateLimiterInvalid
	}
	return &MongoLoginRateLimitStore{
		component:  component,
		collection: defaultLoginRateLimitCollection,
	}, nil
}

func (s *MongoLoginRateLimitStore) collectionHandle() (*mongo.Collection, error) {
	if s == nil || s.component == nil || s.component.Client() == nil ||
		strings.TrimSpace(s.collection) == "" {
		return nil, ErrLoginRateLimiterInvalid
	}
	collection := s.component.Client().Collection(s.collection)
	if collection == nil {
		return nil, ErrLoginRateLimiterInvalid
	}
	return collection, nil
}

// Allow 在当前固定窗口 bucket 中原子增加计数。
func (s *MongoLoginRateLimitStore) Allow(
	ctx context.Context,
	key string,
	limit int,
	window time.Duration,
	now time.Time,
) (bool, error) {
	if ctx == nil {
		return false, ErrLoginRateLimiterContextRequired
	}
	if strings.TrimSpace(key) == "" || limit <= 0 || window <= 0 || now.IsZero() {
		return false, ErrLoginRateLimiterInvalid
	}
	collection, err := s.collectionHandle()
	if err != nil {
		return false, err
	}
	bucketStart := now.UnixNano() / window.Nanoseconds() * window.Nanoseconds()
	keyHash := sha256.Sum256([]byte(key))
	bucketID := hex.EncodeToString(keyHash[:]) + "|" + strconv.FormatInt(bucketStart, 10)
	document := loginRateLimitBucket{
		ID:        bucketID,
		ExpiresAt: time.Unix(0, bucketStart).Add(window),
	}
	var updated loginRateLimitBucket
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": document.ID},
		bson.M{
			"$inc": bson.M{"count": 1},
			"$setOnInsert": bson.M{
				"expires_at": document.ExpiresAt,
			},
		},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&updated)
	if err != nil {
		return false, fmt.Errorf("http: update login rate limit bucket: %w", err)
	}
	return updated.Count <= limit, nil
}

// MongoLoginRateLimiterComponent 将 MongoDB bucket 限流挂载到 HTTP Scene。
type MongoLoginRateLimiterComponent struct {
	ecs.BaseComponent

	Store  *MongoLoginRateLimitStore
	Limit  int
	Window time.Duration
}

func (c *MongoLoginRateLimiterComponent) Type() string {
	return "LoginRateLimiterComponent"
}

// NewMongoLoginRateLimiterComponent 创建 MongoDB 限流组件。
func NewMongoLoginRateLimiterComponent(
	store *MongoLoginRateLimitStore,
	limit int,
	window time.Duration,
) (*MongoLoginRateLimiterComponent, error) {
	if store == nil || limit <= 0 {
		return nil, ErrLoginRateLimiterInvalid
	}
	if window <= 0 {
		window = defaultLoginRateLimitWindow
	}
	return &MongoLoginRateLimiterComponent{
		Store:  store,
		Limit:  limit,
		Window: window,
	}, nil
}

func (c *MongoLoginRateLimiterComponent) AllowContext(ctx context.Context, key string) (bool, error) {
	if c == nil || c.Store == nil || c.Limit <= 0 || c.Window <= 0 {
		return false, ErrLoginRateLimiterInvalid
	}
	return c.Store.Allow(ctx, key, c.Limit, c.Window, time.Now())
}

// DBManagerLoginRateLimiterComponent 通过 DBManager 懒加载共享限流数据库。
type DBManagerLoginRateLimiterComponent struct {
	ecs.BaseComponent

	Manager *db.DBManagerComponent
	Zone    int
	Limit   int
	Window  time.Duration
}

func (c *DBManagerLoginRateLimiterComponent) Type() string {
	return "LoginRateLimiterComponent"
}

// NewDBManagerLoginRateLimiterComponent 创建 DBManager 限流组件。
func NewDBManagerLoginRateLimiterComponent(
	manager *db.DBManagerComponent,
	zone int,
	limit int,
	window time.Duration,
) (*DBManagerLoginRateLimiterComponent, error) {
	if manager == nil || zone <= 0 || limit <= 0 {
		return nil, ErrLoginRateLimiterInvalid
	}
	if window <= 0 {
		window = defaultLoginRateLimitWindow
	}
	return &DBManagerLoginRateLimiterComponent{
		Manager: manager,
		Zone:    zone,
		Limit:   limit,
		Window:  window,
	}, nil
}

func (c *DBManagerLoginRateLimiterComponent) AllowContext(ctx context.Context, key string) (bool, error) {
	if c == nil || c.Manager == nil || c.Zone <= 0 || c.Limit <= 0 || c.Window <= 0 {
		return false, ErrLoginRateLimiterInvalid
	}
	component, err := c.Manager.GetZoneDB(c.Zone)
	if err != nil {
		return false, fmt.Errorf("http: resolve login rate limit database: %w", err)
	}
	store, err := NewMongoLoginRateLimitStore(component)
	if err != nil {
		return false, err
	}
	storeComponent, err := NewMongoLoginRateLimiterComponent(store, c.Limit, c.Window)
	if err != nil {
		return false, err
	}
	return storeComponent.AllowContext(ctx, key)
}

var _ interface {
	AllowContext(context.Context, string) (bool, error)
} = (*LoginRateLimiterComponent)(nil)

var _ interface {
	AllowContext(context.Context, string) (bool, error)
} = (*MongoLoginRateLimiterComponent)(nil)

var _ interface {
	AllowContext(context.Context, string) (bool, error)
} = (*DBManagerLoginRateLimiterComponent)(nil)
