package router

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
	"github.com/jerbe/et-go/module/login"
)

type stubRouterMessageSender struct {
	ecs.BaseComponent
	payload []byte
	err     error
	msgID   uint16
	actorID actor.ActorID
}

func (s *stubRouterMessageSender) Type() string { return "MessageSender" }
func (s *stubRouterMessageSender) Call(_ context.Context, actorID actor.ActorID, msgID uint16, _ []byte) ([]byte, error) {
	s.actorID = actorID
	s.msgID = msgID
	if s.err != nil {
		return nil, s.err
	}
	return s.payload, nil
}

type stubSceneFiber struct {
	id        int64
	processID int
}

func (f stubSceneFiber) ID() int64      { return f.id }
func (f stubSceneFiber) ProcessID() int { return f.processID }

func TestRouterListHandler(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(&config.Config{
		Machines: []config.StartMachineConfig{
			{ID: 1, InnerIP: "10.0.0.1", OuterIP: "20.0.0.1"},
		},
		Processes: []config.StartProcessConfig{
			{ID: 1, MachineID: 1, InnerPort: 9001},
			{ID: 2, MachineID: 1, InnerPort: 9002},
		},
		Scenes: []config.StartSceneConfig{
			{ID: 100, SceneType: "Router", Name: "router-manager", Zone: 1, ProcessID: 1, OuterPort: 8080},
			{ID: 101, SceneType: "Realm", Name: "realm", Zone: 10, ProcessID: 1, OuterPort: 1001},
			{ID: 102, SceneType: "RouterNode", Name: "router-node", Zone: 11, ProcessID: 2, OuterPort: 1002},
		},
	})
	req := httptest.NewRequest(nethttp.MethodGet, "/router/list", nil)
	rec := httptest.NewRecorder()
	scene := ecs.NewScene(ecs.SceneTypeRouter, 1, "router")
	scene.SetID(100)
	if err := (&httpRouterListHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal resp err = %v", err)
	}
	if payload["ServerIP"] != "20.0.0.1" {
		t.Fatalf("unexpected ServerIP %v", payload["ServerIP"])
	}
	if realms, ok := payload["Realms"].([]any); !ok || len(realms) != 1 {
		t.Fatalf("expected 1 realm entry, got %v", payload["Realms"])
	} else if realm, ok := realms[0].(string); !ok || realm != "10.0.0.1:1001" {
		t.Fatalf("unexpected realm data %v", realms[0])
	}
	if routers, ok := payload["Routers"].([]any); !ok || len(routers) != 1 {
		t.Fatalf("expected 1 router entry, got %v", payload["Routers"])
	} else if routerEntry, ok := routers[0].(string); !ok || routerEntry != "20.0.0.1:1002" {
		t.Fatalf("unexpected router data %v", routers[0])
	}
}

func TestZoneListHandler(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(&config.Config{
		Zones: []config.StartZoneConfig{
			{ID: 1, Name: "Zone 1", IsLogic: true},
			{ID: 2, DBName: "zone-2-db", IsLogic: true},
			{ID: 3, Name: "Zone 3", IsLogic: false},
		},
	})
	req := httptest.NewRequest(nethttp.MethodGet, "/zone/list", nil)
	rec := httptest.NewRecorder()
	if err := (&httpZoneListHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal resp err = %v", err)
	}
	zones, ok := payload["Zones"].([]any)
	if !ok || len(zones) != 2 {
		t.Fatalf("expected two zones, got %v", payload["Zones"])
	}
	if zone0, ok := zones[0].(map[string]any); !ok || zone0["Id"] != float64(1) || zone0["Name"] != "Zone 1" || zone0["Status"] != float64(0) {
		t.Fatalf("unexpected zone data %v", zones[0])
	}
	if zone1, ok := zones[1].(map[string]any); !ok || zone1["Id"] != float64(2) || zone1["Name"] != "" || zone1["Status"] != float64(0) {
		t.Fatalf("unexpected zone data %v", zones[1])
	}
}

func TestRouterReadHandlersRejectNonGet(t *testing.T) {
	old := config.GetGlobal()
	t.Cleanup(func() { config.SetGlobal(old) })
	config.SetGlobal(&config.Config{})

	tests := []struct {
		name    string
		handler interface {
			Handle(*ecs.Scene, *nethttp.Request, nethttp.ResponseWriter) error
		}
		path string
	}{
		{name: "router list", handler: &httpRouterListHandler{}, path: "/router/list"},
		{name: "zone list", handler: &httpZoneListHandler{}, path: "/zone/list"},
		{name: "last zone", handler: &httpLastZoneHandler{}, path: "/zone/last"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(nethttp.MethodPost, test.path, nil)
			rec := httptest.NewRecorder()
			if err := test.handler.Handle(nil, req, rec); err != nil {
				t.Fatalf("handler error = %v", err)
			}
			if rec.Code != nethttp.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, nethttp.StatusOK)
			}
		})
	}
}

