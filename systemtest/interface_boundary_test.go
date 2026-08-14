//go:build system

package systemtest

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/statesync"
	etproto "github.com/jerbe/et-go/proto"
	inventorypb "github.com/jerbe/et-go/proto/inventorypb"
	mapouterpb "github.com/jerbe/et-go/proto/mapouterpb"
)

// TestSystemHTTPAndProtocolBoundaries supplements the happy-path full-stack
// test with externally observable protocol and input-boundary checks.
func TestSystemHTTPAndProtocolBoundaries(t *testing.T) {
	stack := startFullStack(t)
	username := fmt.Sprintf("boundary_%d", time.Now().UnixNano())
	password := "BoundaryPass-2026!"
	accessToken := testHTTPInterfaces(t, stack, username, password)

	testHTTPBoundaryRequests(t, stack, accessToken, username, password)

	realm := dialKCP(t, stack.realmAddress)
	realmLogin := callProto[*etproto.R2C_Login](
		t,
		realm,
		2001,
		2002,
		&etproto.C2R_Login{AccessToken: accessToken, ZoneId: 1},
	)
	realm.Close()
	if realmLogin.Error != 0 || realmLogin.Address == "" || realmLogin.Token == "" {
		t.Fatalf("boundary realm login response = %#v", realmLogin)
	}

	gate := dialKCP(t, realmLogin.Address)
	t.Cleanup(gate.Close)
	gateLogin := callProto[*etproto.G2C_LoginGate](
		t,
		gate,
		2105,
		2106,
		&etproto.C2G_LoginGate{
			Token:  realmLogin.Token,
			GateId: realmLogin.GateId,
		},
	)
	if gateLogin.Error != 0 || gateLogin.PlayerId <= 0 {
		t.Fatalf("boundary gate login response = %#v", gateLogin)
	}

	testGateTokenReplay(t, realmLogin)
	testMapAndInventoryBoundaryRequests(t, gate, gateLogin.PlayerId)
	testRouterFullUDPFlow(t, stack.routerNode)
}

