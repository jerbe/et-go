package migrations

import (
	"context"
	"errors"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrClientRequired = errors.New("db migrations: client required")

// EnsureAccountIndexes 为 account 集合创建用户名唯一索引。
func EnsureAccountIndexes(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	_, err := client.Collection((&db.CAccount{}).CollectionName()).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("account_username_unique"),
	})
	return err
}

// All 返回当前版本的全部数据库变更。
func All() []db.Migration {
	return []db.Migration{
		{
			Version: 1,
			Name:    "account_username_unique_index",
			Up:      EnsureAccountIndexes,
		},
		{
			Version: 2,
			Name:    "player_profile_account_zone_unique_index",
			Up:      EnsurePlayerProfileIndexes,
		},
		{
			Version: 3,
			Name:    "account_password_algorithm_marker",
			Up:      EnsurePasswordAlgorithms,
		},
		{
			Version: 4,
			Name:    "access_token_revocation_expires_at_ttl",
			Up:      EnsureAccessTokenRevocationIndex,
		},
		{
			Version: 5,
			Name:    "login_audit_created_at_index",
			Up:      EnsureLoginAuditIndex,
		},
		{
			Version: 6,
			Name:    "login_rate_limit_bucket_expires_at_ttl",
			Up:      EnsureLoginRateLimitBucketIndex,
		},
	}
}
