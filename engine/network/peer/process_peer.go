package peer

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/engine/network/kcp"
	"github.com/jerbe/et-go/engine/timer"
)

const (
	processPeerHandshakeMsgID    uint16 = 65500
	processPeerProtocolVersion          = 1
	defaultPeerHandshakeTimeout         = 3 * time.Second
	defaultPeerReconnectInterval        = time.Second
)

var (
	ErrPeerConfigInvalid    = network.ErrPeerConfigInvalid
	ErrPeerComponentClosed  = network.ErrPeerComponentClosed
	ErrPeerHandshakeInvalid = network.ErrPeerHandshakeInvalid
	ErrPeerHandshakeTimeout = network.ErrPeerHandshakeTimeout
)

// PeerEndpoint 描述一个远端 Process 的 NetInner 地址。
type PeerEndpoint struct {
	ProcessID int
	Address   string
	Secret    string
}

type peerHandshake struct {
	Version         uint16 `json:"version"`
	SenderProcessID int    `json:"sender_process_id"`
	TargetProcessID int    `json:"target_process_id"`
	Nonce           string `json:"nonce"`
	MAC             string `json:"mac"`
}

type peerSessionState struct {
	expectedProcessID int
	processID         int
	established       bool
}

// ProcessPeerComponent 驱动 NetInner KCP、peer 握手和 ProcessOuterSender。
//
// 它只负责连接生命周期和协议边界；业务消息仍由 ProcessOuterSender 根据
// Actor envelope 投递到目标 Fiber，不在网络 goroutine 中直接执行业务代码。
type ProcessPeerComponent struct {
	ecs.BaseComponent

	ProcessID         int
	ListenAddr        string
	ProtocolVersion   uint16
	HandshakeTimeout  time.Duration
	ReconnectInterval time.Duration
	Sender            *actor.ProcessOuterSender
	PeerEndpoints     []PeerEndpoint

	ctx     context.Context
	cancel  context.CancelFunc
	service *kcp.KService

	mu           sync.Mutex
	endpoints    map[int]PeerEndpoint
	sessions     map[*network.Session]*peerSessionState
	connected    map[int]*network.Session
	connecting   map[int]bool
	nextAttempt  map[int]time.Time
	started      bool
	closed       bool
	sessionIDGen atomic.Int64
}

// NewProcessPeerComponent 创建跨进程 peer 组件。
func NewProcessPeerComponent(processID int, listenAddr string, sender *actor.ProcessOuterSender, endpoints []PeerEndpoint) *ProcessPeerComponent {
	return &ProcessPeerComponent{
		ProcessID:         processID,
		ListenAddr:        listenAddr,
		ProtocolVersion:   processPeerProtocolVersion,
		HandshakeTimeout:  defaultPeerHandshakeTimeout,
		ReconnectInterval: defaultPeerReconnectInterval,
		Sender:            sender,
		PeerEndpoints:     append([]PeerEndpoint(nil), endpoints...),
	}
}

// Type 返回组件名称。
func (c *ProcessPeerComponent) Type() string { return "ProcessPeerComponent" }

// SetContext 注入 Fiber 生命周期上下文。
func (c *ProcessPeerComponent) SetContext(ctx context.Context) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started || c.closed {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.ctx = ctx
}

