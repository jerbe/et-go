//go:build system

package systemtest

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jerbe/et-go/config"
	etdb "github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/engine/network/kcp"
	"github.com/jerbe/et-go/module/inventory"
	"github.com/jerbe/et-go/module/lockstep"
	"github.com/jerbe/et-go/module/statesync"
	etproto "github.com/jerbe/et-go/proto"
	inventorypb "github.com/jerbe/et-go/proto/inventorypb"
	locksteppb "github.com/jerbe/et-go/proto/locksteppb"
	mapouterpb "github.com/jerbe/et-go/proto/mapouterpb"
	gproto "google.golang.org/protobuf/proto"
)

var sessionID atomic.Int64

type fullStack struct {
	cmd          *exec.Cmd
	logFile      *os.File
	configDir    string
	dbName       string
	httpURL      string
	routerURL    string
	realmAddress string
	gateAddress  string
	routerNode   string
}

type kcpClient struct {
	service    *kcp.KService
	channel    *kcp.KChannel
	session    *network.Session
	ctx        context.Context
	cancel     context.CancelFunc
	messages   chan *codec.Packet
	pending    []*codec.Packet
	updateDone chan struct{}
}

type httpEnvelope struct {
	RequestID   string `json:"RequestId"`
	Error       int    `json:"Error"`
	Message     string `json:"Message"`
	AccessToken string `json:"AccessToken"`
}

func TestSystemFullStackInterfaces(t *testing.T) {
	stack := startFullStack(t)

	username := fmt.Sprintf("system_%d", time.Now().UnixNano())
	password := "SystemPass-2026!"
	accessToken := testHTTPInterfaces(t, stack, username, password)

	realm := dialKCP(t, stack.realmAddress)
	loginResponse := callProto[*etproto.R2C_Login](
		t,
		realm,
		2001,
		2002,
		&etproto.C2R_Login{AccessToken: accessToken, ZoneId: 1},
	)
	if loginResponse.Error != 0 || loginResponse.GateId <= 0 || loginResponse.Token == "" {
		t.Fatalf("realm login response = %#v", loginResponse)
	}
	realm.Close()

	badRealm := dialKCP(t, stack.realmAddress)
	badRealmLogin := callProto[*etproto.R2C_Login](
		t,
		badRealm,
		2001,
		2002,
		&etproto.C2R_Login{AccessToken: "invalid-access-token", ZoneId: 1},
	)
	if badRealmLogin.Error == 0 || badRealmLogin.GateId != 0 || badRealmLogin.Token != "" {
		t.Fatalf("invalid realm login unexpectedly succeeded: %#v", badRealmLogin)
	}
	badRealm.Close()

	badGate := dialKCP(t, stack.gateAddress)
	sendProtoRequest(t, badGate, 2105, &etproto.C2G_LoginGate{
		Token:  "invalid-gate-token",
		GateId: loginResponse.GateId,
	})
	select {
	case <-badGate.session.Context().Done():
	case <-time.After(3 * time.Second):
		t.Fatal("invalid gate login did not close the session")
	}
	badGate.Close()

	gate := dialKCP(t, loginResponse.Address)
	gateLogin := callProto[*etproto.G2C_LoginGate](
		t,
		gate,
		2105,
		2106,
		&etproto.C2G_LoginGate{
			Token:  loginResponse.Token,
			GateId: loginResponse.GateId,
		},
	)
	if gateLogin.Error != 0 || gateLogin.PlayerId <= 0 || gateLogin.CharacterCount != 1 {
		t.Fatalf("gate login response = %#v", gateLogin)
	}

	ping := callProto[*etproto.G2C_Ping](t, gate, 2103, 2104, &etproto.C2G_Ping{})
	if ping.Error != 0 || ping.Time <= 0 {
		t.Fatalf("gate ping response = %#v", ping)
	}

	testMapAndInventoryInterfaces(t, gate, gateLogin.PlayerId)
	testLockstepInterfaces(t, stack, gate, gateLogin.PlayerId, password)
	testMapTransferInterface(t, gate, gateLogin.PlayerId)
	testRouterTransports(t, stack.routerNode)
}

