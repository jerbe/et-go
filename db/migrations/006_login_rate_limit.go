package migrations

import (
	"context"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const loginRateLimitBucketCollection = "login_rate_limit_buckets"

// EnsureLoginRateLimitBucketIndex 为登录限流 bucket 创建 TTL 索引。
func EnsureLoginRateLimitBucketIndex(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	_, err := client.Collection(loginRateLimitBucketCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().
			SetName("login_rate_limit_bucket_expires_at_ttl").
			SetExpireAfterSeconds(0),
	})
	return err
}
