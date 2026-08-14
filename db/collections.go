package db

import "time"

// CAccount 表示 account 集合中的账号文档。
type CAccount struct {
	Id                int64  `bson:"_id"`
	Username          string `bson:"username"`
	PasswordHash      string `bson:"password_hash"`
	PasswordAlgorithm string `bson:"password_algorithm,omitempty"`
}

// CollectionName 返回集合名。
func (c *CAccount) CollectionName() string { return "account" }

// GetID 返回文档 ID。
func (c *CAccount) GetID() int64 { return c.Id }

// CIncrement 表示 increment 集合中的原子自增文档。
type CIncrement struct {
	Key   string `bson:"_id"`
	Value int64  `bson:"value"`
}

// CollectionName 返回集合名。
func (c *CIncrement) CollectionName() string { return "increment" }

// CPlayerProfile 表示 player_profile 集合中的玩家档案文档。
type CPlayerProfile struct {
	Id        int64     `bson:"_id"`
	ZoneId    int32     `bson:"zone_id"`
	AccountId int64     `bson:"account_id"`
	ShortId   string    `bson:"short_id"`
	CreatedAt time.Time `bson:"created_at"`
}

// CollectionName 返回集合名。
func (c *CPlayerProfile) CollectionName() string { return "player_profile" }

// GetID 返回文档 ID。
func (c *CPlayerProfile) GetID() int64 { return c.Id }

// HeroUnit 表示英雄单元的持久化结构。
type HeroUnit struct {
	ConfigId int `bson:"config_id"`
	Level    int `bson:"level"`
}

// CHero 表示 hero 集合中的英雄数据文档。
type CHero struct {
	Id     int64            `bson:"_id"`
	Heroes map[int]HeroUnit `bson:"heroes"`
}

// CollectionName 返回集合名。
func (c *CHero) CollectionName() string { return "hero" }

// GetID 返回文档 ID。
func (c *CHero) GetID() int64 { return c.Id }