func testHTTPInterfaces(t *testing.T, stack *fullStack, username, password string) string {
	t.Helper()

	var register httpEnvelope
	status, _, err := requestJSON(
		t,
		stack.httpURL+"/register",
		http.MethodPost,
		map[string]string{"Username": username, "Password": password},
		&register,
	)
	if err != nil {
		t.Fatalf("HTTP register error: %v", err)
	}
	if status != http.StatusOK || register.Error != 0 {
		t.Fatalf("HTTP register status=%d response=%#v", status, register)
	}

	var duplicate httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.httpURL+"/register",
		http.MethodPost,
		map[string]string{"Username": username, "Password": password},
		&duplicate,
	)
	if err != nil {
		t.Fatalf("HTTP duplicate register error: %v", err)
	}
	if duplicate.Error == 0 {
		t.Fatalf("duplicate registration unexpectedly succeeded: %#v", duplicate)
	}

	var invalidRegister httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.httpURL+"/register",
		http.MethodPost,
		map[string]string{"Username": "", "Password": ""},
		&invalidRegister,
	)
	if err != nil {
		t.Fatalf("HTTP invalid register error: %v", err)
	}
	if invalidRegister.Error == 0 {
		t.Fatalf("invalid registration unexpectedly succeeded: %#v", invalidRegister)
	}

	var area struct {
		httpEnvelope
		Areas []struct {
			ID        int32  `json:"Id"`
			Name      string `json:"Name"`
			ServerURL string `json:"ServerURL"`
		} `json:"Areas"`
	}
	status, _, err = requestJSON(t, stack.httpURL+"/get_area_list", http.MethodGet, nil, &area)
	if err != nil {
		t.Fatalf("HTTP area list error: %v", err)
	}
	if status != http.StatusOK || area.Error != 0 || len(area.Areas) != 1 || area.Areas[0].ID != 1 {
		t.Fatalf("HTTP area list status=%d response=%#v", status, area)
	}

	var loginResponse httpEnvelope
	status, _, err = requestJSON(
		t,
		stack.httpURL+"/login",
		http.MethodPost,
		map[string]string{"Username": username, "Password": password},
		&loginResponse,
	)
	if err != nil {
		t.Fatalf("HTTP login error: %v", err)
	}
	if status != http.StatusOK || loginResponse.Error != 0 || loginResponse.AccessToken == "" {
		t.Fatalf("HTTP login status=%d response=%#v", status, loginResponse)
	}

	var badLogin httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.httpURL+"/login",
		http.MethodPost,
		map[string]string{"Username": username, "Password": "wrong-password"},
		&badLogin,
	)
	if err != nil {
		t.Fatalf("HTTP bad login error: %v", err)
	}
	if badLogin.Error == 0 {
		t.Fatalf("bad login unexpectedly succeeded: %#v", badLogin)
	}

	var routerList struct {
		httpEnvelope
		ServerIP string   `json:"ServerIP"`
		Realms   []string `json:"Realms"`
		Routers  []string `json:"Routers"`
	}
	status, _, err = requestJSON(t, stack.routerURL+"/router/list", http.MethodGet, nil, &routerList)
	if err != nil {
		t.Fatalf("Router list error: %v", err)
	}
	if status != http.StatusOK || routerList.Error != 0 ||
		len(routerList.Realms) != 1 || len(routerList.Routers) != 1 {
		t.Fatalf("Router list status=%d response=%#v", status, routerList)
	}

	var zoneList struct {
		httpEnvelope
		Zones []struct {
			ID   int32  `json:"Id"`
			Name string `json:"Name"`
		} `json:"Zones"`
	}
	status, _, err = requestJSON(t, stack.routerURL+"/zone/list", http.MethodGet, nil, &zoneList)
	if err != nil {
		t.Fatalf("Router zone list error: %v", err)
	}
	if status != http.StatusOK || zoneList.Error != 0 || len(zoneList.Zones) != 1 {
		t.Fatalf("Router zone list status=%d response=%#v", status, zoneList)
	}

	var lastZone struct {
		httpEnvelope
		LastOne *struct {
			ID     int32 `json:"Id"`
			Status int32 `json:"Status"`
		} `json:"LastOne"`
	}
	status, _, err = requestJSON(
		t,
		stack.routerURL+"/zone/last?access_token="+url.QueryEscape(loginResponse.AccessToken),
		http.MethodGet,
		nil,
		&lastZone,
	)
	if err != nil {
		t.Fatalf("Router last zone error: %v", err)
	}
	if status != http.StatusOK || lastZone.Error != 0 || lastZone.LastOne == nil ||
		lastZone.LastOne.ID != 1 {
		t.Fatalf("Router last zone status=%d response=%#v", status, lastZone)
	}

	var missingTokenZone httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.routerURL+"/zone/last",
		http.MethodGet,
		nil,
		&missingTokenZone,
	)
	if err != nil {
		t.Fatalf("Router last zone missing-token error: %v", err)
	}
	if missingTokenZone.Error == 0 {
		t.Fatalf("Router last zone without token unexpectedly succeeded: %#v", missingTokenZone)
	}

	var invalidTokenZone httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.routerURL+"/zone/last?access_token=invalid-access-token",
		http.MethodGet,
		nil,
		&invalidTokenZone,
	)
	if err != nil {
		t.Fatalf("Router last zone invalid-token error: %v", err)
	}
	if invalidTokenZone.Error == 0 {
		t.Fatalf("Router last zone with invalid token unexpectedly succeeded: %#v", invalidTokenZone)
	}

	var routerLogin httpEnvelope
	status, _, err = requestJSON(
		t,
		stack.routerURL+"/login",
		http.MethodPost,
		map[string]string{"Username": username, "Password": password},
		&routerLogin,
	)
	if err != nil {
		t.Fatalf("Router login error: %v", err)
	}
	if status != http.StatusOK || routerLogin.Error != 0 || routerLogin.AccessToken == "" {
		t.Fatalf("Router login status=%d response=%#v", status, routerLogin)
	}

	return loginResponse.AccessToken
}

