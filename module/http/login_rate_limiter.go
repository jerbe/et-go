package http

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

const (
	defaultLoginRateLimitWindow = time.Minute
	maxLoginRateLimitKeys       = 8192
)

var (
	// ErrLoginRateLimiterInvalid 表示限流配置非法。
	ErrLoginRateLimiterInvalid = errors.New("http: login rate limiter invalid")
	// ErrLoginRateLimiterClosed 表示限流组件已经销毁。
	ErrLoginRateLimiterClosed = errors.New("http: login rate limiter closed")
	// ErrLoginRateLimiterContextRequired 表示限流操作缺少 context。
	ErrLoginRateLimiterContextRequired = errors.New("http: login rate limiter context required")
)

type loginRateLimitWindow struct {
	start time.Time
	count int
}

// LoginRateLimiterComponent 对 HTTP 登录请求执行固定窗口限流。
//
// key 由请求来源和规范化用户名组成。该组件是进程内保护，不伪装成跨
// Process/分布式限流；多进程部署仍需要在网关或共享存储层配置同等策略。
//
// TODO(security): 真实 MongoDB 多 Process 故障演练、数据库不可用时的运营
// 策略和限流 bucket 保留/清理验收仍需外部部署协议；生产配置不会把本地
// map 当作全局安全边界。
type LoginRateLimiterComponent struct {
	ecs.BaseComponent

	Limit  int
	Window time.Duration

	mu      sync.Mutex
	windows map[string]loginRateLimitWindow
	closed  bool
}

// NewLoginRateLimiterComponent 创建登录限流组件。
func NewLoginRateLimiterComponent(limit int, window time.Duration) (*LoginRateLimiterComponent, error) {
	if limit <= 0 {
		return nil, ErrLoginRateLimiterInvalid
	}
	if window <= 0 {
		window = defaultLoginRateLimitWindow
	}
	return &LoginRateLimiterComponent{
		Limit:   limit,
		Window:  window,
		windows: make(map[string]loginRateLimitWindow),
	}, nil
}

func (c *LoginRateLimiterComponent) Type() string { return "LoginRateLimiterComponent" }

// Allow 判断 key 是否仍可发起登录请求。
func (c *LoginRateLimiterComponent) Allow(key string) bool {
	allowed, err := c.AllowContext(context.Background(), key)
	return err == nil && allowed
}

// AllowContext 在调用方 context 中判断 key 是否仍可发起登录请求。
func (c *LoginRateLimiterComponent) AllowContext(ctx context.Context, key string) (bool, error) {
	if c == nil {
		return false, ErrLoginRateLimiterClosed
	}
	if ctx == nil {
		return false, ErrLoginRateLimiterContextRequired
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, ErrLoginRateLimiterInvalid
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.Limit <= 0 {
		return false, ErrLoginRateLimiterClosed
	}
	if c.Window <= 0 {
		c.Window = defaultLoginRateLimitWindow
	}
	c.evictExpiredLocked(now)
	window, ok := c.windows[key]
	if !ok {
		if len(c.windows) >= maxLoginRateLimitKeys {
			return false, nil
		}
		window = loginRateLimitWindow{start: now}
	}
	if now.Sub(window.start) >= c.Window {
		window = loginRateLimitWindow{start: now}
	}
	window.count++
	c.windows[key] = window
	return window.count <= c.Limit, nil
}

// OnDestroy 关闭并清理限流状态。
func (c *LoginRateLimiterComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.windows = nil
	c.mu.Unlock()
}

func (c *LoginRateLimiterComponent) evictExpiredLocked(now time.Time) {
	for key, window := range c.windows {
		if now.Sub(window.start) >= c.Window {
			delete(c.windows, key)
		}
	}
}

func loginRateLimitKey(remoteAddr, username string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	username = strings.ToLower(strings.TrimSpace(username))
	if remoteAddr == "" {
		remoteAddr = "unknown"
	}
	return remoteAddr + "|" + username
}
