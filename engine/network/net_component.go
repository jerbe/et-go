package network

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/engine/network/kcp"
)

var netComponentSessionID atomic.Int64

// PacketHandler 定义 NetComponent 的入站包处理器。
type PacketHandler func(scene *ecs.Scene, session *Session, packet *codec.Packet) (*codec.Packet, error)

// SessionAcceptHandler 定义连接建立后的回调。
type SessionAcceptHandler func(scene *ecs.Scene, session *Session)

// NetComponent 负责为场景挂载 KCP 网络入口。
type NetComponent struct {
	ecs.BaseComponent

	Protocol      string
	Address       string
	KCPConfig     *kcp.Config
	packetHandler PacketHandler
	acceptHandler SessionAcceptHandler

	service          *kcp.KService
	started          bool
	mu               sync.RWMutex
	closed           bool
	updateRegistered bool
}

// NewNetComponent 创建网络组件。
func NewNetComponent(protocol, address string) *NetComponent {
	return &NetComponent{
		Protocol: protocol,
		Address:  address,
	}
}

// Type 返回组件名称。
func (c *NetComponent) Type() string { return "NetComponent" }

// SetPacketHandler 设置包处理回调。
func (c *NetComponent) SetPacketHandler(handler PacketHandler) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.packetHandler = handler
}

// SetAcceptHandler 设置连接建立回调。
func (c *NetComponent) SetAcceptHandler(handler SessionAcceptHandler) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.acceptHandler = handler
}

// Addr 返回当前监听地址。
func (c *NetComponent) Addr() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	service := c.service
	address := c.Address
	c.mu.RUnlock()
	if service == nil || service.Addr() == nil {
		return address
	}
	return service.Addr().String()
}

// Start 启动底层网络服务。
func (c *NetComponent) Start() error {
	if c == nil {
		return ErrNetComponentClosed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrNetComponentClosed
	}
	if c.started {
		return nil
	}

	protocol := strings.ToLower(strings.TrimSpace(c.Protocol))
	if protocol == "" {
		return ErrProtocolRequired
	}
	if protocol != "kcp" {
		return errors.New("network: unsupported protocol")
	}

	addr := c.Address
	if addr == "" {
		resolved, err := ResolveSceneListenAddr(c.scene(), true)
		if err != nil {
			return err
		}
		addr = resolved
	}

	service := kcp.NewService(c.kcpConfigLocked(), nil)
	if err := service.Listen(addr); err != nil {
		return err
	}
	service.SetOnAccept(func(_ *kcp.KChannel, conn net.Conn) {
		c.bindAcceptedConn(conn)
	})

	c.service = service
	c.started = true
	if listenAddr := service.Addr(); listenAddr != nil {
		c.Address = listenAddr.String()
	}
	return nil
}

// Awake 仅注册 Fiber Update。
//
// 监听属于显式启动动作，调用方必须在把组件加入 Scene 前调用 Start，
// 这样启动错误不会被 ECS 生命周期吞掉。
func (c *NetComponent) Awake() {
	if c == nil {
		return
	}
	scene := c.scene()
	if scene == nil {
		return
	}
	registrar, ok := scene.Fiber().(interface{ RegisterUpdate(ecs.UpdateSystem) })
	if !ok || registrar == nil {
		return
	}
	c.mu.Lock()
	if c.closed || c.updateRegistered {
		c.mu.Unlock()
		return
	}
	c.updateRegistered = true
	registrar.RegisterUpdate(c)
	c.mu.Unlock()
}

// Update 驱动底层网络服务轮询。
func (c *NetComponent) Update() {
	if c == nil {
		return
	}
	c.mu.RLock()
	service := c.service
	closed := c.closed
	c.mu.RUnlock()
	if service != nil && !closed {
		service.Update()
	}
}

// OnDestroy 停止服务并取消 Update 注册。
func (c *NetComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	service := c.service
	c.service = nil
	c.started = false
	updateRegistered := c.updateRegistered
	c.updateRegistered = false
	c.mu.Unlock()
	if updateRegistered {
		if scene := c.scene(); scene != nil {
			if registrar, ok := scene.Fiber().(interface{ UnregisterUpdate(ecs.UpdateSystem) }); ok {
				registrar.UnregisterUpdate(c)
			}
		}
	}
	if service != nil {
		service.Close()
	}
}

func (c *NetComponent) bindAcceptedConn(conn net.Conn) {
	if conn == nil {
		return
	}
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		_ = conn.Close()
		return
	}

	scene := c.scene()
	sessionContext := context.Background()
	if scene != nil {
		if provider, ok := scene.Fiber().(interface{ Context() context.Context }); ok {
			if provided := provider.Context(); provided != nil {
				sessionContext = provided
			}
		}
	}
	session := NewSession(sessionContext, netComponentSessionID.Add(1), conn, nil)
	entity := ecs.NewEntity()
	session.SetEntity(entity)

	acceptTimeout := NewSessionAcceptTimeoutComponent(session, defaultSessionAcceptTimeout)
	idleChecker := NewSessionIdleCheckerComponent(session, defaultSessionIdleCheckInterval, defaultSessionIdleTimeout)
	entity.AddComponent(acceptTimeout)
	entity.AddComponent(idleChecker)
	session.setAcceptTimeoutComponent(acceptTimeout)
	session.setIdleCheckerComponent(idleChecker)

	if scene != nil {
		scene.AddChildWithID(session.ID(), entity)
	}

	session.SetOnClose(func() {
		if entity != nil {
			entity.Dispose()
		}
	})

	session.StartReadLoop(func(sess *Session, pkt *codec.Packet) {
		c.mu.RLock()
		handler := c.packetHandler
		c.mu.RUnlock()
		if handler == nil {
			return
		}
		resp, err := handler(scene, sess, pkt)
		if err != nil {
			slog.Error(
				"network packet handler failed",
				"scene_type", sceneType(scene),
				"session_id", sess.ID(),
				"msg_id", pkt.MsgID,
				"rpc_id", pkt.RpcID,
				"err", err,
			)
			sess.Close()
			return
		}
		if resp != nil {
			if err := sess.Send(resp); err != nil {
				sess.Close()
			}
		}
	})
	session.StartWriteLoop()

	c.mu.RLock()
	acceptHandler := c.acceptHandler
	c.mu.RUnlock()
	if acceptHandler != nil {
		acceptHandler(scene, session)
	}
}

func sceneType(scene *ecs.Scene) string {
	if scene == nil {
		return ""
	}
	return scene.SceneType().String()
}

func (c *NetComponent) scene() *ecs.Scene {
	entity := c.GetEntity()
	if entity == nil {
		return nil
	}
	return entity.Scene()
}

func (c *NetComponent) kcpConfig() *kcp.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.kcpConfigLocked()
}

func (c *NetComponent) kcpConfigLocked() *kcp.Config {
	if c.KCPConfig != nil {
		return c.KCPConfig
	}
	return kcp.OuterConfig()
}
