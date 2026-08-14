package kcp

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	etlog "github.com/jerbe/et-go/internal/log"
)

const (
	// controlFrameSize 对齐 ET 原始 KService 的 SYN/ACK/Router 帧：
	// flag + sender localConn + receiver localConn。
	controlFrameSize = 1 + 4 + 4
	// messageFrameSize 对齐 ET 原始 KChannel.Output：
	// flag + sender localConn + 标准 KCP segment。
	messageFrameSize = 1 + 4
	// finFrameSize 对齐 ET 原始 KService.Disconnect：
	// flag + sender localConn + receiver localConn + error。
	finFrameSize = 1 + 4 + 4 + 4
)

// KService 管理所有 KChannel。
type KService struct {
	mu         sync.RWMutex
	conn       *net.UDPConn
	config     *Config
	channels   map[uint32]*KChannel        // 以本地 localConn 索引。
	waitAccept map[uint32]*KChannel        // 主动连接：以本地 localConn 索引。
	acceptWait map[acceptWaitKey]*KChannel // 被动连接：以远端地址和 localConn 索引。
	onAccept   func(ch *KChannel, conn net.Conn)
	nextConv   uint32
	recvBuf    []byte
	logger     *etlog.Logger
}

type acceptWaitKey struct {
	remoteAddr string
	localConn  uint32
}

// NewService 创建 KCP 服务。
func NewService(config *Config, logger *etlog.Logger) *KService {
	if config == nil {
		config = InnerConfig()
	}
	return &KService{
		config:     config,
		channels:   make(map[uint32]*KChannel),
		waitAccept: make(map[uint32]*KChannel),
		acceptWait: make(map[acceptWaitKey]*KChannel),
		recvBuf:    make([]byte, 64*1024),
		// ET 的连接 ID 不使用 0；从 100 开始也能避免与控制值混淆。
		nextConv: 99,
		logger:   logger,
	}
}

// Listen 监听 UDP 地址。
func (s *KService) Listen(addr string) error {
	if s == nil {
		return ErrServiceNotListening
	}
	if strings.TrimSpace(addr) == "" {
		return ErrAddressRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		return errors.New("kcp: service already listening")
	}

	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	s.conn = conn
	return nil
}

// Addr 返回当前监听地址。
func (s *KService) Addr() *net.UDPAddr {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return nil
	}
	addr, _ := conn.LocalAddr().(*net.UDPAddr)
	return addr
}

// SetOnAccept 设置新连接回调。
func (s *KService) SetOnAccept(fn func(ch *KChannel, conn net.Conn)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.onAccept = fn
	s.mu.Unlock()
}

// Connect 主动连接目标地址。
func (s *KService) Connect(addr string) (*KChannel, error) {
	if s == nil {
		return nil, ErrServiceNotListening
	}
	s.mu.RLock()
	listening := s.conn != nil
	config := s.config
	s.mu.RUnlock()
	if !listening {
		return nil, ErrServiceNotListening
	}

	remoteAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	localConn := s.nextChannelID()
	channel := newConnectingKChannel(localConn, remoteAddr, config, s)

	s.mu.Lock()
	s.channels[localConn] = channel
	s.waitAccept[localConn] = channel
	s.mu.Unlock()
	s.prepareConnection(channel)

	if err := s.sendControlPair(remoteAddr, ProtoSYN, localConn, 0, nil); err != nil {
		channel.close(true)
		return nil, err
	}
	channel.markHandshakeSent(time.Now().UnixMilli())
	return channel, nil
}

// Update 执行 KCP 服务主循环。
func (s *KService) Update() {
	if s == nil {
		return
	}
	now := time.Now().UnixMilli()
	s.timerOut(now)
	s.checkWaitAccept(now)
	s.recv()
	s.updateChannels(uint32(now))
}

// Close 关闭服务及所有通道。
func (s *KService) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	seen := make(map[*KChannel]struct{}, len(s.channels)+len(s.waitAccept)+len(s.acceptWait))
	channels := make([]*KChannel, 0, len(s.channels)+len(s.waitAccept)+len(s.acceptWait))
	for _, channel := range s.channels {
		if channel != nil {
			if _, ok := seen[channel]; !ok {
				seen[channel] = struct{}{}
				channels = append(channels, channel)
			}
		}
	}
	for _, channel := range s.waitAccept {
		if channel != nil {
			if _, ok := seen[channel]; !ok {
				seen[channel] = struct{}{}
				channels = append(channels, channel)
			}
		}
	}
	for _, channel := range s.acceptWait {
		if channel != nil {
			if _, ok := seen[channel]; !ok {
				seen[channel] = struct{}{}
				channels = append(channels, channel)
			}
		}
	}
	s.channels = make(map[uint32]*KChannel)
	s.waitAccept = make(map[uint32]*KChannel)
	s.acceptWait = make(map[acceptWaitKey]*KChannel)
	conn := s.conn
	s.conn = nil
	s.mu.Unlock()

	for _, channel := range channels {
		channel.close(true)
	}
	if conn != nil {
		if err := conn.Close(); err != nil {
			s.logWarn("KCP UDP 连接关闭失败", "err", err)
		}
	}
}