func testMapAndInventoryInterfaces(t *testing.T, client *kcpClient, playerID int64) {
	t.Helper()

	enter := callProto[*mapouterpb.M2C_EnterMap](
		t,
		client,
		statesync.MsgC2MEnterMap,
		statesync.MsgM2CEnterMap,
		&mapouterpb.C2M_EnterMap{},
	)
	if enter.Error != 0 {
		t.Fatalf("enter map response = %#v", enter)
	}

	createMyUnit := waitMessage(t, client, statesync.MsgCreateMyUnit)
	var unitInfo mapouterpb.M2C_CreateMyUnit
	unmarshalMessage(t, createMyUnit, &unitInfo)
	if unitInfo.Unit == nil || unitInfo.Unit.UnitId != playerID {
		t.Fatalf("create my unit = %#v, want unit id %d", unitInfo.Unit, playerID)
	}

	bag := callProto[*inventorypb.M2C_GetBagInfo](
		t,
		client,
		inventory.MsgC2MGetBagInfo,
		inventory.MsgM2CGetBagInfo,
		&inventorypb.C2M_GetBagInfo{UnitId: playerID},
	)
	if bag.Error != 0 || bag.MaxCapacity <= 0 {
		t.Fatalf("bag response = %#v", bag)
	}

	warehouse := callProto[*inventorypb.M2C_GetWarehouseInfo](
		t,
		client,
		inventory.MsgC2MGetWarehouseInfo,
		inventory.MsgM2CGetWarehouseInfo,
		&inventorypb.C2M_GetWarehouseInfo{UnitId: playerID},
	)
	if warehouse.Error != 0 || warehouse.MaxCapacity <= 0 {
		t.Fatalf("warehouse response = %#v", warehouse)
	}

	bagOperation := callProto[*inventorypb.M2C_BagOperation](
		t,
		client,
		inventory.MsgC2MBagOperation,
		inventory.MsgM2CBagOperation,
		&inventorypb.C2M_BagOperation{
			UnitId: playerID,
			OpType: 99,
		},
	)
	if bagOperation.Error == 0 {
		t.Fatalf("invalid bag operation unexpectedly succeeded: %#v", bagOperation)
	}

	warehouseOperation := callProto[*inventorypb.M2C_WarehouseOperation](
		t,
		client,
		inventory.MsgC2MWarehouseOp,
		inventory.MsgM2CWarehouseOp,
		&inventorypb.C2M_WarehouseOperation{
			UnitId: playerID,
			OpType: 99,
		},
	)
	if warehouseOperation.Error == 0 {
		t.Fatalf("invalid warehouse operation unexpectedly succeeded: %#v", warehouseOperation)
	}

	emptyBagSwap := callProto[*inventorypb.M2C_BagOperation](
		t,
		client,
		inventory.MsgC2MBagOperation,
		inventory.MsgM2CBagOperation,
		&inventorypb.C2M_BagOperation{
			UnitId:     playerID,
			OpType:     3,
			SourceSlot: 0,
			TargetSlot: 1,
		},
	)
	if emptyBagSwap.Error != 0 {
		t.Fatalf("empty bag swap response = %#v", emptyBagSwap)
	}

	emptyBagSort := callProto[*inventorypb.M2C_BagOperation](
		t,
		client,
		inventory.MsgC2MBagOperation,
		inventory.MsgM2CBagOperation,
		&inventorypb.C2M_BagOperation{
			UnitId: playerID,
			OpType: 4,
		},
	)
	if emptyBagSort.Error != 0 {
		t.Fatalf("empty bag sort response = %#v", emptyBagSort)
	}

	emptyWarehouseSwap := callProto[*inventorypb.M2C_WarehouseOperation](
		t,
		client,
		inventory.MsgC2MWarehouseOp,
		inventory.MsgM2CWarehouseOp,
		&inventorypb.C2M_WarehouseOperation{
			UnitId:     playerID,
			OpType:     3,
			SourceSlot: 0,
			TargetSlot: 1,
		},
	)
	if emptyWarehouseSwap.Error != 0 {
		t.Fatalf("empty warehouse swap response = %#v", emptyWarehouseSwap)
	}

	invalidWarehouseCount := callProto[*inventorypb.M2C_WarehouseOperation](
		t,
		client,
		inventory.MsgC2MWarehouseOp,
		inventory.MsgM2CWarehouseOp,
		&inventorypb.C2M_WarehouseOperation{
			UnitId: playerID,
			OpType: 1,
			Count:  0,
		},
	)
	if invalidWarehouseCount.Error == 0 {
		t.Fatalf("zero-count warehouse operation unexpectedly succeeded: %#v", invalidWarehouseCount)
	}

	sendProtoMessage(t, client, statesync.MsgC2MPathfindingResult, &mapouterpb.C2M_PathfindingResult{
		Position: &mapouterpb.Float3{X: 10, Y: 0, Z: 10},
	})
	stopMessage := waitMessage(t, client, statesync.MsgStop)
	var stop mapouterpb.M2C_Stop
	unmarshalMessage(t, stopMessage, &stop)
	if stop.Error != 3 || stop.Id != playerID {
		t.Fatalf("missing-finder stop = %#v", stop)
	}

	sendProtoMessage(t, client, statesync.MsgC2MStop, &mapouterpb.C2M_Stop{})
	stopMessage = waitMessage(t, client, statesync.MsgStop)
	unmarshalMessage(t, stopMessage, &stop)
	if stop.Error != 0 || stop.Id != playerID {
		t.Fatalf("stop response = %#v", stop)
	}
}

