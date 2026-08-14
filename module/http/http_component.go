package http

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	nethttp "net/http"
	"strings"
	"sync"
	"time"

	"github.com/jerbe/et-go/engine/ecs"
)

const maxRequestBodyBytes = 1 << 20

// HttpComponent 表示 HTTP 服务组件。
type HttpComponent struct {
	ecs.BaseComponent

	addr             string
	dispatcher       *HttpDispatcher
	server           *nethttp.Server
	listener         net.Listener
	registerDefaults bool
	allowedOrigins   map[string]struct{}
	tlsCertFile      string
	tlsKeyFile       string
	requireTLS       bool
	mu               sync.RWMutex
	serveErr         error
	closed           bool
}

// NewHttpComponent 创建 HTTP 服务组件。
func NewHttpComponent(addr string) *HttpComponent {
	return &HttpComponent{addr: addr, registerDefaults: true}
}

// NewBareHttpComponent 创建不注册默认路由的 HTTP 服务组件。
func NewBareHttpComponent(addr string) *HttpComponent {
	return &HttpComponent{addr: addr}
}

// SetCORSAllowedOrigins 设置精确匹配的跨域来源白名单。
func (c *HttpComponent) SetCORSAllowedOrigins(origins []string) {
	if c == nil {
		return
	}
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" && origin != "*" {
			allowed[origin] = struct{}{}
		}
	}
	c.mu.Lock()
	c.allowedOrigins = allowed
	c.mu.Unlock()
}

// ConfigureTLS 配置 HTTP 监听的 TLS 证书。
//
// requireTLS=true 时，Start 不允许回退到明文 TCP。证书和私钥必须同时
// 提供；未配置 TLS 时保留明文监听只适用于显式选择的内部/开发环境。
func (c *HttpComponent) ConfigureTLS(certFile, keyFile string, requireTLS bool) error {
	if c == nil {
		return ErrTLSConfigurationInvalid
	}
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if (certFile == "") != (keyFile == "") || (requireTLS && (certFile == "" || keyFile == "")) {
		return ErrTLSConfigurationInvalid
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrServerClosed
	}
	c.tlsCertFile = certFile
	c.tlsKeyFile = keyFile
	c.requireTLS = requireTLS
	c.mu.Unlock()
	return nil
}

// Type 返回组件名称。
func (c *HttpComponent) Type() string { return "HttpComponent" }

// Awake 初始化 HTTP Dispatcher；监听必须由 Start 显式执行。
func (c *HttpComponent) Awake() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureDispatcherLocked()
}

// Start 启动 HTTP 服务并返回监听错误。
func (c *HttpComponent) Start() error {
	if c == nil {
		return ErrServerNotStarted
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrServerClosed
	}
	c.ensureDispatcherLocked()
	if c.listener != nil || c.server != nil {
		c.mu.Unlock()
		return nil
	}
	addr := c.addr
	certFile := c.tlsCertFile
	keyFile := c.tlsKeyFile
	requireTLS := c.requireTLS
	if addr == "" {
		c.mu.Unlock()
		return ErrAddressRequired
	}
	entity := c.GetEntity()
	if entity == nil || entity.Scene() == nil {
		c.mu.Unlock()
		return ErrSceneRequired
	}
	scene := entity.Scene()
	dispatcher := c.dispatcher
	if requireTLS && (certFile == "" || keyFile == "") {
		c.mu.Unlock()
		return ErrTLSConfigurationInvalid
	}
	var tlsConfig *tls.Config
	if certFile != "" || keyFile != "" {
		if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
			c.mu.Unlock()
			return fmt.Errorf("%w: %v", ErrTLSCertificateLoad, err)
		}
		tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			// 初始证书在启动阶段严格校验；后续握手重新读取文件，
			// 允许部署层以原子替换方式轮换证书。轮换文件损坏时
			// 新连接直接握手失败，不回退到旧证书或明文。
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				next, err := tls.LoadX509KeyPair(certFile, keyFile)
				if err != nil {
					return nil, fmt.Errorf("%w: %v", ErrTLSCertificateLoad, err)
				}
				return &next, nil
			},
		}
	}
	handler := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		applyCrossDomainHeaders(w, r, c.allowedOriginsSnapshot())
		r.Body = nethttp.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		dispatcher.Dispatch(scene, w, r)
	})
	server := &nethttp.Server{
		Addr:    addr,
		Handler: handler,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	c.listener = listener
	c.server = server
	c.serveErr = nil
	c.addr = listener.Addr().String()
	listenerAddr := c.addr
	c.mu.Unlock()
	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, nethttp.ErrServerClosed) {
			c.mu.Lock()
			if c.server == server {
				c.serveErr = err
			}
			c.mu.Unlock()
			slog.Error("HTTP server stopped unexpectedly", "addr", listenerAddr, "err", err)
		}
	}()
	return nil
}

// OnDestroy 优雅关闭 HTTP 服务。
func (c *HttpComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	server := c.server
	c.server = nil
	c.listener = nil
	addr := c.addr
	c.mu.Unlock()
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown failed", "addr", addr, "err", err)
	}
}

// Addr 返回当前监听地址。
func (c *HttpComponent) Addr() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.addr
}

// Dispatcher 返回当前分发器。
func (c *HttpComponent) Dispatcher() *HttpDispatcher {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dispatcher
}

// ServeError 返回异步 Serve 的非正常退出错误。
func (c *HttpComponent) ServeError() error {
	if c == nil {
		return ErrServerNotStarted
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serveErr
}

func (c *HttpComponent) ensureDispatcherLocked() {
	if c.dispatcher != nil || c.closed {
		return
	}
	c.dispatcher = &HttpDispatcher{}
	if c.registerDefaults {
		c.dispatcher.Register("/login", &HttpPostLoginHandler{})
		c.dispatcher.Register("/register", &HttpPostRegisterHandler{})
		c.dispatcher.Register("/get_area_list", &HttpGetAreaListHandler{})
	}
}

func (c *HttpComponent) allowedOriginsSnapshot() map[string]struct{} {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	allowed := make(map[string]struct{}, len(c.allowedOrigins))
	for origin := range c.allowedOrigins {
		allowed[origin] = struct{}{}
	}
	return allowed
}

// ErrServerNotStarted 表示 HTTP 服务尚未启动。
var ErrServerNotStarted = errors.New("http: server not started")

// ErrAddressRequired 表示 HTTP 没有配置监听地址。
var ErrAddressRequired = errors.New("http: listen address required")

// ErrSceneRequired 表示 HTTP 组件尚未挂载到 Scene。
var ErrSceneRequired = errors.New("http: scene required")

func applyCrossDomainHeaders(w nethttp.ResponseWriter, r *nethttp.Request, allowed map[string]struct{}) {
	if w == nil {
		return
	}
	origin := ""
	if r != nil {
		origin = r.Header.Get("Origin")
	}
	if _, ok := allowed[origin]; !ok || origin == "" {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
	if r.Method == nethttp.MethodOptions {
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST")
		w.Header().Set("Access-Control-Max-Age", "1728000")
	}
}