func testHTTPBoundaryRequests(t *testing.T, stack *fullStack, accessToken, username, password string) {
	t.Helper()

	assertHTTPEnvelope := func(endpoint, method, body string, wantError int, headers map[string]string) httpEnvelope {
		t.Helper()
		status, responseHeaders, payload := requestRaw(t, endpoint, method, body, headers)
		if status != http.StatusOK {
			t.Fatalf("%s %s status=%d, want %d; body=%s", method, endpoint, status, http.StatusOK, payload)
		}
		var response httpEnvelope
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("%s %s response is not JSON: %v; body=%s", method, endpoint, err, payload)
		}
		if wantError >= 0 && response.Error != wantError {
			t.Fatalf("%s %s Error=%d, want %d; response=%s", method, endpoint, response.Error, wantError, payload)
		}
		if wantError < 0 && response.Error == 0 {
			t.Fatalf("%s %s unexpectedly succeeded; response=%s", method, endpoint, payload)
		}
		if responseHeaders.Get("Content-Type") != "application/json; charset=utf-8" {
			t.Fatalf("%s %s Content-Type=%q", method, endpoint, responseHeaders.Get("Content-Type"))
		}
		return response
	}

	assertHTTPEnvelope(stack.httpURL+"/", http.MethodGet, "", http.StatusNotFound, nil)
	assertHTTPEnvelope(stack.httpURL+"/unknown", http.MethodGet, "", http.StatusNotFound, nil)
	assertHTTPEnvelope(stack.httpURL+"/register", http.MethodGet, "", http.StatusNotFound, nil)
	assertHTTPEnvelope(stack.httpURL+"/login", http.MethodGet, "", http.StatusNotFound, nil)
	assertHTTPEnvelope(stack.httpURL+"/get_area_list", http.MethodPost, `{}`, http.StatusNotFound, map[string]string{
		"Content-Type": "application/json",
	})
	assertHTTPEnvelope(stack.httpURL+"/register", http.MethodPost, `{"Username":`, http.StatusInternalServerError, map[string]string{
		"Content-Type": "application/json",
	})
	assertHTTPEnvelope(stack.httpURL+"/login", http.MethodPost, `{"Username":`, http.StatusInternalServerError, map[string]string{
		"Content-Type": "application/json",
	})

	allowedHeaders := map[string]string{"Origin": "http://127.0.0.1:3000"}
	_, responseHeaders, _ := requestRaw(
		t,
		stack.httpURL+"/get_area_list",
		http.MethodGet,
		"",
		allowedHeaders,
	)
	if got := responseHeaders.Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:3000" {
		t.Fatalf("allowed CORS header = %q", got)
	}
	_, responseHeaders, _ = requestRaw(
		t,
		stack.httpURL+"/get_area_list",
		http.MethodGet,
		"",
		map[string]string{"Origin": "http://evil.invalid"},
	)
	if got := responseHeaders.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured CORS header = %q", got)
	}
	_, responseHeaders, _ = requestRaw(
		t,
		stack.httpURL+"/login",
		http.MethodOptions,
		"",
		allowedHeaders,
	)
	if got := responseHeaders.Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Fatalf("CORS preflight methods = %q", got)
	}

	routerEnvelope := func(endpoint, method, body string, wantError int) httpEnvelope {
		t.Helper()
		return assertHTTPEnvelope(endpoint, method, body, wantError, map[string]string{
			"Content-Type": "application/json",
		})
	}
	routerEnvelope(stack.routerURL+"/unknown", http.MethodGet, "", http.StatusNotFound)
	routerEnvelope(stack.routerURL+"/router/list", http.MethodPost, `{}`, http.StatusNotFound)
	routerEnvelope(stack.routerURL+"/zone/list", http.MethodPost, `{}`, http.StatusNotFound)
	routerEnvelope(stack.routerURL+"/zone/last", http.MethodPost, `{}`, http.StatusNotFound)
	routerEnvelope(stack.routerURL+"/login", http.MethodPost, `{"Username":`, http.StatusInternalServerError)

	badRouterLogin := routerEnvelope(
		stack.routerURL+"/login",
		http.MethodPost,
		fmt.Sprintf(`{"Username":%q,"Password":"wrong"}`, username),
		-1,
	)
	if badRouterLogin.Error == 0 {
		t.Fatalf("router bad login unexpectedly succeeded: %#v", badRouterLogin)
	}

	validLastZone := routerEnvelope(
		stack.routerURL+"/zone/last?access_token="+url.QueryEscape(accessToken),
		http.MethodGet,
		"",
		0,
	)
	if validLastZone.Error != 0 {
		t.Fatalf("router valid last-zone response = %#v", validLastZone)
	}

}

func testGateTokenReplay(t *testing.T, loginResponse *etproto.R2C_Login) {
	t.Helper()
	replay := dialKCP(t, loginResponse.Address)
	t.Cleanup(replay.Close)
	sendProtoRequest(t, replay, 2105, &etproto.C2G_LoginGate{
		Token:  loginResponse.Token,
		GateId: loginResponse.GateId,
	})
	select {
	case <-replay.session.Context().Done():
	case <-time.After(3 * time.Second):
		t.Fatal("replayed Gate token did not close the session")
	}
}

