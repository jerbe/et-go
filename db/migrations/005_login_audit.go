package migrations

import (
	"context"

	"github.com/jerbe/et-go/db"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const loginAuditCollection = "login_audit"

// EnsureLoginAuditIndex 为登录审计集合创建时间索引。
//
// 保留周期由部署策略决定，因此这里只建立查询索引，不擅自设置 TTL 删除
// 审计证据。
func EnsureLoginAuditIndex(ctx context.Context, client *db.Client) error {
	if client == nil {
		return ErrClientRequired
	}
	_, err := client.Collection(loginAuditCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("login_audit_created_at"),
	})
	return err
}
