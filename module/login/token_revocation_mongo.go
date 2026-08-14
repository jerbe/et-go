package login

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const defaultAccessTokenRevocationCollection = "access_token_revocations"

// MongoAccessTokenRevocationStore 将 Token 撤销状态保存到共享 MongoDB。
//
// 该组件必须由启动层显式注入 AccessTokenConfig。它不会在 MongoDB 不可用时
// 回退到 MemoryAccessTokenRevocationStore，也不会把查询错误当作“未撤销”。
type MongoAccessTokenRevocationStore struct {
	component  *db.DBComponent
	collection string
	now        func() time.Time
}

// DBManagerAccessTokenRevocationStore 通过 DBManager 懒加载共享 Zone DB。
//
// 它适合在 cmd/server 创建 Fiber 前安装到 AccessTokenConfig：数据库连接仍由
// 第一次撤销查询/写入触发，连接失败会原样返回，不会回退到进程内存储。
type DBManagerAccessTokenRevocationStore struct {
	manager    *db.DBManagerComponent
	zone       int
	collection string
	now        func() time.Time
}

type accessTokenRevocationDocument struct {
	ID        string    `bson:"_id"`
	AccountID int64     `bson:"account_id"`
	ExpiresAt time.Time `bson:"expires_at"`
	RevokedAt time.Time `bson:"revoked_at"`
}

// NewMongoAccessTokenRevocationStore 创建显式 MongoDB 撤销存储。
func NewMongoAccessTokenRevocationStore(component *db.DBComponent) (*MongoAccessTokenRevocationStore, error) {
	if component == nil || component.Client() == nil {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	return &MongoAccessTokenRevocationStore{
		component:  component,
		collection: defaultAccessTokenRevocationCollection,
		now:        time.Now,
	}, nil
}

// NewDBManagerAccessTokenRevocationStore 创建基于 DBManager 的共享撤销存储。
func NewDBManagerAccessTokenRevocationStore(
	manager *db.DBManagerComponent,
	zone int,
) (*DBManagerAccessTokenRevocationStore, error) {
	if manager == nil || zone <= 0 {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	return &DBManagerAccessTokenRevocationStore{
		manager:    manager,
		zone:       zone,
		collection: defaultAccessTokenRevocationCollection,
		now:        time.Now,
	}, nil
}

// SetCollection 设置撤销集合名；空字符串被拒绝。
func (s *MongoAccessTokenRevocationStore) SetCollection(collection string) error {
	if s == nil || strings.TrimSpace(collection) == "" {
		return ErrTokenRevocationStoreUnavailable
	}
	s.collection = strings.TrimSpace(collection)
	return nil
}

// SetCollection 设置 DBManager 撤销集合名；空字符串被拒绝。
func (s *DBManagerAccessTokenRevocationStore) SetCollection(collection string) error {
	if s == nil || strings.TrimSpace(collection) == "" {
		return ErrTokenRevocationStoreUnavailable
	}
	s.collection = strings.TrimSpace(collection)
	return nil
}

func (s *DBManagerAccessTokenRevocationStore) mongoStore() (*MongoAccessTokenRevocationStore, error) {
	if s == nil || s.manager == nil || s.zone <= 0 {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	component, err := s.manager.GetZoneDB(s.zone)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve zone %d database: %v", ErrTokenRevocationStoreUnavailable, s.zone, err)
	}
	store, err := NewMongoAccessTokenRevocationStore(component)
	if err != nil {
		return nil, err
	}
	if err := store.SetCollection(s.collection); err != nil {
		return nil, err
	}
	store.now = s.now
	return store, nil
}

// IsRevoked 查询 DBManager 对应 Zone 的共享撤销状态。
func (s *DBManagerAccessTokenRevocationStore) IsRevoked(
	ctx context.Context,
	token AccessTokenRevocation,
) (bool, error) {
	store, err := s.mongoStore()
	if err != nil {
		return false, err
	}
	return store.IsRevoked(ctx, token)
}

// Revoke 将撤销状态写入 DBManager 对应 Zone 的共享数据库。
func (s *DBManagerAccessTokenRevocationStore) Revoke(
	ctx context.Context,
	token AccessTokenRevocation,
) error {
	store, err := s.mongoStore()
	if err != nil {
		return err
	}
	return store.Revoke(ctx, token)
}

func (s *MongoAccessTokenRevocationStore) collectionHandle() (*mongo.Collection, error) {
	if s == nil || s.component == nil || s.component.Client() == nil {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	collectionName := strings.TrimSpace(s.collection)
	if collectionName == "" {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	collection := s.component.Client().Collection(collectionName)
	if collection == nil {
		return nil, ErrTokenRevocationStoreUnavailable
	}
	return collection, nil
}

func (s *MongoAccessTokenRevocationStore) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

// IsRevoked 查询 Token 是否已撤销。
func (s *MongoAccessTokenRevocationStore) IsRevoked(
	ctx context.Context,
	token AccessTokenRevocation,
) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	if token.TokenID == "" || token.AccountID <= 0 || token.ExpiresAt.IsZero() {
		return false, ErrTokenRevocationStoreUnavailable
	}
	collection, err := s.collectionHandle()
	if err != nil {
		return false, err
	}
	var document accessTokenRevocationDocument
	if err := collection.FindOne(ctx, bson.M{"_id": token.TokenID}).Decode(&document); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return false, nil
		}
		return false, fmt.Errorf("%w: query token %s: %v", ErrTokenRevocationStoreUnavailable, token.TokenID, err)
	}
	if document.AccountID != token.AccountID {
		return false, fmt.Errorf("%w: token %s account mismatch", ErrTokenRevocationStoreUnavailable, token.TokenID)
	}
	if !document.ExpiresAt.IsZero() && !s.clock().Before(document.ExpiresAt) {
		if _, err := collection.DeleteOne(ctx, bson.M{
			"_id":        token.TokenID,
			"expires_at": document.ExpiresAt,
		}); err != nil {
			return false, fmt.Errorf(
				"%w: remove expired token %s: %v",
				ErrTokenRevocationStoreUnavailable,
				token.TokenID,
				err,
			)
		}
		return false, nil
	}
	return true, nil
}

// Revoke 将 Token 撤销记录以 TokenID 幂等写入 MongoDB。
func (s *MongoAccessTokenRevocationStore) Revoke(
	ctx context.Context,
	token AccessTokenRevocation,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if token.TokenID == "" || token.AccountID <= 0 || token.ExpiresAt.IsZero() {
		return ErrTokenRevocationStoreUnavailable
	}
	collection, err := s.collectionHandle()
	if err != nil {
		return err
	}

	now := s.clock()
	document := accessTokenRevocationDocument{
		ID:        token.TokenID,
		AccountID: token.AccountID,
		ExpiresAt: token.ExpiresAt,
		RevokedAt: now,
	}
	_, err = collection.UpdateOne(
		ctx,
		bson.M{
			"_id": document.ID,
			"$or": []bson.M{
				{"account_id": document.AccountID},
				{"account_id": bson.M{"$exists": false}},
			},
		},
		bson.M{
			"$set": bson.M{
				"account_id": document.AccountID,
				"expires_at": document.ExpiresAt,
				"revoked_at": document.RevokedAt,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf(
				"%w: token %s account mismatch",
				ErrTokenRevocationStoreUnavailable,
				token.TokenID,
			)
		}
		return fmt.Errorf("%w: revoke token %s: %v", ErrTokenRevocationStoreUnavailable, token.TokenID, err)
	}
	return nil
}