func testMapTransferInterface(t *testing.T, client *kcpClient, playerID int64) {
	t.Helper()

	response := callProto[*mapouterpb.M2C_TransferMap](
		t,
		client,
		statesync.MsgC2MTransferMap,
		statesync.MsgM2CTransferMap,
		&mapouterpb.C2M_TransferMap{},
	)
	if response.Error != 0 {
		t.Fatalf("transfer response = %#v", response)
	}

	sceneChange := waitMessage(t, client, statesync.MsgStartSceneChange)
	var change mapouterpb.M2C_StartSceneChange
	unmarshalMessage(t, sceneChange, &change)
	if change.SceneName != "Map2" {
		t.Fatalf("scene change = %#v, want Map2", change)
	}

	removeUnits := waitMessage(t, client, statesync.MsgRemoveUnits)
	var removed mapouterpb.M2C_RemoveUnits
	unmarshalMessage(t, removeUnits, &removed)
	if len(removed.GetUnits()) == 0 {
		t.Fatalf("remove units after transfer = %#v", removed)
	}

	createMyUnit := waitMessage(t, client, statesync.MsgCreateMyUnit)
	var unitInfo mapouterpb.M2C_CreateMyUnit
	unmarshalMessage(t, createMyUnit, &unitInfo)
	if unitInfo.Unit == nil || unitInfo.Unit.UnitId != playerID {
		t.Fatalf("transferred unit = %#v, want unit id %d", unitInfo.Unit, playerID)
	}
}

func containsUnit(units []*mapouterpb.UnitInfo, unitID int64) bool {
	for _, unit := range units {
		if unit != nil && unit.UnitId == unitID {
			return true
		}
	}
	return false
}

