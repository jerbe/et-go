package db

import "errors"

var (
	// ErrCollectionNotFound 表示目标集合不存在。
	ErrCollectionNotFound = errors.New("db: collection not found")
	// ErrDocumentNotFound 表示目标文档不存在。
	ErrDocumentNotFound = errors.New("db: document not found")
	// ErrInvalidEntity 表示实体缺少有效的 `_id`。
	ErrInvalidEntity = errors.New("db: invalid entity, missing _id field")
	// ErrContextRequired 表示数据库操作必须显式提供 context。
	ErrContextRequired = errors.New("db: context required")
	// ErrDatabaseAddressRequired 表示 MongoDB URI 未配置。
	ErrDatabaseAddressRequired = errors.New("db: database address required")
	// ErrDatabaseNameRequired 表示数据库名未配置。
	ErrDatabaseNameRequired = errors.New("db: database name required")
	// ErrDBManagerClosed 表示 DBManager 已经关闭。
	ErrDBManagerClosed = errors.New("db: manager is closed")
)