func (s *KService) timerOut(now int64) {
	s.mu.RLock()
	channels := make([]*KChannel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, channel)
	}
	s.mu.RUnlock()

	for _, channel := range channels {
		if channel != nil && channel.IsTimeout(now) {
			channel.close(true)
		}
	}
}

func (s *KService) checkWaitAccept(now int64) {
	s.mu.RLock()
	seen := make(map[*KChannel]struct{}, len(s.waitAccept)+len(s.acceptWait))
	channels := make([]*KChannel, 0, len(s.waitAccept)+len(s.acceptWait))
	for _, channel := range s.waitAccept {
		if channel != nil {
			seen[channel] = struct{}{}
			channels = append(channels, channel)
		}
	}
	for _, channel := range s.acceptWait {
		if channel != nil {
			if _, ok := seen[channel]; !ok {
				channels = append(channels, channel)
			}
		}
	}
	s.mu.RUnlock()

	for _, channel := range channels {
		if channel.IsTimeout(now) {
			channel.close(true)
			continue
		}
		if !channel.needsHandshakeRetry(now) {
			continue
		}
		localConn, remoteConn, remoteAddr, accepted := channel.handshakeState()
		var err error
		if accepted {
			err = s.sendControlPair(remoteAddr, ProtoACK, localConn, remoteConn, nil)
		} else {
			err = s.sendControlPair(remoteAddr, ProtoSYN, localConn, 0, nil)
		}
		if err != nil {
			channel.close(true)
			continue
		}
		channel.markHandshakeSent(now)
	}
}

func (s *KService) recv() {
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		s.logWarn("KCP UDP read deadline 设置失败", "err", err)
		return
	}
	for {
		n, remoteAddr, err := conn.ReadFromUDP(s.recvBuf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.logWarn("KCP UDP 读取失败", "err", err)
			return
		}
		if n == 0 || remoteAddr == nil {
			continue
		}
		proto, localConn, remoteConn, payload, ok := decodeFrame(s.recvBuf[:n])
		if !ok {
			continue
		}
		s.handleFrame(proto, localConn, remoteConn, remoteAddr, payload)
	}
}

func (s *KService) updateChannels(now uint32) {
	s.mu.RLock()
	channels := make([]*KChannel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, channel)
	}
	s.mu.RUnlock()

	for _, channel := range channels {
		if channel != nil {
			if err := channel.Update(now); err != nil {
				channel.close(true)
			}
		}
	}
}

func (s *KService) handleFrame(proto byte, localConn, remoteConn uint32, remoteAddr *net.UDPAddr, payload []byte) {
	switch proto {
	case ProtoSYN, ProtoRouterSYN:
		s.handleSyn(proto, localConn, remoteConn, remoteAddr)
	case ProtoACK, ProtoRouterACK:
		s.handleAck(proto, localConn, remoteConn, remoteAddr)
	case ProtoFIN:
		s.handleFin(localConn, remoteConn, payload)
	case ProtoMSG:
		s.handleMsg(localConn, payload, remoteAddr)
	case ProtoRouterReconnSYN:
		s.handleRouterReconnSYN(localConn, remoteConn, remoteAddr)
	case ProtoRouterReconnACK:
		s.handleRouterReconnACK(localConn, remoteConn)
	}
}

