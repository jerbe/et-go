package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Client 封装 MongoDB 客户端
type Client struct {
	client   *mongo.Client
	database *mongo.Database
}

// New 创建并连接 MongoDB 客户端
func New(ctx context.Context, uri string, dbName string) (*Client, error) {
	if ctx == nil {
		return nil, ErrContextRequired
	}
	if strings.TrimSpace(uri) == "" {
		return nil, ErrDatabaseAddressRequired
	}
	if strings.TrimSpace(dbName) == "" {
		return nil, ErrDatabaseNameRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	opts := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(32)

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctx, nil); err != nil {
		if closeErr := client.Disconnect(context.Background()); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("disconnect after ping failure: %w", closeErr))
		}
		return nil, err
	}

	return &Client{
		client:   client,
		database: client.Database(dbName),
	}, nil
}

// Collection 获取集合
func (c *Client) Collection(name string) *mongo.Collection {
	if c == nil || c.database == nil || name == "" {
		return nil
	}
	return c.database.Collection(name)
}

// Database 获取底层 database 对象
func (c *Client) Database() *mongo.Database {
	if c == nil {
		return nil
	}
	return c.database
}

// Close 关闭连接
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("db: close context required")
	}
	return c.client.Disconnect(ctx)
}
