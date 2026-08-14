package migrations

import (
	"context"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EnsurePlayerProfileIndexes 为玩家档案建立账号和 Zone 的唯一索引。
func EnsurePlayerProfileIndexes(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	_, err := client.Collection((&db.CPlayerProfile{}).CollectionName()).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "account_id", Value: 1},
			{Key: "zone_id", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("player_profile_account_zone_unique"),
	})
	return err
}