func (s *KService) handleSyn(proto byte, senderLocal, receiverLocal uint32, remoteAddr *net.UDPAddr) {
	if senderLocal == 0 || remoteAddr == nil {
		return
	}

	s.mu.Lock()
	key := acceptWaitKey{remoteAddr: remoteAddr.String(), localConn: senderLocal}
	channel := s.acceptWait[key]
	if channel == nil {
		localConn := s.nextChannelIDLocked()
		channel = newAcceptedKChannel(localConn, senderLocal, remoteAddr, s.config, s)
		s.channels[localConn] = channel
		s.acceptWait[key] = channel
	}
	onAccept := s.onAccept
	s.mu.Unlock()

	channel.setRemoteAddr(remoteAddr)
	if channel.Conn() == nil {
		s.prepareConnection(channel)
		if onAccept != nil {
			onAccept(channel, channel.Conn())
		}
	}
	channel.markConnected(senderLocal)
	ackProto := ProtoACK
	if proto == ProtoRouterSYN {
		ackProto = ProtoRouterACK
	}
	if err := s.sendControlPair(remoteAddr, ackProto, channel.ID(), senderLocal, nil); err != nil {
		channel.close(true)
		return
	}
	channel.markHandshakeSent(time.Now().UnixMilli())
	_ = receiverLocal // 保留协议字段，严格按 sender/receiver 校验由对端完成。
}

func (s *KService) handleAck(_ byte, senderLocal, receiverLocal uint32, remoteAddr *net.UDPAddr) {
	if senderLocal == 0 || receiverLocal == 0 {
		return
	}
	s.mu.RLock()
	channel := s.channels[receiverLocal]
	s.mu.RUnlock()
	if channel == nil {
		return
	}
	localConn, remoteConn, _, _ := channel.handshakeState()
	if localConn != receiverLocal || (remoteConn != 0 && remoteConn != senderLocal) {
		return
	}
	channel.setRemoteAddr(remoteAddr)
	channel.markConnected(senderLocal)
	s.mu.Lock()
	delete(s.waitAccept, receiverLocal)
	s.mu.Unlock()
}

func (s *KService) handleFin(senderLocal, receiverLocal uint32, payload []byte) {
	if len(payload) < 4 {
		return
	}
	s.mu.RLock()
	channel := s.channels[receiverLocal]
	s.mu.RUnlock()
	if channel == nil {
		return
	}
	remoteConn := channel.remoteConnection()
	if remoteConn != senderLocal {
		return
	}
	channel.close(true)
}

func (s *KService) handleMsg(senderLocal uint32, payload []byte, remoteAddr *net.UDPAddr) {
	if len(payload) < 4 {
		return
	}
	receiverLocal := binary.LittleEndian.Uint32(payload[:4])
	s.mu.RLock()
	channel := s.channels[receiverLocal]
	if !messageChannelMatches(channel, senderLocal, remoteAddr) {
		channel = nil
		for _, candidate := range s.channels {
			if messageChannelMatches(candidate, senderLocal, remoteAddr) &&
				candidate.remoteConnection() == receiverLocal {
				channel = candidate
				break
			}
		}
	}
	s.mu.RUnlock()
	if channel == nil {
		return
	}
	if channel.Status() == ChannelStatusConnecting {
		channel.markConnected(senderLocal)
		s.mu.Lock()
		delete(s.acceptWait, acceptWaitKey{remoteAddr: remoteAddr.String(), localConn: senderLocal})
		delete(s.waitAccept, receiverLocal)
		s.mu.Unlock()
	}
	channel.HandleRecv(payload)
}

func messageChannelMatches(channel *KChannel, senderLocal uint32, remoteAddr *net.UDPAddr) bool {
	if channel == nil || channel.remoteConnection() != senderLocal {
		return false
	}
	return sameUDPAddr(channel.RemoteAddr(), remoteAddr)
}

func (s *KService) handleRouterReconnSYN(senderLocal, receiverLocal uint32, remoteAddr *net.UDPAddr) {
	s.mu.RLock()
	channel := s.channels[receiverLocal]
	s.mu.RUnlock()
	if channel == nil || channel.remoteConnection() != senderLocal {
		return
	}
	channel.setRemoteAddr(remoteAddr)
	if err := s.sendControlPair(remoteAddr, ProtoRouterReconnACK, receiverLocal, senderLocal, nil); err != nil {
		channel.close(true)
	}
}

func (s *KService) handleRouterReconnACK(senderLocal, receiverLocal uint32) {
	s.mu.RLock()
	channel := s.channels[receiverLocal]
	s.mu.RUnlock()
	if channel != nil && channel.remoteConnection() == senderLocal {
		channel.markConnected(senderLocal)
	}
}

func (s *KService) prepareConnection(channel *KChannel) {
	if channel == nil || channel.Conn() != nil {
		return
	}
	conn := newChannelConn(channel)
	channel.attachConn(conn)
	channel.SetOnDisconnected(func(_ *KChannel) {
		conn.close()
	})
}

func (s *KService) sendControl(addr *net.UDPAddr, proto byte, localConn uint32, payload []byte) error {
	return s.sendControlPair(addr, proto, localConn, 0, payload)
}