func TestRouterLoginHandler(t *testing.T) {
	centralScene := ecs.NewScene(ecs.SceneTypeCentral, 1, "central")
	centralScene.SetFiber(stubSceneFiber{id: 100, processID: 2})
	actor.UpdateSceneRegistry(centralScene)
	defer actor.RemoveSceneRegistry(centralScene)

	payload, err := central.MarshalCentral2RAccountLogin(&central.Central2RAccountLogin{
		RpcId:       1,
		AccessToken: "access-token",
	})
	if err != nil {
		t.Fatalf("marshal resp err = %v", err)
	}
	sender := &stubRouterMessageSender{payload: payload}
	scene := ecs.NewScene(ecs.SceneTypeRouter, 2, "router")
	scene.AddComponent(sender)

	req := httptest.NewRequest(nethttp.MethodPost, "/login", strings.NewReader(`{"Username":"user","Password":"pass"}`))
	rec := httptest.NewRecorder()
	if err := (&httpRouterLoginHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if sender.msgID != central.MsgR2CentralAccountLogin || !sender.actorID.IsValid() {
		t.Fatalf("unexpected sender call msgID=%d actorID=%+v", sender.msgID, sender.actorID)
	}
	var resp moduleHTTPLoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if resp.AccessToken != "access-token" || resp.Error != 0 {
		t.Fatalf("unexpected login resp %+v", resp)
	}
}

func TestLastZoneHandler(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(&config.Config{
		Zones: []config.StartZoneConfig{
			{ID: 1, Name: "Zone 1", IsLogic: true},
			{ID: 2, DBName: "zone-2-db", IsLogic: true},
		},
	})
	accessToken, err := login.GenerateAccessToken(1001)
	if err != nil {
		t.Fatalf("GenerateAccessToken err = %v", err)
	}

	req := httptest.NewRequest(nethttp.MethodGet, "/zone/last?access_token="+accessToken, nil)
	rec := httptest.NewRecorder()
	if err := (&httpLastZoneHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("handler err = %v", err)
	}
	var resp struct {
		Error   int             `json:"Error"`
		Message string          `json:"Message"`
		LastOne *routerZoneInfo `json:"LastOne"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode err = %v", err)
	}
	if resp.Error != 0 || resp.LastOne == nil || resp.LastOne.Name != "" || resp.LastOne.Status != 1 {
		t.Fatalf("unexpected resp %+v", resp)
	}
}

func TestLastZoneHandlerErrors(t *testing.T) {
	req := httptest.NewRequest(nethttp.MethodGet, "/zone/last", nil)
	rec := httptest.NewRecorder()
	if err := (&httpLastZoneHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("handler err = %v", err)
	}
	var missingResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &missingResp); err != nil {
		t.Fatalf("decode missing resp err = %v", err)
	}
	if missingResp["Error"] != float64(ERR_WithException) || missingResp["Message"] != "param invalid" {
		t.Fatalf("unexpected missing resp %v", missingResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(nethttp.MethodGet, "/zone/last?access_token=", nil)
	if err := (&httpLastZoneHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("empty token handler err = %v", err)
	}
	var emptyResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &emptyResp); err != nil {
		t.Fatalf("decode empty resp err = %v", err)
	}
	if emptyResp["Error"] != float64(login.ERR_TokenInvalidError) || emptyResp["Message"] != "token invalid" {
		t.Fatalf("unexpected empty resp %v", emptyResp)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(nethttp.MethodGet, "/zone/last?access_token=bad", nil)
	if err := (&httpLastZoneHandler{}).Handle(nil, req, rec); err != nil {
		t.Fatalf("invalid token handler err = %v", err)
	}
	var invalidResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &invalidResp); err != nil {
		t.Fatalf("decode invalid resp err = %v", err)
	}
	if invalidResp["Error"] != float64(login.ERR_TokenInvalidError) || invalidResp["Message"] != "token invalid" {
		t.Fatalf("unexpected invalid resp %v", invalidResp)
	}
}

type moduleHTTPLoginResponse struct {
	Error       int    `json:"Error"`
	Message     string `json:"Message"`
	AccessToken string `json:"AccessToken"`
}