func testMapAndInventoryBoundaryRequests(t *testing.T, client *kcpClient, playerID int64) {
	t.Helper()

	badBag := callProto[*inventorypb.M2C_GetBagInfo](
		t,
		client,
		inventory.MsgC2MGetBagInfo,
		inventory.MsgM2CGetBagInfo,
		&inventorypb.C2M_GetBagInfo{UnitId: playerID + 1000000},
	)
	if badBag.Error == 0 {
		t.Fatalf("unknown bag unit unexpectedly succeeded: %#v", badBag)
	}

	badWarehouse := callProto[*inventorypb.M2C_GetWarehouseInfo](
		t,
		client,
		inventory.MsgC2MGetWarehouseInfo,
		inventory.MsgM2CGetWarehouseInfo,
		&inventorypb.C2M_GetWarehouseInfo{UnitId: playerID + 1000000},
	)
	if badWarehouse.Error == 0 {
		t.Fatalf("unknown warehouse unit unexpectedly succeeded: %#v", badWarehouse)
	}

	enter := callProto[*mapouterpb.M2C_EnterMap](
		t,
		client,
		statesync.MsgC2MEnterMap,
		statesync.MsgM2CEnterMap,
		&mapouterpb.C2M_EnterMap{},
	)
	if enter.Error != 0 {
		t.Fatalf("boundary enter map response = %#v", enter)
	}
	_ = waitMessage(t, client, statesync.MsgCreateMyUnit)

	transfer := callProto[*mapouterpb.M2C_TransferMap](
		t,
		client,
		statesync.MsgC2MTransferMap,
		statesync.MsgM2CTransferMap,
		&mapouterpb.C2M_TransferMap{},
	)
	if transfer.Error != 0 {
		t.Fatalf("boundary transfer response = %#v", transfer)
	}
	_ = waitMessage(t, client, statesync.MsgStartSceneChange)
	_ = waitMessage(t, client, statesync.MsgCreateMyUnit)

	// Map2 has no configured target. This verifies that a validly routed
	// request returns an explicit business failure instead of a false success.
	noTarget := callProto[*mapouterpb.M2C_TransferMap](
		t,
		client,
		statesync.MsgC2MTransferMap,
		statesync.MsgM2CTransferMap,
		&mapouterpb.C2M_TransferMap{},
	)
	if noTarget.Error == 0 {
		t.Fatalf("transfer from target-less map unexpectedly succeeded: %#v", noTarget)
	}
}