// Start 启动 NetInner 监听和 peer 管理。
func (c *ProcessPeerComponent) Start() error {
	if c == nil {
		return ErrPeerComponentClosed
	}
	if c.ProcessID <= 0 || strings.TrimSpace(c.ListenAddr) == "" || c.Sender == nil {
		return ErrPeerConfigInvalid
	}
	if c.ProtocolVersion == 0 {
		c.ProtocolVersion = processPeerProtocolVersion
	}
	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = defaultPeerHandshakeTimeout
	}
	if c.ReconnectInterval <= 0 {
		c.ReconnectInterval = defaultPeerReconnectInterval
	}

	endpoints := make(map[int]PeerEndpoint, len(c.PeerEndpoints))
	for _, endpoint := range c.PeerEndpoints {
		if endpoint.ProcessID <= 0 || endpoint.ProcessID == c.ProcessID ||
			strings.TrimSpace(endpoint.Address) == "" || strings.TrimSpace(endpoint.Secret) == "" {
			return fmt.Errorf("%w: process=%d peer=%+v", ErrPeerConfigInvalid, c.ProcessID, endpoint)
		}
		if _, exists := endpoints[endpoint.ProcessID]; exists {
			return fmt.Errorf("%w: duplicate peer process=%d", ErrPeerConfigInvalid, endpoint.ProcessID)
		}
		endpoints[endpoint.ProcessID] = PeerEndpoint{
			ProcessID: endpoint.ProcessID,
			Address:   strings.TrimSpace(endpoint.Address),
			Secret:    endpoint.Secret,
		}
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrPeerComponentClosed
	}
	if c.started {
		c.mu.Unlock()
		return nil
	}
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	c.ctx = ctx
	c.cancel = cancel
	c.endpoints = endpoints
	c.sessions = make(map[*network.Session]*peerSessionState)
	c.connected = make(map[int]*network.Session)
	c.connecting = make(map[int]bool)
	c.nextAttempt = make(map[int]time.Time)
	c.mu.Unlock()

	service := kcp.NewService(kcp.InnerConfig(), nil)
	service.SetOnAccept(c.accept)
	if err := service.Listen(strings.TrimSpace(c.ListenAddr)); err != nil {
		cancel()
		c.mu.Lock()
		c.endpoints = nil
		c.sessions = nil
		c.connected = nil
		c.connecting = nil
		c.nextAttempt = nil
		c.cancel = nil
		c.mu.Unlock()
		return err
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		service.Close()
		cancel()
		return ErrPeerComponentClosed
	}
	c.service = service
	if addr := service.Addr(); addr != nil {
		c.ListenAddr = addr.String()
	}
	c.started = true
	c.mu.Unlock()
	return nil
}

// Update 驱动 KCP 和缺失 peer 的重连。
func (c *ProcessPeerComponent) Update() {
	if c == nil {
		return
	}
	c.mu.Lock()
	service := c.service
	if !c.started || c.closed || service == nil {
		c.mu.Unlock()
		return
	}
	endpoints := make([]PeerEndpoint, 0)
	now := time.Now()
	for processID, endpoint := range c.endpoints {
		if c.ProcessID > processID {
			// 同一 peer 只允许较小 ProcessID 主动拨号，避免双向同时
			// handshake 导致 Session 竞争和 pending RPC 被替换路径失败。
			continue
		}
		if c.connected[processID] != nil || c.connecting[processID] {
			continue
		}
		if next := c.nextAttempt[processID]; !next.IsZero() && now.Before(next) {
			continue
		}
		c.connecting[processID] = true
		endpoints = append(endpoints, endpoint)
	}
	c.mu.Unlock()

	service.Update()
	for _, endpoint := range endpoints {
		go c.connect(endpoint)
	}
}

// ConnectedProcesses 返回当前已完成握手的远端 ProcessID。
func (c *ProcessPeerComponent) ConnectedProcesses() []int {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]int, 0, len(c.connected))
	for processID, session := range c.connected {
		if session != nil && !session.IsClosed() {
			result = append(result, processID)
		}
	}
	return result
}

// OnDestroy 停止服务、关闭会话并使 pending RPC fail-fast。
func (c *ProcessPeerComponent) OnDestroy() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.started = false
	cancel := c.cancel
	service := c.service
	c.cancel = nil
	c.service = nil
	sessions := make([]*network.Session, 0, len(c.sessions))
	for session := range c.sessions {
		sessions = append(sessions, session)
	}
	c.sessions = nil
	c.connected = nil
	c.connecting = nil
	c.nextAttempt = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if service != nil {
		service.Close()
	}
	for _, session := range sessions {
		if session != nil {
			session.Close()
		}
	}
}

func (c *ProcessPeerComponent) accept(_ *kcp.KChannel, conn net.Conn) {
	if c == nil || conn == nil {
		return
	}
	c.attachSession(network.NewSession(c.context(), c.nextSessionID(), conn, nil), 0)
}