func (s *KService) sendControlPair(addr *net.UDPAddr, proto byte, localConn, remoteConn uint32, payload []byte) error {
	if addr == nil {
		return ErrAddressRequired
	}
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return errors.New("kcp: service is not listening")
	}
	var frame []byte
	if proto == ProtoFIN {
		frame = encodeFINFrame(localConn, remoteConn, payload)
	} else {
		frame = encodeControlFrame(proto, localConn, remoteConn, payload)
	}
	_, err := conn.WriteToUDP(frame, addr)
	return err
}

func (s *KService) sendMessage(addr *net.UDPAddr, localConn uint32, payload []byte) error {
	if addr == nil {
		return ErrAddressRequired
	}
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return errors.New("kcp: service is not listening")
	}
	_, err := conn.WriteToUDP(encodeMessageFrame(localConn, payload), addr)
	return err
}

func (s *KService) removeChannel(localConn uint32) {
	s.mu.Lock()
	delete(s.channels, localConn)
	delete(s.waitAccept, localConn)
	for remoteConn, channel := range s.acceptWait {
		if channel == nil || channel.ID() == localConn {
			delete(s.acceptWait, remoteConn)
		}
	}
	s.mu.Unlock()
}

func (s *KService) removeWaitAccept(localConn uint32) {
	s.mu.Lock()
	delete(s.waitAccept, localConn)
	for remoteConn, channel := range s.acceptWait {
		if channel == nil || channel.ID() == localConn {
			delete(s.acceptWait, remoteConn)
		}
	}
	s.mu.Unlock()
}

func (s *KService) nextChannelID() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextChannelIDLocked()
}

func (s *KService) nextChannelIDLocked() uint32 {
	for {
		s.nextConv++
		if s.nextConv == 0 {
			continue
		}
		if _, exists := s.channels[s.nextConv]; !exists {
			return s.nextConv
		}
	}
}

func encodeControlFrame(proto byte, localConn, remoteConn uint32, payload []byte) []byte {
	buf := make([]byte, controlFrameSize+len(payload))
	buf[0] = proto
	binary.LittleEndian.PutUint32(buf[1:5], localConn)
	binary.LittleEndian.PutUint32(buf[5:9], remoteConn)
	copy(buf[controlFrameSize:], payload)
	return buf
}

func encodeMessageFrame(localConn uint32, payload []byte) []byte {
	buf := make([]byte, messageFrameSize+len(payload))
	buf[0] = ProtoMSG
	binary.LittleEndian.PutUint32(buf[1:5], localConn)
	copy(buf[messageFrameSize:], payload)
	return buf
}

func encodeFINFrame(localConn, remoteConn uint32, payload []byte) []byte {
	buf := make([]byte, finFrameSize)
	buf[0] = ProtoFIN
	binary.LittleEndian.PutUint32(buf[1:5], localConn)
	binary.LittleEndian.PutUint32(buf[5:9], remoteConn)
	copy(buf[9:], payload)
	return buf
}

func decodeFrame(data []byte) (proto byte, localConn, remoteConn uint32, payload []byte, ok bool) {
	if len(data) < 1 {
		return 0, 0, 0, nil, false
	}
	proto = data[0]
	switch proto {
	case ProtoMSG:
		if len(data) < messageFrameSize {
			return 0, 0, 0, nil, false
		}
		localConn = binary.LittleEndian.Uint32(data[1:5])
		return proto, localConn, 0, append([]byte(nil), data[messageFrameSize:]...), true
	case ProtoFIN:
		if len(data) < finFrameSize {
			return 0, 0, 0, nil, false
		}
		localConn = binary.LittleEndian.Uint32(data[1:5])
		remoteConn = binary.LittleEndian.Uint32(data[5:9])
		return proto, localConn, remoteConn, append([]byte(nil), data[9:]...), true
	default:
		if len(data) < controlFrameSize {
			return 0, 0, 0, nil, false
		}
		localConn = binary.LittleEndian.Uint32(data[1:5])
		remoteConn = binary.LittleEndian.Uint32(data[5:9])
		return proto, localConn, remoteConn, append([]byte(nil), data[controlFrameSize:]...), true
	}
}

func sameUDPAddr(left, right *net.UDPAddr) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.IP.Equal(right.IP) && left.Port == right.Port && left.Zone == right.Zone
}

func (s *KService) logWarn(message string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Warn(message, args...)
		return
	}
	slog.Warn(message, args...)
}