func testLockstepInterfaces(t *testing.T, stack *fullStack, first *kcpClient, firstPlayerID int64, password string) {
	t.Helper()

	secondUsername := fmt.Sprintf("system_second_%d", time.Now().UnixNano())
	var register httpEnvelope
	_, _, err := requestJSON(
		t,
		stack.httpURL+"/register",
		http.MethodPost,
		map[string]string{"Username": secondUsername, "Password": password},
		&register,
	)
	if err != nil || register.Error != 0 {
		t.Fatalf("second registration response=%#v err=%v", register, err)
	}

	var loginResponse httpEnvelope
	_, _, err = requestJSON(
		t,
		stack.httpURL+"/login",
		http.MethodPost,
		map[string]string{"Username": secondUsername, "Password": password},
		&loginResponse,
	)
	if err != nil || loginResponse.Error != 0 {
		t.Fatalf("second HTTP login response=%#v err=%v", loginResponse, err)
	}

	realm := dialKCP(t, stack.realmAddress)
	realmLogin := callProto[*etproto.R2C_Login](
		t,
		realm,
		2001,
		2002,
		&etproto.C2R_Login{
			AccessToken: loginResponse.AccessToken,
			ZoneId:      1,
		},
	)
	realm.Close()
	if realmLogin.Error != 0 {
		t.Fatalf("second realm login response = %#v", realmLogin)
	}

	second := dialKCP(t, realmLogin.Address)
	t.Cleanup(second.Close)
	secondLogin := callProto[*etproto.G2C_LoginGate](
		t,
		second,
		2105,
		2106,
		&etproto.C2G_LoginGate{
			Token:  realmLogin.Token,
			GateId: realmLogin.GateId,
		},
	)
	if secondLogin.Error != 0 || secondLogin.PlayerId <= 0 {
		t.Fatalf("second gate login response = %#v", secondLogin)
	}

	secondEnter := callProto[*mapouterpb.M2C_EnterMap](
		t,
		second,
		statesync.MsgC2MEnterMap,
		statesync.MsgM2CEnterMap,
		&mapouterpb.C2M_EnterMap{},
	)
	if secondEnter.Error != 0 {
		t.Fatalf("second enter map response = %#v", secondEnter)
	}
	secondCreateMyUnit := waitMessage(t, second, statesync.MsgCreateMyUnit)
	var secondUnitInfo mapouterpb.M2C_CreateMyUnit
	unmarshalMessage(t, secondCreateMyUnit, &secondUnitInfo)
	if secondUnitInfo.Unit == nil || secondUnitInfo.Unit.UnitId != secondLogin.PlayerId {
		t.Fatalf("second create my unit = %#v", secondUnitInfo.Unit)
	}

	firstVisibleUnits := waitMessage(t, first, statesync.MsgCreateUnits)
	var firstVisible mapouterpb.M2C_CreateUnits
	unmarshalMessage(t, firstVisibleUnits, &firstVisible)
	if !containsUnit(firstVisible.GetUnits(), secondLogin.PlayerId) {
		t.Fatalf("first AOI create units = %#v, missing second player %d", firstVisible.GetUnits(), secondLogin.PlayerId)
	}

	secondVisibleUnits := waitMessage(t, second, statesync.MsgCreateUnits)
	var secondVisible mapouterpb.M2C_CreateUnits
	unmarshalMessage(t, secondVisibleUnits, &secondVisible)
	if !containsUnit(secondVisible.GetUnits(), firstPlayerID) {
		t.Fatalf("second AOI create units = %#v, missing first player %d", secondVisible.GetUnits(), firstPlayerID)
	}

	spoofedMatch := callProto[*locksteppb.G2C_Match](
		t,
		first,
		lockstep.MsgC2GMatch,
		lockstep.MsgG2CMatch,
		&locksteppb.C2G_Match{PlayerId: firstPlayerID + 999999},
	)
	if spoofedMatch.Error == 0 {
		t.Fatalf("spoofed match unexpectedly succeeded: %#v", spoofedMatch)
	}

	firstMatch := callProto[*locksteppb.G2C_Match](
		t,
		first,
		lockstep.MsgC2GMatch,
		lockstep.MsgG2CMatch,
		&locksteppb.C2G_Match{PlayerId: firstPlayerID},
	)
	if firstMatch.Error != 0 {
		t.Fatalf("first match response = %#v", firstMatch)
	}
	firstNotification := waitMessage(t, first, lockstep.MsgG2CNotifyMatchSuccess)
	var firstSuccess locksteppb.G2C_NotifyMatchSuccess
	unmarshalMessage(t, firstNotification, &firstSuccess)
	if firstSuccess.MapActor == nil || firstSuccess.RoomActor == nil {
		t.Fatalf("first match notification missing actor ids: %#v", firstSuccess)
	}
	sendProtoMessage(t, first, lockstep.MsgC2RoomChangeSceneFinish, &locksteppb.C2Room_ChangeSceneFinish{})
	firstStart := waitMessage(t, first, lockstep.MsgRoom2CStart)
	var firstRoomStart locksteppb.Room2C_Start
	unmarshalMessage(t, firstStart, &firstRoomStart)
	if len(firstRoomStart.UnitInfos) != 1 {
		t.Fatalf("first room start units: %d, want 1", len(firstRoomStart.UnitInfos))
	}

	secondMatch := callProto[*locksteppb.G2C_Match](
		t,
		second,
		lockstep.MsgC2GMatch,
		lockstep.MsgG2CMatch,
		&locksteppb.C2G_Match{PlayerId: secondLogin.PlayerId},
	)
	if secondMatch.Error != 0 {
		t.Fatalf("second match response = %#v", secondMatch)
	}
	secondNotification := waitMessage(t, second, lockstep.MsgG2CNotifyMatchSuccess)
	var secondSuccess locksteppb.G2C_NotifyMatchSuccess
	unmarshalMessage(t, secondNotification, &secondSuccess)
	if secondSuccess.MapActor == nil || secondSuccess.RoomActor == nil {
		t.Fatalf("second match notification missing actor ids: %#v", secondSuccess)
	}
	sendProtoMessage(t, second, lockstep.MsgC2RoomChangeSceneFinish, &locksteppb.C2Room_ChangeSceneFinish{})
	secondStart := waitMessage(t, second, lockstep.MsgRoom2CStart)
	var secondRoomStart locksteppb.Room2C_Start
	unmarshalMessage(t, secondStart, &secondRoomStart)
	if len(secondRoomStart.UnitInfos) != 1 {
		t.Fatalf("room start units: first=%d second=%d", len(firstRoomStart.UnitInfos), len(secondRoomStart.UnitInfos))
	}

	sendProtoMessage(t, first, lockstep.MsgFrameMessage, &locksteppb.FrameMessage{
		PlayerId: firstPlayerID,
		Frame:    1,
		Input: &locksteppb.LSInput{
			V: &locksteppb.TSVector2{X: 1 << 32, Y: 0},
		},
	})
	sendProtoMessage(t, second, lockstep.MsgFrameMessage, &locksteppb.FrameMessage{
		PlayerId: secondLogin.PlayerId,
		Frame:    1,
		Input: &locksteppb.LSInput{
			V: &locksteppb.TSVector2{X: 0, Y: 1 << 32},
		},
	})

	oneFrame := waitMessage(t, first, lockstep.MsgOneFrameInputs)
	var oneFrameInputs locksteppb.OneFrameInputs
	unmarshalMessage(t, oneFrame, &oneFrameInputs)
	if _, ok := oneFrameInputs.Inputs[firstPlayerID]; !ok {
		t.Fatalf("one-frame inputs = %#v, missing player %d", oneFrameInputs.Inputs, firstPlayerID)
	}

	sendProtoMessage(t, first, lockstep.MsgFrameMessage, &locksteppb.FrameMessage{
		PlayerId: firstPlayerID,
		Frame:    8,
		Input: &locksteppb.LSInput{
			V: &locksteppb.TSVector2{X: 8 << 32},
		},
	})
	adjust := waitMessage(t, first, lockstep.MsgRoom2CAdjustUpdateTime)
	var adjustMessage locksteppb.Room2C_AdjustUpdateTime
	unmarshalMessage(t, adjust, &adjustMessage)
	if adjustMessage.DiffTime == 0 {
		t.Fatalf("adjust update time = %#v", adjustMessage)
	}

	sendProtoMessage(t, first, lockstep.MsgC2RoomCheckHash, &locksteppb.C2Room_CheckHash{
		Frame: 1,
		Hash:  12345,
	})
	sendProtoMessage(t, first, lockstep.MsgC2RoomCheckHash, &locksteppb.C2Room_CheckHash{
		Frame: 1,
		Hash:  12346,
	})
	hashFail := waitMessage(t, first, lockstep.MsgRoom2CCheckHashFail)
	var hashFailMessage locksteppb.Room2C_CheckHashFail
	unmarshalMessage(t, hashFail, &hashFailMessage)
	if hashFailMessage.Frame != 1 || len(hashFailMessage.Snapshot) == 0 {
		t.Fatalf("hash mismatch notification = %#v", hashFailMessage)
	}
	pingAfterRoomMessages := callProto[*etproto.G2C_Ping](
		t,
		first,
		2103,
		2104,
		&etproto.C2G_Ping{},
	)
	if pingAfterRoomMessages.Error != 0 {
		t.Fatalf("ping after room messages = %#v", pingAfterRoomMessages)
	}
}