func (c *ProcessPeerComponent) connect(endpoint PeerEndpoint) {
	c.mu.Lock()
	service := c.service
	closed := c.closed
	timeout := c.HandshakeTimeout
	c.mu.Unlock()
	if closed || service == nil {
		return
	}

	channel, err := service.Connect(endpoint.Address)
	if err != nil {
		c.connectionFailed(endpoint.ProcessID, err)
		return
	}
	conn := channel.Conn()
	if conn == nil {
		channel.Close()
		c.connectionFailed(endpoint.ProcessID, fmt.Errorf("peer %d returned nil connection", endpoint.ProcessID))
		return
	}
	session := network.NewSession(c.context(), c.nextSessionID(), conn, nil)
	c.attachSession(session, endpoint.ProcessID)

	nonce, err := newPeerNonce()
	if err != nil {
		session.Close()
		c.connectionFailed(endpoint.ProcessID, err)
		return
	}
	payload, err := marshalPeerHandshake(peerHandshake{
		Version:         c.ProtocolVersion,
		SenderProcessID: c.ProcessID,
		TargetProcessID: endpoint.ProcessID,
		Nonce:           nonce,
	}, endpoint.Secret)
	if err != nil {
		session.Close()
		c.connectionFailed(endpoint.ProcessID, err)
		return
	}

	ctx, cancel := context.WithTimeout(c.context(), timeout)
	defer cancel()
	response, err := session.Call(ctx, &codec.Packet{
		Type:    codec.PacketTypeRequest,
		MsgID:   processPeerHandshakeMsgID,
		Payload: payload,
	})
	if err != nil {
		session.Close()
		c.connectionFailed(endpoint.ProcessID, err)
		return
	}
	handshake, err := unmarshalPeerHandshake(response.Payload)
	if err != nil || handshake.Version != c.ProtocolVersion ||
		handshake.SenderProcessID != endpoint.ProcessID ||
		handshake.TargetProcessID != c.ProcessID ||
		handshake.Nonce != nonce {
		session.Close()
		c.connectionFailed(endpoint.ProcessID, ErrPeerHandshakeInvalid)
		return
	}
	if !verifyPeerHandshake(handshake, endpoint.Secret) {
		session.Close()
		c.connectionFailed(endpoint.ProcessID, ErrPeerHandshakeInvalid)
		return
	}
	c.establish(session, endpoint.ProcessID)
}

func (c *ProcessPeerComponent) attachSession(session *network.Session, expectedProcessID int) {
	if session == nil {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		session.Close()
		return
	}
	if c.sessions == nil {
		c.sessions = make(map[*network.Session]*peerSessionState)
	}
	c.sessions[session] = &peerSessionState{expectedProcessID: expectedProcessID}
	c.mu.Unlock()
	session.SetOnClose(func() {
		c.sessionClosed(session)
	})
	session.StartReadLoop(c.handlePacket)
	session.StartWriteLoop()
}

func (c *ProcessPeerComponent) handlePacket(session *network.Session, packet *codec.Packet) {
	if c == nil || session == nil || packet == nil {
		if session != nil {
			session.Close()
		}
		return
	}
	if packet.MsgID == processPeerHandshakeMsgID && packet.Type == codec.PacketTypeRequest {
		c.handleIncomingHandshake(session, packet)
		return
	}
	c.mu.Lock()
	state := c.sessions[session]
	established := state != nil && state.established
	c.mu.Unlock()
	if !established {
		session.Close()
		return
	}
	if c.Sender == nil {
		session.Close()
		return
	}
	if err := c.Sender.HandleSessionPacket(c.context(), session, packet); err != nil {
		session.Close()
	}
}

