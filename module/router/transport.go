package router

import (
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
)

const routerFrameHeaderSize = 1 + 4 + 4

type routerFrameDestination uint8

const (
	// routerFrameToInner means the source is the external side. The target
	// protocol writes outerConn before innerConn.
	routerFrameToInner routerFrameDestination = iota + 1
	// routerFrameToOuter means the source is the internal side. The target
	// protocol writes innerConn before outerConn.
	routerFrameToOuter
)

type routerFrameSource uint8

const (
	routerFrameFromOuter routerFrameSource = iota + 1
	routerFrameFromInner
	routerFrameSourceAuto
)

type directionalPollingTransport interface {
	Poll(system *RouterComponentSystem)
	PollOuter(system *RouterComponentSystem)
	PollInner(system *RouterComponentSystem)
}

type routerFrameSender interface {
	SendFrame(
		to *net.UDPAddr,
		destination routerFrameDestination,
		protocol KcpProtocol,
		outerConn uint32,
		innerConn uint32,
		connectID uint32,
		payload []byte,
	) error
}

var (
	ErrInvalidRouterFrame       = errors.New("router: invalid frame")
	ErrRouterAddressRequired    = errors.New("router: bind address required")
	ErrRouterConfigMissing      = errors.New("router: runtime config missing")
	ErrRouterTransportClosed    = errors.New("router: transport unavailable")
	ErrRouterDestinationMissing = errors.New("router: destination address missing")
	ErrRouterConnectIDMismatch  = errors.New("router: connect id mismatch")
)

type pollingTransport interface {
	Poll(system *RouterComponentSystem)
	Close() error
}

type udpTransport struct {
	conn *net.UDPConn
	buf  []byte
}

func newUDPTransport(addr string) (*udpTransport, error) {
	if addr == "" {
		return nil, ErrRouterAddressRequired
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	return &udpTransport{
		conn: conn,
		buf:  make([]byte, 64*1024),
	}, nil
}

// Send implements PacketTransport for direct callers. Runtime Router code
// uses SendFrame so the outer/inner direction is explicit.
func (t *udpTransport) Send(to *net.UDPAddr, protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) error {
	return t.SendFrame(to, routerFrameToOuter, protocol, outerConn, innerConn, connectID, payload)
}

// SendFrame writes the target Router wire format.
func (t *udpTransport) SendFrame(
	to *net.UDPAddr,
	destination routerFrameDestination,
	protocol KcpProtocol,
	outerConn uint32,
	innerConn uint32,
	connectID uint32,
	payload []byte,
) error {
	if t == nil || t.conn == nil {
		return ErrRouterTransportClosed
	}
	if to == nil {
		return ErrRouterDestinationMissing
	}
	frame := encodeRouterFrameForDestination(destination, protocol, outerConn, innerConn, connectID, payload)
	n, err := t.conn.WriteToUDP(frame, to)
	if err != nil {
		return err
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}

// Poll reads a socket whose side is not known by the caller. This is used by
// tests and compatibility callers that intentionally share one UDP socket for
// outer and inner traffic; production uses PollOuter/PollInner.
func (t *udpTransport) Poll(system *RouterComponentSystem) {
	t.poll(system, routerFrameSourceAuto)
}

func (t *udpTransport) PollOuter(system *RouterComponentSystem) {
	t.poll(system, routerFrameFromOuter)
}

func (t *udpTransport) PollInner(system *RouterComponentSystem) {
	t.poll(system, routerFrameFromInner)
}

func (t *udpTransport) poll(system *RouterComponentSystem, source routerFrameSource) {
	if t == nil || t.conn == nil || system == nil {
		return
	}
	if err := t.conn.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		slog.Warn("router UDP read deadline 设置失败", "err", err)
		return
	}
	for {
		n, addr, err := t.conn.ReadFromUDP(t.buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return
			}
			slog.Warn("router UDP 读取失败", "err", err)
			return
		}
		packetSource := source
		if packetSource == routerFrameSourceAuto {
			packetSource = inferRouterFrameSource(system, t.buf[:n], addr)
		}
		protocol, outerConn, innerConn, connectID, payload, err := decodeRouterFrameFromSource(t.buf[:n], packetSource)
		if err != nil {
			continue
		}
		if packetSource == routerFrameFromInner {
			handleInnerRouterFrame(system, protocol, outerConn, innerConn, connectID, payload)
			continue
		}
		handleOuterRouterFrame(system, t, protocol, outerConn, innerConn, connectID, payload, addr)
	}
}