func testRouterTransports(t *testing.T, address string) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split router address %q: %v", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("router port %q: %v", portText, err)
	}
	routerAddr := &net.UDPAddr{IP: net.ParseIP(host), Port: port}
	innerAddress := []byte("127.0.0.1:19999")

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen router UDP test socket: %v", err)
	}
	defer udpConn.Close()
	if err := udpConn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set UDP deadline: %v", err)
	}
	udpFrame := routerSYNFrame(101, 0, 501, innerAddress)
	if _, err := udpConn.WriteToUDP(udpFrame, routerAddr); err != nil {
		t.Fatalf("write router UDP SYN: %v", err)
	}
	buffer := make([]byte, 1024)
	n, _, err := udpConn.ReadFromUDP(buffer)
	if err != nil {
		t.Fatalf("read router UDP ACK: %v", err)
	}
	assertRouterACK(t, buffer[:n], 101)

	tcpConn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		t.Fatalf("dial router TCP: %v", err)
	}
	defer tcpConn.Close()
	tcpFrame := routerSYNFrame(102, 0, 502, innerAddress)
	length := make([]byte, 2)
	binary.LittleEndian.PutUint16(length, uint16(len(tcpFrame)))
	if _, err := tcpConn.Write(append(length, tcpFrame...)); err != nil {
		t.Fatalf("write router TCP SYN: %v", err)
	}
	if err := tcpConn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set TCP deadline: %v", err)
	}
	if _, err := io.ReadFull(tcpConn, length); err != nil {
		t.Fatalf("read router TCP frame length: %v", err)
	}
	ackLength := int(binary.LittleEndian.Uint16(length))
	if ackLength < 9 {
		t.Fatalf("router TCP ACK length = %d", ackLength)
	}
	ack := make([]byte, ackLength)
	if _, err := io.ReadFull(tcpConn, ack); err != nil {
		t.Fatalf("read router TCP ACK: %v", err)
	}
	assertRouterACK(t, ack, 102)
}

func startFullStack(t *testing.T) *fullStack {
	t.Helper()
	binaryPath := os.Getenv("ETGO_SERVER_BINARY")
	if binaryPath == "" {
		binaryPath = filepath.Join(repoRoot(), "bin", "server")
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("server binary %q is unavailable: %v", binaryPath, err)
	}

	httpPort := reserveTCPPort(t)
	routerPort := reserveTCPPort(t)
	realmPort := reserveUDPPort(t)
	gatePort := reserveUDPPort(t)
	routerNodePort := reserveTCPPort(t)
	dbName := "etgo_system_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	configDir := t.TempDir()

	machine := []config.StartMachineConfig{{
		ID:      1,
		InnerIP: "127.0.0.1",
		OuterIP: "127.0.0.1",
	}}
	process := []config.StartProcessConfig{{
		ID:        1,
		MachineID: 1,
	}}
	scenes := []config.StartSceneConfig{
		{ID: 3001, ProcessID: 1, Zone: 1, SceneType: "Location", Name: "Location"},
		{ID: 20001, ProcessID: 1, Zone: 1, SceneType: "Central", Name: "Central"},
		{ID: 18001, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Home", MapTargets: []string{"Map2"}},
		{ID: 18002, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Map2"},
		{ID: 11002, ProcessID: 1, Zone: 1, SceneType: "Match", Name: "Match"},
		{ID: 9003, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "Realm", OuterPort: realmPort},
		{ID: 9004, ProcessID: 1, Zone: 1, SceneType: "Gate", Name: "Gate", OuterPort: gatePort},
		{ID: 16001, ProcessID: 1, Zone: 1, SceneType: "HTTP", Name: "HTTP", OuterPort: httpPort},
		{ID: 9001, ProcessID: 1, Zone: 1, SceneType: "Router", Name: "Router", OuterPort: routerPort},
		{ID: 9002, ProcessID: 1, Zone: 1, SceneType: "RouterNode", Name: "RouterNode", OuterPort: routerNodePort},
	}
	zones := []config.StartZoneConfig{{
		ID:        1,
		Name:      "system",
		DBName:    dbName,
		DBAddr:    "mongodb://127.0.0.1:27017",
		ServerURL: "http://127.0.0.1:" + strconv.Itoa(httpPort),
		IsLogic:   true,
	}}
	areas := []config.StartAreaConfig{{
		ID:        1,
		Name:      "system",
		ServerURL: "http://127.0.0.1:" + strconv.Itoa(httpPort),
	}}
	security := config.StartSecurityConfig{
		AccessTokenFormat:       "signed",
		AccessTokenCurrentKeyID: "system-primary",
		AccessTokenKeys: []config.StartAccessTokenKeyConfig{{
			ID:     "system-primary",
			Secret: "et-go-system-test-secret-32-bytes-minimum",
		}},
		CORSAllowedOrigins:      []string{"http://127.0.0.1:3000"},
		LoginRateLimitPerMinute: 100,
	}

	writeJSONFile(t, configDir, "startmachineconfig.json", machine)
	writeJSONFile(t, configDir, "startprocessconfig.json", process)
	writeJSONFile(t, configDir, "startsceneconfig.json", scenes)
	writeJSONFile(t, configDir, "startzoneconfig.json", zones)
	writeJSONFile(t, configDir, "startareaconfig.json", areas)
	writeJSONFile(t, configDir, "startsecurityconfig.json", security)

	logFile, err := os.Create(filepath.Join(configDir, "server.log"))
	if err != nil {
		t.Fatalf("create system server log: %v", err)
	}
	cmd := exec.Command(
		binaryPath,
		"--process=1",
		"--config="+configDir,
		"--log-level=debug",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start full stack server: %v", err)
	}

	stack := &fullStack{
		cmd:          cmd,
		logFile:      logFile,
		configDir:    configDir,
		dbName:       dbName,
		httpURL:      "http://127.0.0.1:" + strconv.Itoa(httpPort),
		routerURL:    "http://127.0.0.1:" + strconv.Itoa(routerPort),
		realmAddress: "127.0.0.1:" + strconv.Itoa(realmPort),
		gateAddress:  "127.0.0.1:" + strconv.Itoa(gatePort),
		routerNode:   "127.0.0.1:" + strconv.Itoa(routerNodePort),
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("full stack server log:\n%s", readServerLog(stack))
		}
		stack.stop()
		dropDatabase(t, dbName)
	})
	waitForServer(t, stack)
	return stack
}

