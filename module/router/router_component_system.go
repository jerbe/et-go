package router

import (
	"net"
	"time"
)

// RouterComponentSystem handles router events.
type RouterComponentSystem struct {
	component *RouterComponent
}

// NewRouterComponentSystem returns system.
func NewRouterComponentSystem(c *RouterComponent) *RouterComponentSystem {
	return &RouterComponentSystem{component: c}
}

// Update implements ecs.UpdateSystem.
func (s *RouterComponentSystem) Update() {
	if s == nil || s.component == nil {
		return
	}

	outerTransport := s.component.Transport()
	innerTransport := s.component.InnerTransport()
	if outerTransport != nil {
		if poller, ok := outerTransport.(directionalPollingTransport); ok {
			if outerTransport == innerTransport {
				poller.Poll(s)
			} else {
				poller.PollOuter(s)
			}
		} else if poller, ok := outerTransport.(pollingTransport); ok {
			poller.Poll(s)
		}
	}
	tcpTransport := s.component.TCPTransport()
	if tcpTransport != nil && tcpTransport != outerTransport {
		if poller, ok := tcpTransport.(pollingTransport); ok {
			poller.Poll(s)
		}
	}
	if innerTransport != nil && innerTransport != outerTransport {
		if poller, ok := innerTransport.(directionalPollingTransport); ok {
			poller.PollInner(s)
		} else if poller, ok := innerTransport.(pollingTransport); ok {
			poller.Poll(s)
		}
	}

	now := s.component.Now()
	if !s.component.LastCheckTime.IsZero() && now.Sub(s.component.LastCheckTime) < time.Second {
		return
	}
	s.component.LastCheckTime = now
	nodes := s.component.Nodes()
	n := len(nodes)
	if n == 0 {
		return
	}
	for i := 0; i < n && i < 10; i++ {
		node := nodes[(s.component.checkCursor+i)%n]
		s.checkTimeout(node, now)
	}
	s.component.checkCursor = (s.component.checkCursor + 10) % n
}

// HandleRouterSYN creates a Sync route node for callers that do not carry the
// target RouterSYN connect ID. Runtime wire handling uses
// HandleRouterSYNWithConnect so the C# Router protocol's connect ID is kept.
func (s *RouterComponentSystem) HandleRouterSYN(outerConn uint32, innerAddress string, outerAddr *net.UDPAddr) (*RouterNode, error) {
	if s == nil || s.component == nil {
		return nil, ErrRouterDestinationMissing
	}
	return s.HandleRouterSYNWithTransport(
		s.component.Transport(),
		outerConn,
		innerAddress,
		s.component.AllocConnectID(),
		outerAddr,
	)
}

// HandleRouterSYNWithConnect handles the target RouterSYN frame.
//
// RouterSYN is an outer-side registration frame. The router responds with a
// nine-byte RouterACK to the outer peer; it is not forwarded to the inner
// socket.
func (s *RouterComponentSystem) HandleRouterSYNWithConnect(
	outerConn uint32,
	innerAddress string,
	connectID uint32,
	outerAddr *net.UDPAddr,
) (*RouterNode, error) {
	if s == nil || s.component == nil {
		return nil, ErrRouterDestinationMissing
	}
	return s.HandleRouterSYNWithTransport(s.component.Transport(), outerConn, innerAddress, connectID, outerAddr)
}

// HandleRouterSYNWithTransport handles RouterSYN received by a specific
// external transport. TCP and UDP peers must keep their response on the same
// accepted transport.
func (s *RouterComponentSystem) HandleRouterSYNWithTransport(
	transport PacketTransport,
	outerConn uint32,
	innerAddress string,
	connectID uint32,
	outerAddr *net.UDPAddr,
) (*RouterNode, error) {
	if s == nil || s.component == nil || outerConn == 0 || outerAddr == nil {
		return nil, ErrRouterDestinationMissing
	}
	if connectID == 0 {
		connectID = s.component.AllocConnectID()
	}
	innerAddr, err := net.ResolveUDPAddr("udp", innerAddress)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, ErrRouterTransportClosed
	}

	node, exists := s.component.GetNodeByOuter(outerConn)
	if exists && node != nil {
		if node.ConnectId != connectID {
			return nil, ErrRouterConnectIDMismatch
		}
		node.OuterAddr = outerAddr
		node.InnerAddr = innerAddr
		node.InnerAddress = innerAddress
		node.OuterTransport = transport
		node.LastRecvOuterTime = s.component.Now()
	} else {
		node = &RouterNode{
			OuterConnID:       outerConn,
			ConnectId:         connectID,
			InnerAddress:      innerAddress,
			OuterAddr:         outerAddr,
			InnerAddr:         innerAddr,
			OuterTransport:    transport,
			LastRecvOuterTime: s.component.Now(),
			LastRecvInnerTime: s.component.Now(),
			Status:            RouterStatusSync,
		}
		s.component.AddNode(node)
	}

	if err := s.sendToOuter(KcpRouterACK, node.OuterConn(), node.InnerConn, 0, nil); err != nil {
		if !exists {
			s.disposeNode(node)
		}
		return nil, err
	}
	return node, nil
}