func inferRouterFrameSource(system *RouterComponentSystem, frame []byte, addr *net.UDPAddr) routerFrameSource {
	if system == nil || system.component == nil || addr == nil || len(frame) < routerFrameHeaderSize {
		return routerFrameFromOuter
	}
	first := binary.LittleEndian.Uint32(frame[1:5])
	second := binary.LittleEndian.Uint32(frame[5:9])
	if node, ok := system.component.GetNodeByOuter(first); ok && sameUDPAddr(node.InnerAddr, addr) {
		return routerFrameFromInner
	}
	if node, ok := system.component.GetNodeByOuter(second); ok && sameUDPAddr(node.InnerAddr, addr) {
		return routerFrameFromInner
	}
	return routerFrameFromOuter
}

func sameUDPAddr(left, right *net.UDPAddr) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Port == right.Port && left.IP.Equal(right.IP)
}

func handleOuterRouterFrame(
	system *RouterComponentSystem,
	transport PacketTransport,
	protocol KcpProtocol,
	outerConn, innerConn, connectID uint32,
	payload []byte,
	outerAddr *net.UDPAddr,
) {
	switch protocol {
	case KcpRouterSYN:
		if _, err := system.HandleRouterSYNWithTransport(transport, outerConn, string(payload), connectID, outerAddr); err != nil {
			slog.Warn("router handle RouterSYN failed", "outer_conn", outerConn, "err", err)
		}
	case KcpRouterReconnSYN:
		system.BindOuterTransport(outerConn, transport)
		if !system.HandleRouterReconnSYNWithState(outerConn, innerConn, connectID, outerAddr) {
			slog.Debug("router ignored RouterReconnectSYN", "outer_conn", outerConn)
		}
	case KcpSYN:
		system.BindOuterTransport(outerConn, transport)
		if !system.HandleOuterSYN(outerConn, innerConn, outerAddr) {
			slog.Debug("router ignored SYN", "outer_conn", outerConn)
		}
	case KcpMSG:
		system.BindOuterTransport(outerConn, transport)
		if !system.HandleOuterMsg(outerConn, payload, outerAddr) {
			slog.Debug("router ignored outer message", "outer_conn", outerConn)
		}
	case KcpFIN:
		system.BindOuterTransport(outerConn, transport)
		if !system.HandleOuterFIN(outerConn, innerConn, connectID) {
			slog.Debug("router ignored outer FIN", "outer_conn", outerConn)
		}
	}
}

func handleInnerRouterFrame(
	system *RouterComponentSystem,
	protocol KcpProtocol,
	outerConn, innerConn, connectID uint32,
	payload []byte,
) {
	switch protocol {
	case KcpACK, KcpRouterACK:
		if !system.HandleRouterACK(innerConn, outerConn) {
			slog.Debug("router ignored inner ACK", "outer_conn", outerConn, "inner_conn", innerConn)
		}
	case KcpRouterReconnACK:
		if !system.HandleRouterReconnACKWithState(innerConn, outerConn) {
			slog.Debug("router ignored inner RouterReconnectACK", "outer_conn", outerConn)
		}
	case KcpMSG:
		if !system.HandleInnerMsg(innerConn, outerConn, payload) {
			slog.Debug("router ignored inner message", "outer_conn", outerConn, "inner_conn", innerConn)
		}
	case KcpFIN:
		if !system.HandleInnerFIN(innerConn, outerConn, connectID) {
			slog.Debug("router ignored inner FIN", "outer_conn", outerConn)
		}
	}
}

func encodeRouterFrame(protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) []byte {
	return encodeRouterFrameForDestination(routerFrameToInner, protocol, outerConn, innerConn, connectID, payload)
}

func encodeRouterFrameFromInner(protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) []byte {
	return encodeRouterFrameForDestination(routerFrameToOuter, protocol, outerConn, innerConn, connectID, payload)
}

func encodeRouterFrameForDestination(
	destination routerFrameDestination,
	protocol KcpProtocol,
	outerConn uint32,
	innerConn uint32,
	connectID uint32,
	payload []byte,
) []byte {
	first, second := outerConn, innerConn
	if destination == routerFrameToOuter || isRouterAck(protocol) {
		first, second = innerConn, outerConn
	}

	length := routerFrameHeaderSize
	switch protocol {
	case KcpRouterSYN:
		length += 4 + len(payload)
	case KcpRouterReconnSYN:
		if connectID != 0 || len(payload) > 0 {
			length += 4 + len(payload)
		}
	case KcpFIN:
		length += 4
	default:
		length += len(payload)
	}

	frame := make([]byte, length)
	frame[0] = byte(protocol)
	binary.LittleEndian.PutUint32(frame[1:5], first)
	binary.LittleEndian.PutUint32(frame[5:9], second)
	offset := routerFrameHeaderSize
	switch protocol {
	case KcpRouterSYN, KcpRouterReconnSYN:
		if length >= routerFrameHeaderSize+4 {
			binary.LittleEndian.PutUint32(frame[offset:offset+4], connectID)
			offset += 4
		}
	case KcpFIN:
		binary.LittleEndian.PutUint32(frame[offset:offset+4], connectID)
		offset += 4
	}
	copy(frame[offset:], payload)
	return frame
}