func testRouterFullUDPFlow(t *testing.T, address string) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split router address %q: %v", address, err)
	}
	port := 0
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse router port %q: %v", portText, err)
	}
	routerAddr := &net.UDPAddr{IP: net.ParseIP(host), Port: port}

	outer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen router outer test socket: %v", err)
	}
	defer outer.Close()
	inner, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen router inner test socket: %v", err)
	}
	defer inner.Close()

	const (
		outerConn = uint32(701)
		innerConn = uint32(1701)
		connectID = uint32(2701)
	)
	if _, err := outer.WriteToUDP(
		routerFrame(7, outerConn, 0, connectID, []byte(inner.LocalAddr().String())),
		routerAddr,
	); err != nil {
		t.Fatalf("write RouterSYN: %v", err)
	}
	ack := readUDPFrame(t, outer)
	if len(ack) < 9 || ack[0] != 8 ||
		binary.LittleEndian.Uint32(ack[1:5]) != 0 ||
		binary.LittleEndian.Uint32(ack[5:9]) != outerConn {
		t.Fatalf("RouterACK frame = %x", ack)
	}

	if _, err := outer.WriteToUDP(routerFrame(1, outerConn, 0, 0, nil), routerAddr); err != nil {
		t.Fatalf("write outer SYN: %v", err)
	}
	forwarded, routerInnerAddr, err := readUDPFrameFrom(t, inner)
	if err != nil {
		t.Fatalf("read forwarded SYN: %v", err)
	}
	if len(forwarded) < 9 || forwarded[0] != 1 ||
		binary.LittleEndian.Uint32(forwarded[1:5]) != outerConn {
		t.Fatalf("forwarded SYN frame = %x", forwarded)
	}
	if _, err := inner.WriteToUDP(routerFrame(2, innerConn, outerConn, 0, nil), routerInnerAddr); err != nil {
		t.Fatalf("write inner ACK: %v", err)
	}
	outerAck := readUDPFrame(t, outer)
	if len(outerAck) < 9 || outerAck[0] != 2 ||
		binary.LittleEndian.Uint32(outerAck[1:5]) != innerConn ||
		binary.LittleEndian.Uint32(outerAck[5:9]) != outerConn {
		t.Fatalf("outer ACK frame = %x", outerAck)
	}

	if _, err := outer.WriteToUDP(routerFrame(4, outerConn, innerConn, 0, []byte("outer-message")), routerAddr); err != nil {
		t.Fatalf("write outer MSG: %v", err)
	}
	innerMessage, _, err := readUDPFrameFrom(t, inner)
	if err != nil {
		t.Fatalf("read inner MSG: %v", err)
	}
	if len(innerMessage) < 9 || innerMessage[0] != 4 ||
		binary.LittleEndian.Uint32(innerMessage[1:5]) != outerConn ||
		string(innerMessage[9:]) != "outer-message" {
		t.Fatalf("inner MSG frame = %x", innerMessage)
	}

	if _, err := inner.WriteToUDP(routerFrame(4, innerConn, outerConn, 0, []byte("inner-message")), routerInnerAddr); err != nil {
		t.Fatalf("write inner MSG: %v", err)
	}
	outerMessage := readUDPFrame(t, outer)
	if len(outerMessage) < 9 || outerMessage[0] != 4 ||
		binary.LittleEndian.Uint32(outerMessage[1:5]) != innerConn ||
		string(outerMessage[9:]) != "inner-message" {
		t.Fatalf("outer MSG frame = %x", outerMessage)
	}

	if _, err := outer.WriteToUDP(routerFrame(5, outerConn, innerConn, connectID, nil), routerAddr); err != nil {
		t.Fatalf("write RouterReconnectSYN: %v", err)
	}
	reconnect := readUDPFrame(t, inner)
	if len(reconnect) < 9 || reconnect[0] != 5 {
		t.Fatalf("inner RouterReconnectSYN frame = %x", reconnect)
	}
	if _, err := inner.WriteToUDP(routerFrame(6, innerConn, outerConn, 0, nil), routerInnerAddr); err != nil {
		t.Fatalf("write RouterReconnectACK: %v", err)
	}
	reconnectAck := readUDPFrame(t, outer)
	if len(reconnectAck) < 9 || reconnectAck[0] != 6 ||
		binary.LittleEndian.Uint32(reconnectAck[1:5]) != innerConn ||
		binary.LittleEndian.Uint32(reconnectAck[5:9]) != outerConn {
		t.Fatalf("outer RouterReconnectACK frame = %x", reconnectAck)
	}

	if _, err := outer.WriteToUDP(routerFrame(3, outerConn, innerConn, 9, nil), routerAddr); err != nil {
		t.Fatalf("write outer FIN: %v", err)
	}
	innerFIN := readUDPFrame(t, inner)
	if len(innerFIN) != 13 || innerFIN[0] != 3 ||
		binary.LittleEndian.Uint32(innerFIN[9:13]) != 9 {
		t.Fatalf("inner FIN frame = %x", innerFIN)
	}
}

func requestRaw(t *testing.T, endpoint, method, body string, headers map[string]string) (int, http.Header, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatalf("create HTTP request %s %s: %v", method, endpoint, err)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("HTTP request %s %s: %v", method, endpoint, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read HTTP response %s %s: %v", method, endpoint, err)
	}
	return response.StatusCode, response.Header, payload
}

func routerFrame(protocol byte, first, second, control uint32, payload []byte) []byte {
	length := 9 + len(payload)
	switch protocol {
	case 3:
		length = 13
	case 5:
		length = 13 + len(payload)
	case 7:
		length = 13 + len(payload)
	}
	frame := make([]byte, length)
	frame[0] = protocol
	binary.LittleEndian.PutUint32(frame[1:5], first)
	binary.LittleEndian.PutUint32(frame[5:9], second)
	switch protocol {
	case 3, 5, 7:
		binary.LittleEndian.PutUint32(frame[9:13], control)
		copy(frame[13:], payload)
	default:
		copy(frame[9:], payload)
	}
	return frame
}

func readUDPFrame(t *testing.T, conn *net.UDPConn) []byte {
	t.Helper()
	frame, _, _ := readUDPFrameFrom(t, conn)
	return frame
}

func readUDPFrameFrom(t *testing.T, conn *net.UDPConn) ([]byte, *net.UDPAddr, error) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set UDP frame deadline: %v", err)
	}
	buffer := make([]byte, 64*1024)
	n, address, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return nil, nil, err
	}
	return buffer[:n], address, nil
}