func (c *ProcessPeerComponent) handleIncomingHandshake(session *network.Session, packet *codec.Packet) {
	if packet == nil || packet.RpcID == 0 {
		if session != nil {
			session.Close()
		}
		return
	}
	request, err := unmarshalPeerHandshake(packet.Payload)
	if err != nil || request.Version != c.ProtocolVersion ||
		request.TargetProcessID != c.ProcessID || request.SenderProcessID <= 0 ||
		request.SenderProcessID == c.ProcessID {
		session.Close()
		return
	}
	c.mu.Lock()
	endpoint, ok := c.endpoints[request.SenderProcessID]
	c.mu.Unlock()
	if !ok || !verifyPeerHandshake(request, endpoint.Secret) {
		session.Close()
		return
	}
	response, err := marshalPeerHandshake(peerHandshake{
		Version:         c.ProtocolVersion,
		SenderProcessID: c.ProcessID,
		TargetProcessID: request.SenderProcessID,
		Nonce:           request.Nonce,
	}, endpoint.Secret)
	if err != nil {
		session.Close()
		return
	}
	if err := session.Send(&codec.Packet{
		Type:    codec.PacketTypeResponse,
		MsgID:   processPeerHandshakeMsgID,
		RpcID:   packet.RpcID,
		Payload: response,
	}); err != nil {
		session.Close()
		return
	}
	c.establish(session, request.SenderProcessID)
}

func (c *ProcessPeerComponent) establish(session *network.Session, processID int) {
	if session == nil || processID <= 0 {
		if session != nil {
			session.Close()
		}
		return
	}
	c.mu.Lock()
	state := c.sessions[session]
	if state == nil || c.closed {
		c.mu.Unlock()
		session.Close()
		return
	}
	if state.expectedProcessID != 0 && state.expectedProcessID != processID {
		c.mu.Unlock()
		session.Close()
		return
	}
	old := c.connected[processID]
	state.processID = processID
	state.established = true
	c.connected[processID] = session
	c.connecting[processID] = false
	c.nextAttempt[processID] = time.Time{}
	c.mu.Unlock()

	if c.Sender != nil {
		c.Sender.AddSession(processID, session)
	}
	if old != nil && old != session {
		old.Close()
	}
}

func (c *ProcessPeerComponent) sessionClosed(session *network.Session) {
	if c == nil || session == nil {
		return
	}
	c.mu.Lock()
	state := c.sessions[session]
	delete(c.sessions, session)
	if state == nil {
		c.mu.Unlock()
		return
	}
	processID := state.processID
	if processID == 0 {
		processID = state.expectedProcessID
	}
	if processID > 0 && c.connected[processID] == session {
		delete(c.connected, processID)
		if c.Sender != nil {
			c.Sender.RemoveSession(processID)
		}
	}
	if !c.closed && processID > 0 {
		c.connecting[processID] = false
		c.nextAttempt[processID] = time.Now().Add(c.ReconnectInterval)
	}
	c.mu.Unlock()
}

func (c *ProcessPeerComponent) connectionFailed(processID int, cause error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.connecting[processID] = false
	c.nextAttempt[processID] = time.Now().Add(c.ReconnectInterval)
	c.mu.Unlock()

	if cause == nil {
		return
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		cause = fmt.Errorf("%w: peer_process=%d", ErrPeerHandshakeTimeout, processID)
	}
	slog.Warn(
		"peer connection failed; retry scheduled",
		"process_id", c.ProcessID,
		"peer_process_id", processID,
		"retry_after", c.ReconnectInterval,
		"err", cause,
	)
}

func (c *ProcessPeerComponent) context() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func newPeerNonce() (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce), nil
}

func marshalPeerHandshake(handshake peerHandshake, secret string) ([]byte, error) {
	if handshake.Version == 0 || handshake.SenderProcessID <= 0 ||
		handshake.TargetProcessID <= 0 || handshake.Nonce == "" || secret == "" {
		return nil, ErrPeerHandshakeInvalid
	}
	handshake.MAC = hex.EncodeToString(peerHandshakeMAC(handshake, secret))
	return json.Marshal(handshake)
}