func decodeRouterFrame(frame []byte) (KcpProtocol, uint32, uint32, uint32, []byte, error) {
	return decodeRouterFrameFromSource(frame, routerFrameFromOuter)
}

func decodeRouterFrameFromSource(frame []byte, source routerFrameSource) (KcpProtocol, uint32, uint32, uint32, []byte, error) {
	if len(frame) < routerFrameHeaderSize {
		return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
	}
	protocol := KcpProtocol(frame[0])
	if !IsValidKcpProtocol(protocol) {
		return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
	}

	first := binary.LittleEndian.Uint32(frame[1:5])
	second := binary.LittleEndian.Uint32(frame[5:9])
	outerConn, innerConn := first, second
	if source == routerFrameFromInner || isRouterAck(protocol) {
		outerConn, innerConn = second, first
	}

	connectID := uint32(0)
	payloadOffset := routerFrameHeaderSize
	switch protocol {
	case KcpRouterSYN:
		if len(frame) < routerFrameHeaderSize+4 {
			return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
		}
		connectID = binary.LittleEndian.Uint32(frame[payloadOffset : payloadOffset+4])
		payloadOffset += 4
	case KcpRouterReconnSYN:
		if len(frame) >= routerFrameHeaderSize+4 {
			connectID = binary.LittleEndian.Uint32(frame[payloadOffset : payloadOffset+4])
			payloadOffset += 4
		} else if len(frame) != routerFrameHeaderSize {
			return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
		}
	case KcpFIN:
		if len(frame) != routerFrameHeaderSize+4 {
			return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
		}
		connectID = binary.LittleEndian.Uint32(frame[payloadOffset : payloadOffset+4])
		payloadOffset += 4
	default:
		if len(frame) < routerFrameHeaderSize {
			return 0, 0, 0, 0, nil, ErrInvalidRouterFrame
		}
	}
	return protocol, outerConn, innerConn, connectID, append([]byte(nil), frame[payloadOffset:]...), nil
}

func isRouterAck(protocol KcpProtocol) bool {
	switch protocol {
	case KcpACK, KcpRouterACK, KcpRouterReconnACK:
		return true
	default:
		return false
	}
}

func sendRouterPacket(
	transport PacketTransport,
	destination routerFrameDestination,
	protocol KcpProtocol,
	outerConn, innerConn, connectID uint32,
	payload []byte,
	to *net.UDPAddr,
) error {
	if transport == nil {
		return ErrRouterTransportClosed
	}
	if sender, ok := transport.(routerFrameSender); ok {
		return sender.SendFrame(to, destination, protocol, outerConn, innerConn, connectID, payload)
	}
	return transport.Send(to, protocol, outerConn, innerConn, connectID, payload)
}

func (t *udpTransport) Close() error {
	if t == nil || t.conn == nil {
		return nil
	}
	return t.conn.Close()
}

func (t *udpTransport) Addr() *net.UDPAddr {
	if t == nil || t.conn == nil {
		return nil
	}
	addr, _ := t.conn.LocalAddr().(*net.UDPAddr)
	return addr
}

func resolveRouterBindAddr(scene *ecs.Scene) (string, error) {
	addr, _, err := resolveRouterRuntimeConfig(scene)
	return addr, err
}

func resolveRouterRuntimeConfig(scene *ecs.Scene) (string, string, error) {
	cfg := config.GetGlobal()
	if cfg == nil || scene == nil {
		return "", "", ErrRouterConfigMissing
	}
	addr, err := network.ResolveSceneListenAddr(scene, false)
	if err != nil {
		return "", "", err
	}
	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.ID > 0 && int64(sceneCfg.ID) == scene.ID() {
			innerIP := ""
			if machine := machineForProcess(cfg, sceneCfg.ProcessID); machine != nil {
				innerIP = machine.InnerIP
			}
			if innerIP == "" {
				return "", "", ErrRouterConfigMissing
			}
			return addr, innerIP, nil
		}
		if sceneCfg.SceneType != scene.SceneType().String() && sceneCfg.SceneType != "RouterNode" {
			continue
		}
		if sceneCfg.Zone != scene.Zone() {
			continue
		}
		machine := machineForProcess(cfg, sceneCfg.ProcessID)
		if machine == nil || machine.InnerIP == "" {
			return "", "", ErrRouterConfigMissing
		}
		return addr, machine.InnerIP, nil
	}
	return "", "", ErrRouterConfigMissing
}
