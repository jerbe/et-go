package migrations

import (
	"context"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const accessTokenRevocationCollection = "access_token_revocations"

// EnsureAccessTokenRevocationIndex 为 Token 撤销记录创建 TTL 索引。
//
// 撤销记录到期后由 MongoDB 自动清理；验证路径仍会检查过期时间，避免
// TTL monitor 延迟期间把已过期记录当作永久状态。
func EnsureAccessTokenRevocationIndex(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	_, err := client.Collection(accessTokenRevocationCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().
			SetName("access_token_revocation_expires_at_ttl").
			SetExpireAfterSeconds(0),
	})
	return err
}