func unmarshalPeerHandshake(data []byte) (peerHandshake, error) {
	var handshake peerHandshake
	if len(data) == 0 || json.Unmarshal(data, &handshake) != nil {
		return peerHandshake{}, ErrPeerHandshakeInvalid
	}
	if handshake.Version == 0 || handshake.SenderProcessID <= 0 ||
		handshake.TargetProcessID <= 0 || handshake.Nonce == "" || handshake.MAC == "" {
		return peerHandshake{}, ErrPeerHandshakeInvalid
	}
	return handshake, nil
}

func verifyPeerHandshake(handshake peerHandshake, secret string) bool {
	if secret == "" {
		return false
	}
	received, err := hex.DecodeString(handshake.MAC)
	if err != nil {
		return false
	}
	expected := peerHandshakeMAC(handshake, secret)
	return hmac.Equal(received, expected)
}

func peerHandshakeMAC(handshake peerHandshake, secret string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d|%d|%d|%s", handshake.Version, handshake.SenderProcessID, handshake.TargetProcessID, handshake.Nonce)
	return mac.Sum(nil)
}

func (c *ProcessPeerComponent) nextSessionID() int64 {
	return c.sessionIDGen.Add(1)
}

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeNetInner, initNetInnerFiber)
}

func initNetInnerFiber(f *fiber.Fiber) error {
	if f == nil || f.Root() == nil {
		return ErrPeerConfigInvalid
	}
	cfg := config.GetGlobal()
	if cfg == nil {
		return ErrPeerConfigInvalid
	}
	process, ok := processConfig(cfg, f.ProcessID())
	if !ok {
		return fmt.Errorf("%w: process %d is not configured", ErrPeerConfigInvalid, f.ProcessID())
	}
	listenAddr, err := processListenAddr(cfg, process)
	if err != nil {
		return err
	}
	endpoints := make([]PeerEndpoint, 0, len(process.Peers))
	for _, peer := range process.Peers {
		endpoints = append(endpoints, PeerEndpoint{
			ProcessID: peer.ProcessID,
			Address:   peer.Address,
			Secret:    peer.Secret,
		})
	}

	scene := f.Root()
	scene.AddComponent(actor.NewMailBox(actor.SceneActorID(scene), actor.MailBoxTypeUnOrderedMessage))
	scene.AddComponent(&timer.TimerComponent{})
	scene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	sender := actor.NewProcessOuterSender(actor.NewRpcManager())
	sender.SetFiberManager(f.Manager())
	scene.AddComponent(sender)
	peerComponent := NewProcessPeerComponent(f.ProcessID(), listenAddr, sender, endpoints)
	peerComponent.SetContext(f.Context())
	scene.AddComponent(peerComponent)
	if err := peerComponent.Start(); err != nil {
		return err
	}
	if err := actor.RegisterProcessOuterSender(f.ProcessID(), sender); err != nil {
		peerComponent.OnDestroy()
		return err
	}
	f.RegisterUpdate(peerComponent)
	return nil
}

func processConfig(cfg *config.Config, processID int) (config.StartProcessConfig, bool) {
	if cfg == nil {
		return config.StartProcessConfig{}, false
	}
	for _, process := range cfg.Processes {
		if process.ID == processID {
			return process, true
		}
	}
	return config.StartProcessConfig{}, false
}

func processListenAddr(cfg *config.Config, process config.StartProcessConfig) (string, error) {
	if process.InnerPort <= 0 {
		return "", fmt.Errorf("%w: process %d innerPort must be positive", ErrPeerConfigInvalid, process.ID)
	}
	for _, machine := range cfg.Machines {
		if machine.ID != process.MachineID {
			continue
		}
		host := strings.TrimSpace(machine.InnerIP)
		if host == "" {
			host = strings.TrimSpace(machine.OuterIP)
		}
		if host == "" {
			return "", fmt.Errorf("%w: process %d machine %d has no address", ErrPeerConfigInvalid, process.ID, process.MachineID)
		}
		return net.JoinHostPort(host, strconv.Itoa(process.InnerPort)), nil
	}
	return "", fmt.Errorf("%w: process %d machine %d missing", ErrPeerConfigInvalid, process.ID, process.MachineID)
}