// BindOuterTransport records the transport on which an existing outer peer
// was received, so its ACK/MSG/FIN responses return through that connection.
func (s *RouterComponentSystem) BindOuterTransport(outerConn uint32, transport PacketTransport) bool {
	if s == nil || s.component == nil || outerConn == 0 || transport == nil {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil {
		return false
	}
	node.OuterTransport = transport
	return true
}

// HandleRouterACK completes the Sync -> Msg transition after the inner
// service has accepted the ordinary KCP SYN. The second argument may be the
// target outer connection ID or the runtime connect ID for compatibility with
// direct callers.
func (s *RouterComponentSystem) HandleRouterACK(innerConn uint32, outerOrConnectID uint32) bool {
	if s == nil || s.component == nil || innerConn == 0 || outerOrConnectID == 0 {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerOrConnectID)
	if !ok {
		node, ok = s.component.GetNodeByConnect(outerOrConnectID)
	}
	if !ok || node == nil {
		return false
	}
	if node.OuterTransport == nil && s.component.Transport() == nil {
		return false
	}

	oldInnerConn := node.InnerConn
	oldStatus := node.Status
	oldLastInner := node.LastRecvInnerTime
	node.InnerConn = innerConn
	node.Status = RouterStatusMsg
	node.LastRecvInnerTime = s.component.Now()
	if err := s.sendToOuter(KcpACK, node.OuterConn(), node.InnerConn, 0, nil); err != nil {
		node.InnerConn = oldInnerConn
		node.Status = oldStatus
		node.LastRecvInnerTime = oldLastInner
		return false
	}
	return true
}

// HandleOuterSYN forwards the external KCP SYN to the inner service and adds
// the observed public address as the target protocol's real-address suffix.
func (s *RouterComponentSystem) HandleOuterSYN(outerConn, innerConn uint32, outerAddr *net.UDPAddr) bool {
	if s == nil || s.component == nil || outerConn == 0 || outerAddr == nil {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || !s.hasInnerTransport() {
		return false
	}
	now := s.component.Now()
	if !node.CheckOuterCount(now) {
		return false
	}
	if err := s.sendToInner(KcpSYN, outerConn, innerConn, 0, []byte(outerAddr.String())); err != nil {
		return false
	}
	node.OuterAddr = outerAddr
	node.LastRecvOuterTime = now
	return true
}

// HandleOuterMsg forwards an external message to the inner service.
func (s *RouterComponentSystem) HandleOuterMsg(outerConn uint32, data []byte, outerAddr *net.UDPAddr) bool {
	if s == nil || s.component == nil || outerAddr == nil || !s.hasInnerTransport() {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.Status != RouterStatusMsg || node.InnerConn == 0 {
		return false
	}
	now := s.component.Now()
	if !node.CheckOuterCount(now) {
		return false
	}
	node.OuterAddr = outerAddr
	if err := s.sendToInner(KcpMSG, node.OuterConn(), node.InnerConn, 0, data); err != nil {
		return false
	}
	node.LastRecvOuterTime = now
	return true
}

// HandleInnerMsg forwards an inner message to the external peer.
func (s *RouterComponentSystem) HandleInnerMsg(innerConn uint32, outerConn uint32, data []byte) bool {
	if s == nil || s.component == nil || innerConn == 0 || outerConn == 0 {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.Status != RouterStatusMsg || node.InnerConn != innerConn {
		return false
	}
	if err := s.sendToOuter(KcpMSG, node.OuterConn(), node.InnerConn, 0, data); err != nil {
		return false
	}
	node.LastRecvInnerTime = s.component.Now()
	return true
}

// HandleOuterFIN forwards an external disconnect frame to the inner service.
func (s *RouterComponentSystem) HandleOuterFIN(outerConn, innerConn, errorCode uint32) bool {
	if s == nil || s.component == nil || outerConn == 0 || innerConn == 0 || !s.hasInnerTransport() {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.InnerConn != innerConn {
		return false
	}
	if err := s.sendToInner(KcpFIN, outerConn, innerConn, errorCode, nil); err != nil {
		return false
	}
	node.LastRecvOuterTime = s.component.Now()
	return true
}

// HandleInnerFIN forwards an inner disconnect frame to the external peer.
func (s *RouterComponentSystem) HandleInnerFIN(innerConn, outerConn, errorCode uint32) bool {
	if s == nil || s.component == nil || innerConn == 0 || outerConn == 0 {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.InnerConn != innerConn {
		return false
	}
	if err := s.sendToOuter(KcpFIN, node.OuterConn(), node.InnerConn, errorCode, nil); err != nil {
		return false
	}
	node.LastRecvInnerTime = s.component.Now()
	return true
}

// HandleFIN destroys a route node. Wire FIN handling uses HandleOuterFIN or
// HandleInnerFIN because the target Router forwards FIN before timeout cleanup.
func (s *RouterComponentSystem) HandleFIN(outerConn uint32) {
	if s == nil || s.component == nil || outerConn == 0 {
		return
	}
	if node, ok := s.component.GetNodeByOuter(outerConn); ok {
		s.disposeNode(node)
	}
}

// HandleRouterReconnSYN updates the external address and starts a reconnect
// handshake using the node's existing inner connection and connect ID.
func (s *RouterComponentSystem) HandleRouterReconnSYN(outerConn uint32, outerAddr *net.UDPAddr) bool {
	if s == nil || s.component == nil {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil {
		return false
	}
	return s.HandleRouterReconnSYNWithState(
		outerConn,
		node.InnerConn,
		node.ConnectId,
		outerAddr,
	)
}

// HandleRouterReconnSYNWithState validates and forwards the target reconnect
// frame to the inner service.
func (s *RouterComponentSystem) HandleRouterReconnSYNWithState(
	outerConn, innerConn, connectID uint32,
	outerAddr *net.UDPAddr,
) bool {
	if s == nil || s.component == nil || outerConn == 0 || innerConn == 0 || connectID == 0 || outerAddr == nil || !s.hasInnerTransport() {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.InnerConn != innerConn || node.ConnectId != connectID {
		return false
	}
	oldAddr := node.OuterAddr
	oldStatus := node.Status
	oldSyncCount := node.RouterSyncCount
	oldLastOuter := node.LastRecvOuterTime
	node.OuterAddr = outerAddr
	node.Status = RouterStatusSync
	node.RouterSyncCount = 0
	node.SyncCount = 0
	node.LastRecvOuterTime = s.component.Now()
	if err := s.sendToInner(KcpRouterReconnSYN, node.OuterConn(), node.InnerConn, 0, nil); err != nil {
		node.OuterAddr = oldAddr
		node.Status = oldStatus
		node.RouterSyncCount = oldSyncCount
		node.LastRecvOuterTime = oldLastOuter
		return false
	}
	return true
}

// HandleRouterReconnACK forwards the inner reconnect confirmation.
func (s *RouterComponentSystem) HandleRouterReconnACK(outerConn uint32) bool {
	if s == nil || s.component == nil {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil {
		return false
	}
	return s.HandleRouterReconnACKWithState(node.InnerConn, outerConn)
}

// HandleRouterReconnACKWithState validates and forwards the target reconnect
// acknowledgement to the external peer.
func (s *RouterComponentSystem) HandleRouterReconnACKWithState(innerConn, outerConn uint32) bool {
	if s == nil || s.component == nil || innerConn == 0 || outerConn == 0 {
		return false
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.InnerConn != innerConn {
		return false
	}
	if node.OuterTransport == nil && s.component.Transport() == nil {
		return false
	}
	oldStatus := node.Status
	oldLastInner := node.LastRecvInnerTime
	node.Status = RouterStatusMsg
	node.LastRecvInnerTime = s.component.Now()
	if err := s.sendToOuter(KcpRouterReconnACK, node.OuterConn(), node.InnerConn, 0, nil); err != nil {
		node.Status = oldStatus
		node.LastRecvInnerTime = oldLastInner
		return false
	}
	return true
}

func (s *RouterComponentSystem) hasInnerTransport() bool {
	return s != nil && s.component != nil && s.component.InnerTransport() != nil
}

func (s *RouterComponentSystem) sendToInner(protocol KcpProtocol, outerConn, innerConn, connectID uint32, payload []byte) error {
	if s == nil || s.component == nil {
		return ErrRouterDestinationMissing
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.InnerAddr == nil {
		return ErrRouterDestinationMissing
	}
	return sendRouterPacket(s.component.InnerTransport(), routerFrameToInner, protocol, outerConn, innerConn, connectID, payload, func() *net.UDPAddr {
		return node.InnerAddr
	}())
}

func (s *RouterComponentSystem) sendToOuter(protocol KcpProtocol, outerConn, innerConn, connectID uint32, payload []byte) error {
	if s == nil || s.component == nil {
		return ErrRouterDestinationMissing
	}
	node, ok := s.component.GetNodeByOuter(outerConn)
	if !ok || node == nil || node.OuterAddr == nil {
		return ErrRouterDestinationMissing
	}
	transport := node.OuterTransport
	if transport == nil {
		transport = s.component.Transport()
	}
	return sendRouterPacket(transport, routerFrameToOuter, protocol, outerConn, innerConn, connectID, payload, node.OuterAddr)
}

func (s *RouterComponentSystem) checkTimeout(node *RouterNode, now time.Time) {
	if node == nil {
		return
	}
	switch node.Status {
	case RouterStatusSync:
		if now.Sub(node.LastRecvOuterTime) > 10*time.Second {
			s.disposeNode(node)
		}
	case RouterStatusMsg:
		if now.Sub(node.LastRecvOuterTime) > 50*time.Second && now.Sub(node.LastRecvInnerTime) > 50*time.Second {
			s.disposeNode(node)
		}
	}
}

func (s *RouterComponentSystem) disposeNode(node *RouterNode) {
	if s == nil || s.component == nil || node == nil {
		return
	}
	s.component.RemoveNode(node)
	node.OnDestroy()
	node.Dispose()
}
