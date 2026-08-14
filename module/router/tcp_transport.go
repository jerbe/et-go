package router

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

const (
	tcpOuterFrameLengthSize = 2
	tcpOuterMaxFrameSize    = int(^uint16(0))
	tcpOuterPacketCapacity  = 4096
)

type tcpRouterPacket struct {
	addr  *net.UDPAddr
	frame []byte
}

type tcpRouterConn struct {
	conn *net.TCPConn
	addr *net.UDPAddr
	mu   sync.Mutex
}

// tcpTransport implements the target Router outer TService framing:
// little-endian uint16 body length followed by one Router frame.
type tcpTransport struct {
	listener *net.TCPListener

	mu      sync.Mutex
	conns   map[string]*tcpRouterConn
	packets []tcpRouterPacket
	closed  bool
}

func newTCPTransport(addr string) (*tcpTransport, error) {
	if addr == "" {
		return nil, ErrRouterAddressRequired
	}
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return nil, err
	}
	transport := &tcpTransport{
		listener: listener,
		conns:    make(map[string]*tcpRouterConn),
	}
	go transport.acceptLoop()
	return transport, nil
}

func (t *tcpTransport) acceptLoop() {
	t.mu.Lock()
	listener := t.listener
	t.mu.Unlock()
	if listener == nil {
		return
	}
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			t.mu.Lock()
			closed := t.closed
			t.mu.Unlock()
			if closed {
				return
			}
			slog.Warn("router TCP accept failed", "err", err)
			continue
		}
		_ = conn.SetNoDelay(true)
		addr := tcpAddrToUDP(conn.RemoteAddr())
		if addr == nil {
			_ = conn.Close()
			continue
		}
		wrapped := &tcpRouterConn{conn: conn, addr: addr}
		key := routerAddrKey(addr)
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			_ = conn.Close()
			return
		}
		old := t.conns[key]
		t.conns[key] = wrapped
		t.mu.Unlock()
		if old != nil {
			_ = old.conn.Close()
		}
		go t.readLoop(wrapped, key)
	}
}

func (t *tcpTransport) readLoop(routerConn *tcpRouterConn, key string) {
	defer t.removeConn(key, routerConn)
	header := make([]byte, tcpOuterFrameLengthSize)
	for {
		if _, err := io.ReadFull(routerConn.conn, header); err != nil {
			return
		}
		length := int(binary.LittleEndian.Uint16(header))
		if length < routerFrameHeaderSize || length > tcpOuterMaxFrameSize {
			return
		}
		frame := make([]byte, length)
		if _, err := io.ReadFull(routerConn.conn, frame); err != nil {
			return
		}
		if err := t.enqueue(tcpRouterPacket{
			addr:  cloneUDPAddr(routerConn.addr),
			frame: frame,
		}); err != nil {
			slog.Warn("router TCP packet queue closed", "remote", routerConn.addr, "err", err)
			return
		}
	}
}

func (t *tcpTransport) enqueue(packet tcpRouterPacket) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return ErrRouterTransportClosed
	}
	if len(t.packets) >= tcpOuterPacketCapacity {
		return errors.New("router: TCP packet queue full")
	}
	packet.addr = cloneUDPAddr(packet.addr)
	packet.frame = append([]byte(nil), packet.frame...)
	t.packets = append(t.packets, packet)
	return nil
}

func (t *tcpTransport) removeConn(key string, routerConn *tcpRouterConn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if current := t.conns[key]; current == routerConn {
		delete(t.conns, key)
	}
}

func (t *tcpTransport) Send(to *net.UDPAddr, protocol KcpProtocol, outerConn uint32, innerConn uint32, connectID uint32, payload []byte) error {
	return t.SendFrame(to, routerFrameToOuter, protocol, outerConn, innerConn, connectID, payload)
}

func (t *tcpTransport) SendFrame(
	to *net.UDPAddr,
	destination routerFrameDestination,
	protocol KcpProtocol,
	outerConn uint32,
	innerConn uint32,
	connectID uint32,
	payload []byte,
) error {
	if t == nil {
		return ErrRouterTransportClosed
	}
	if to == nil {
		return ErrRouterDestinationMissing
	}
	frame := encodeRouterFrameForDestination(destination, protocol, outerConn, innerConn, connectID, payload)
	if len(frame) > tcpOuterMaxFrameSize {
		return fmt.Errorf("router: TCP frame too large: %d", len(frame))
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrRouterTransportClosed
	}
	routerConn := t.conns[routerAddrKey(to)]
	t.mu.Unlock()
	if routerConn == nil || routerConn.conn == nil {
		return ErrRouterDestinationMissing
	}

	packet := make([]byte, tcpOuterFrameLengthSize+len(frame))
	binary.LittleEndian.PutUint16(packet[:tcpOuterFrameLengthSize], uint16(len(frame)))
	copy(packet[tcpOuterFrameLengthSize:], frame)
	routerConn.mu.Lock()
	defer routerConn.mu.Unlock()
	if _, err := writeFull(routerConn.conn, packet); err != nil {
		return err
	}
	return nil
}

func (t *tcpTransport) Poll(system *RouterComponentSystem) {
	if t == nil || system == nil {
		return
	}
	t.mu.Lock()
	packets := t.packets
	t.packets = nil
	t.mu.Unlock()
	for _, packet := range packets {
		protocol, outerConn, innerConn, connectID, payload, err :=
			decodeRouterFrameFromSource(packet.frame, routerFrameFromOuter)
		if err != nil {
			continue
		}
		handleOuterRouterFrame(system, t, protocol, outerConn, innerConn, connectID, payload, packet.addr)
	}
}

func (t *tcpTransport) PollOuter(system *RouterComponentSystem) {
	t.Poll(system)
}

func (t *tcpTransport) PollInner(_ *RouterComponentSystem) {}

func (t *tcpTransport) Close() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	listener := t.listener
	t.listener = nil
	conns := make([]*tcpRouterConn, 0, len(t.conns))
	for key, routerConn := range t.conns {
		delete(t.conns, key)
		conns = append(conns, routerConn)
	}
	t.packets = nil
	t.mu.Unlock()

	var closeErr error
	if listener != nil {
		closeErr = listener.Close()
	}
	for _, routerConn := range conns {
		if routerConn == nil || routerConn.conn == nil {
			continue
		}
		if err := routerConn.conn.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

func tcpAddrToUDP(addr net.Addr) *net.UDPAddr {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok || tcpAddr == nil {
		return nil
	}
	return &net.UDPAddr{
		IP:   append(net.IP(nil), tcpAddr.IP...),
		Port: tcpAddr.Port,
		Zone: tcpAddr.Zone,
	}
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	return &net.UDPAddr{
		IP:   append(net.IP(nil), addr.IP...),
		Port: addr.Port,
		Zone: addr.Zone,
	}
}

func routerAddrKey(addr *net.UDPAddr) string {
	if addr == nil {
		return ""
	}
	return net.JoinHostPort(addr.IP.String(), fmt.Sprintf("%d", addr.Port))
}

func writeFull(conn net.Conn, data []byte) (int, error) {
	total := 0
	for total < len(data) {
		n, err := conn.Write(data[total:])
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