func (s *fullStack) stop() {
	if s == nil {
		return
	}
	if s.cmd != nil && s.cmd.Process != nil && s.cmd.ProcessState == nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = s.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
			<-done
		}
	}
	if s.logFile != nil {
		_ = s.logFile.Close()
	}
}

func waitForServer(t *testing.T, stack *fullStack) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if stack.cmd.ProcessState != nil {
			t.Fatalf("full stack server exited early:\n%s", readServerLog(stack))
		}
		response, err := client.Get(stack.httpURL + "/")
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("full stack server did not become ready:\n%s", readServerLog(stack))
}

func readServerLog(stack *fullStack) string {
	if stack == nil || stack.configDir == "" {
		return ""
	}
	data, _ := os.ReadFile(filepath.Join(stack.configDir, "server.log"))
	return string(data)
}

func dropDatabase(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := etdb.New(ctx, "mongodb://127.0.0.1:27017", name)
	if err != nil {
		t.Logf("drop system test database connect: %v", err)
		return
	}
	if err := client.Database().Drop(ctx); err != nil {
		t.Logf("drop system test database: %v", err)
	}
	if err := client.Close(ctx); err != nil {
		t.Logf("close system test database client: %v", err)
	}
}

func requestJSON(t *testing.T, endpoint, method string, body any, output any) (int, http.Header, error) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = strings.NewReader(string(payload))
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, response.Header, err
	}
	if output != nil && len(data) > 0 {
		if err := json.Unmarshal(data, output); err != nil {
			return response.StatusCode, response.Header, err
		}
	}
	return response.StatusCode, response.Header, nil
}

