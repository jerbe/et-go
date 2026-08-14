package migrations

import (
	"context"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
)

// EnsurePasswordAlgorithms 为历史 account 文档补齐密码算法标记。
//
// 未带 algorithm 的旧文档按 MD5 处理；新 Argon2id 文档按哈希前缀标记为
// Argon2id。该 migration 不计算密码、不改变 password_hash，首次成功登录
// 再执行真正的 Argon2id 重哈希。
func EnsurePasswordAlgorithms(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	collection := client.Collection((&db.CAccount{}).CollectionName())
	if _, err := collection.UpdateMany(
		ctx,
		bson.M{
			"password_algorithm": bson.M{"$exists": false},
			"password_hash":      bson.M{"$regex": `^\$argon2id\$`},
		},
		bson.M{"$set": bson.M{"password_algorithm": "argon2id"}},
	); err != nil {
		return err
	}
	_, err := collection.UpdateMany(
		ctx,
		bson.M{
			"password_algorithm": bson.M{"$exists": false},
			"password_hash":      bson.M{"$not": bson.M{"$regex": `^\$argon2id\$`}},
		},
		bson.M{"$set": bson.M{"password_algorithm": "md5"}},
	)
	return err
}
