package http

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
)

const defaultLoginAuditCollection = "login_audit"

var (
	// ErrLoginAuditComponentInvalid 表示审计组件或 sink 类型不正确。
	ErrLoginAuditComponentInvalid = errors.New("http: login audit component invalid")
	// ErrLoginAuditContextRequired 表示审计操作缺少 context。
	ErrLoginAuditContextRequired = errors.New("http: login audit context required")
	// ErrLoginAuditSinkUnavailable 表示审计持久化依赖不可用。
	ErrLoginAuditSinkUnavailable = errors.New("http: login audit sink unavailable")
)

// LoginAuditEvent 是一次 HTTP 登录尝试的不可变审计事件。
type LoginAuditEvent struct {
	Username   string
	AccountID  int64
	RemoteAddr string
	Success    bool
	Reason     string
	At         time.Time
}

// LoginAuditSink 定义登录审计的持久化边界。
type LoginAuditSink interface {
	RecordLoginAudit(context.Context, LoginAuditEvent) error
}

// LoginAuditComponent 将显式审计 sink 挂载到 HTTP Scene。
type LoginAuditComponent struct {
	ecs.BaseComponent
	Sink LoginAuditSink
}

func (c *LoginAuditComponent) Type() string { return "LoginAuditComponent" }

// MongoLoginAuditSink 将审计事件写入 MongoDB。
//
// 事件写入失败会返回错误；组件不回退到日志、内存或“认为已记录”的状态。
//
// TODO(security): 定义审计字段脱敏、保留期限、访问控制和归档策略；当前只
// 实现可靠写入与时间索引，不擅自删除或暴露审计证据。
type MongoLoginAuditSink struct {
	component  *db.DBComponent
	collection string
}

type loginAuditDocument struct {
	ID         int64     `bson:"_id"`
	Username   string    `bson:"username"`
	AccountID  int64     `bson:"account_id,omitempty"`
	RemoteAddr string    `bson:"remote_addr,omitempty"`
	Success    bool      `bson:"success"`
	Reason     string    `bson:"reason,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
}

func (d *loginAuditDocument) GetID() int64 {
	if d == nil {
		return 0
	}
	return d.ID
}

// NewMongoLoginAuditSink 创建显式 MongoDB 审计 sink。
func NewMongoLoginAuditSink(component *db.DBComponent) (*MongoLoginAuditSink, error) {
	if component == nil || component.Client() == nil {
		return nil, ErrLoginAuditSinkUnavailable
	}
	return &MongoLoginAuditSink{
		component:  component,
		collection: defaultLoginAuditCollection,
	}, nil
}

// DBManagerLoginAuditSink 通过 DBManager 懒加载当前 Zone 的审计数据库。
type DBManagerLoginAuditSink struct {
	manager    *db.DBManagerComponent
	zone       int
	collection string
}

// NewDBManagerLoginAuditSink 创建基于 DBManager 的审计 sink。
func NewDBManagerLoginAuditSink(manager *db.DBManagerComponent, zone int) (*DBManagerLoginAuditSink, error) {
	if manager == nil || zone <= 0 {
		return nil, ErrLoginAuditSinkUnavailable
	}
	return &DBManagerLoginAuditSink{
		manager:    manager,
		zone:       zone,
		collection: defaultLoginAuditCollection,
	}, nil
}

func (s *DBManagerLoginAuditSink) mongoSink() (*MongoLoginAuditSink, error) {
	if s == nil || s.manager == nil || s.zone <= 0 {
		return nil, ErrLoginAuditSinkUnavailable
	}
	component, err := s.manager.GetZoneDB(s.zone)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve zone %d database: %v", ErrLoginAuditSinkUnavailable, s.zone, err)
	}
	sink, err := NewMongoLoginAuditSink(component)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(s.collection) == "" {
		return nil, ErrLoginAuditSinkUnavailable
	}
	sink.collection = s.collection
	return sink, nil
}

// RecordLoginAudit 写入一条登录审计事件。
func (s *MongoLoginAuditSink) RecordLoginAudit(ctx context.Context, event LoginAuditEvent) error {
	if ctx == nil {
		return ErrLoginAuditContextRequired
	}
	if strings.TrimSpace(event.Username) == "" || event.At.IsZero() {
		return ErrLoginAuditSinkUnavailable
	}
	if s == nil || s.component == nil || s.component.Client() == nil || strings.TrimSpace(s.collection) == "" {
		return ErrLoginAuditSinkUnavailable
	}
	id, err := newLoginAuditID()
	if err != nil {
		return err
	}
	document := &loginAuditDocument{
		ID:         id,
		Username:   event.Username,
		AccountID:  event.AccountID,
		RemoteAddr: event.RemoteAddr,
		Success:    event.Success,
		Reason:     event.Reason,
		CreatedAt:  event.At,
	}
	if err := s.component.Insert(ctx, document, s.collection); err != nil {
		return fmt.Errorf("%w: insert audit event: %v", ErrLoginAuditSinkUnavailable, err)
	}
	return nil
}

// RecordLoginAudit 写入当前 Zone 的共享审计事件。
func (s *DBManagerLoginAuditSink) RecordLoginAudit(ctx context.Context, event LoginAuditEvent) error {
	sink, err := s.mongoSink()
	if err != nil {
		return err
	}
	return sink.RecordLoginAudit(ctx, event)
}

func newLoginAuditID() (int64, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, fmt.Errorf("%w: generate event id: %v", ErrLoginAuditSinkUnavailable, err)
	}
	id := int64(binary.BigEndian.Uint64(raw[:]) & uint64(^uint64(0)>>1))
	if id <= 0 {
		return 0, fmt.Errorf("%w: generated event id is zero", ErrLoginAuditSinkUnavailable)
	}
	return id, nil
}

func recordLoginAudit(
	scene *ecs.Scene,
	req *nethttp.Request,
	username string,
	accountID int64,
	success bool,
	reason string,
) error {
	if scene == nil {
		return nil
	}
	component, ok := scene.GetComponent("LoginAuditComponent")
	if !ok || component == nil {
		return nil
	}
	audit, ok := component.(*LoginAuditComponent)
	if !ok || audit == nil || audit.Sink == nil {
		return ErrLoginAuditComponentInvalid
	}
	ctx := context.Background()
	remoteAddr := ""
	if req != nil {
		if req.Context() != nil {
			ctx = req.Context()
		}
		remoteAddr = req.RemoteAddr
	}
	return audit.Sink.RecordLoginAudit(ctx, LoginAuditEvent{
		Username:   strings.TrimSpace(username),
		AccountID:  accountID,
		RemoteAddr: remoteAddr,
		Success:    success,
		Reason:     reason,
		At:         time.Now(),
	})
}