func dialKCP(t *testing.T, address string) *kcpClient {
	t.Helper()
	service := kcp.NewService(kcp.InnerConfig(), nil)
	if err := service.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("KCP client listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				service.Update()
			}
		}
	}()

	channel, err := service.Connect(address)
	if err != nil {
		cancel()
		service.Close()
		<-updateDone
		t.Fatalf("KCP client connect %s: %v", address, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for channel.Status() != kcp.ChannelStatusConnected && time.Now().Before(deadline) {
		if channel.Status() == kcp.ChannelStatusDisconnected {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if channel.Status() != kcp.ChannelStatusConnected || channel.Conn() == nil {
		cancel()
		service.Close()
		<-updateDone
		t.Fatalf("KCP client did not connect %s, status=%v", address, channel.Status())
	}

	client := &kcpClient{
		service:    service,
		channel:    channel,
		ctx:        ctx,
		cancel:     cancel,
		messages:   make(chan *codec.Packet, 128),
		updateDone: updateDone,
	}
	client.session = network.NewSession(ctx, sessionID.Add(1), channel.Conn(), nil)
	client.session.StartReadLoop(func(_ *network.Session, packet *codec.Packet) {
		select {
		case client.messages <- packet:
		default:
		}
	})
	client.session.StartWriteLoop()
	t.Cleanup(client.Close)
	return client
}

func (c *kcpClient) Close() {
	if c == nil {
		return
	}
	if c.session != nil {
		c.session.Close()
	}
	if c.channel != nil {
		c.channel.Close()
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.service != nil {
		c.service.Close()
	}
	if c.updateDone != nil {
		select {
		case <-c.updateDone:
		case <-time.After(2 * time.Second):
		}
	}
}

func callProto[T gproto.Message](t *testing.T, client *kcpClient, msgID, responseMsgID uint16, request gproto.Message) T {
	t.Helper()
	payload, err := gproto.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request msg=%d: %v", msgID, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	response, err := client.session.Call(ctx, &codec.Packet{
		Type:    codec.PacketTypeRequest,
		MsgID:   msgID,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("call msg=%d: %v", msgID, err)
	}
	if response.MsgID != responseMsgID {
		t.Fatalf("response msg=%d, want %d", response.MsgID, responseMsgID)
	}
	result := newMessage[T]()
	if err := gproto.Unmarshal(response.Payload, result); err != nil {
		t.Fatalf("unmarshal response msg=%d: %v", response.MsgID, err)
	}
	return result
}

func newMessage[T gproto.Message]() T {
	var zero T
	switch any(zero).(type) {
	case *etproto.R2C_Login:
		return any(&etproto.R2C_Login{}).(T)
	case *etproto.G2C_LoginGate:
		return any(&etproto.G2C_LoginGate{}).(T)
	case *etproto.G2C_Ping:
		return any(&etproto.G2C_Ping{}).(T)
	case *mapouterpb.M2C_EnterMap:
		return any(&mapouterpb.M2C_EnterMap{}).(T)
	case *mapouterpb.M2C_TransferMap:
		return any(&mapouterpb.M2C_TransferMap{}).(T)
	case *inventorypb.M2C_GetBagInfo:
		return any(&inventorypb.M2C_GetBagInfo{}).(T)
	case *inventorypb.M2C_GetWarehouseInfo:
		return any(&inventorypb.M2C_GetWarehouseInfo{}).(T)
	case *inventorypb.M2C_BagOperation:
		return any(&inventorypb.M2C_BagOperation{}).(T)
	case *inventorypb.M2C_WarehouseOperation:
		return any(&inventorypb.M2C_WarehouseOperation{}).(T)
	case *locksteppb.G2C_Match:
		return any(&locksteppb.G2C_Match{}).(T)
	default:
		panic(fmt.Sprintf("unsupported system-test response type %T", zero))
	}
}

func sendProtoMessage(t *testing.T, client *kcpClient, msgID uint16, message gproto.Message) {
	t.Helper()
	payload, err := gproto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal message msg=%d: %v", msgID, err)
	}
	if err := client.session.Send(&codec.Packet{
		Type:    codec.PacketTypeMessage,
		MsgID:   msgID,
		Payload: payload,
	}); err != nil {
		t.Fatalf("send message msg=%d: %v", msgID, err)
	}
}

func sendProtoRequest(t *testing.T, client *kcpClient, msgID uint16, message gproto.Message) {
	t.Helper()
	payload, err := gproto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal request msg=%d: %v", msgID, err)
	}
	if err := client.session.Send(&codec.Packet{
		Type:    codec.PacketTypeRequest,
		MsgID:   msgID,
		RpcID:   client.session.NextRpcID(),
		Payload: payload,
	}); err != nil {
		t.Fatalf("send request msg=%d: %v", msgID, err)
	}
}

func waitMessage(t *testing.T, client *kcpClient, msgID uint16) *codec.Packet {
	t.Helper()
	for index, packet := range client.pending {
		if packet != nil && packet.MsgID == msgID {
			client.pending = append(client.pending[:index], client.pending[index+1:]...)
			return packet
		}
	}
	deadline := time.After(8 * time.Second)
	seen := make([]uint16, 0, 8)
	for {
		select {
		case packet := <-client.messages:
			if packet != nil && packet.MsgID == msgID {
				return packet
			}
			if packet != nil && len(seen) < cap(seen) {
				seen = append(seen, packet.MsgID)
			}
			if packet != nil {
				client.pending = append(client.pending, packet)
			}
		case <-deadline:
			t.Fatalf("wait message msg=%d timed out; seen message ids=%v", msgID, seen)
			return nil
		}
	}
}

func unmarshalMessage(t *testing.T, packet *codec.Packet, message gproto.Message) {
	t.Helper()
	if packet == nil {
		t.Fatal("message packet is nil")
	}
	if err := gproto.Unmarshal(packet.Payload, message); err != nil {
		t.Fatalf("unmarshal message msg=%d: %v", packet.MsgID, err)
	}
}

func routerSYNFrame(outerConn, innerConn, connectID uint32, innerAddress []byte) []byte {
	frame := make([]byte, 13+len(innerAddress))
	frame[0] = 7
	binary.LittleEndian.PutUint32(frame[1:5], outerConn)
	binary.LittleEndian.PutUint32(frame[5:9], innerConn)
	binary.LittleEndian.PutUint32(frame[9:13], connectID)
	copy(frame[13:], innerAddress)
	return frame
}

func assertRouterACK(t *testing.T, frame []byte, outerConn uint32) {
	t.Helper()
	if len(frame) < 9 || frame[0] != 8 {
		t.Fatalf("router ACK frame = %x", frame)
	}
	if got := binary.LittleEndian.Uint32(frame[1:5]); got != 0 {
		t.Fatalf("router ACK inner connection = %d, want 0", got)
	}
	if got := binary.LittleEndian.Uint32(frame[5:9]); got != outerConn {
		t.Fatalf("router ACK outer connection = %d, want %d", got, outerConn)
	}
}

func writeJSONFile(t *testing.T, dir, name string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal config %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("write config %s: %v", name, err)
	}
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func reserveUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(file))
}
