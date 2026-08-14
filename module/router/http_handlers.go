package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
	moduleHttp "github.com/jerbe/et-go/module/http"
	"github.com/jerbe/et-go/module/login"
)

const centralZoneID = 1

type httpRouterListHandler struct{}

type routerZoneInfo struct {
	Id     int32  `json:"Id"`
	Name   string `json:"Name"`
	Status int32  `json:"Status"`
}

type routerListResponse struct {
	moduleHttp.HttpResponse
	ServerIP string   `json:"ServerIP"`
	Realms   []string `json:"Realms"`
	Routers  []string `json:"Routers"`
}

type zoneListResponse struct {
	moduleHttp.HttpResponse
	Zones []routerZoneInfo `json:"Zones"`
}

type lastZoneResponse struct {
	moduleHttp.HttpResponse
	LastOne *routerZoneInfo `json:"LastOne"`
}

func (h *httpRouterListHandler) Handle(scene *ecs.Scene, req *http.Request, resp http.ResponseWriter) error {
	if err := validateRouterHTTP(req, resp); err != nil {
		return err
	}
	if req.Method != http.MethodGet {
		moduleHttp.WriteNotFound(resp)
		return nil
	}
	cfg := config.GetGlobal()
	if cfg == nil {
		return fmt.Errorf("router: configuration missing")
	}
	serverIP, err := resolveServerIP(scene, cfg)
	if err != nil {
		return err
	}
	result := routerListResponse{
		HttpResponse: moduleHttp.HttpResponse{},
		ServerIP:     serverIP,
		Realms:       make([]string, 0),
		Routers:      make([]string, 0),
	}
	for _, sceneCfg := range cfg.Scenes {
		sceneType := strings.ToLower(strings.TrimSpace(sceneCfg.SceneType))
		switch sceneType {
		case strings.ToLower(ecs.SceneTypeRealm.String()):
			address, err := resolveSceneAddress(cfg, sceneCfg, true)
			if err != nil {
				return err
			}
			result.Realms = append(result.Realms, address)
		case strings.ToLower(ecs.SceneTypeRouterNode.String()):
			address, err := resolveSceneAddress(cfg, sceneCfg, false)
			if err != nil {
				return err
			}
			result.Routers = append(result.Routers, address)
		}
	}
	moduleHttp.WriteJSON(resp, http.StatusOK, result)
	return nil
}

func resolveServerIP(scene *ecs.Scene, cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("router: configuration missing")
	}
	if scene == nil || scene.ID() <= 0 {
		return "", fmt.Errorf("router: runtime scene identity missing")
	}

	var sceneCfg *config.StartSceneConfig
	for index := range cfg.Scenes {
		if int64(cfg.Scenes[index].ID) == scene.ID() {
			sceneCfg = &cfg.Scenes[index]
			break
		}
	}
	if sceneCfg == nil {
		return "", fmt.Errorf("router: scene %d configuration missing", scene.ID())
	}
	machine := machineForProcess(cfg, sceneCfg.ProcessID)
	if machine == nil {
		return "", fmt.Errorf("router: process %d machine configuration missing", sceneCfg.ProcessID)
	}
	if host := strings.TrimSpace(machine.OuterIP); host != "" {
		return host, nil
	}
	if host := strings.TrimSpace(machine.InnerIP); host != "" {
		return host, nil
	}
	return "", fmt.Errorf("router: process %d machine address missing", sceneCfg.ProcessID)
}

func resolveSceneAddress(cfg *config.Config, sceneCfg config.StartSceneConfig, preferInner bool) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("router: configuration missing")
	}
	machine := machineForProcess(cfg, sceneCfg.ProcessID)
	if machine == nil {
		return "", fmt.Errorf("router: process %d machine configuration missing", sceneCfg.ProcessID)
	}

	var host string
	if preferInner {
		host = strings.TrimSpace(machine.InnerIP)
		if host == "" {
			host = strings.TrimSpace(machine.OuterIP)
		}
	} else {
		host = strings.TrimSpace(machine.OuterIP)
		if host == "" {
			host = strings.TrimSpace(machine.InnerIP)
		}
	}
	if host == "" {
		return "", fmt.Errorf("router: scene %d machine address missing", sceneCfg.ID)
	}

	port := sceneCfg.OuterPort
	if port <= 0 {
		for _, process := range cfg.Processes {
			if process.ID == sceneCfg.ProcessID {
				port = process.InnerPort
				break
			}
		}
	}
	if port <= 0 {
		return "", fmt.Errorf("router: scene %d advertised port missing", sceneCfg.ID)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func machineForProcess(cfg *config.Config, processID int) *config.StartMachineConfig {
	if cfg == nil {
		return nil
	}
	for _, process := range cfg.Processes {
		if process.ID != processID {
			continue
		}
		for index := range cfg.Machines {
			if cfg.Machines[index].ID == process.MachineID {
				return &cfg.Machines[index]
			}
		}
	}
	return nil
}

type httpZoneListHandler struct{}

func (h *httpZoneListHandler) Handle(scene *ecs.Scene, req *http.Request, resp http.ResponseWriter) error {
	if err := validateRouterHTTP(req, resp); err != nil {
		return err
	}
	if req.Method != http.MethodGet {
		moduleHttp.WriteNotFound(resp)
		return nil
	}
	cfg := config.GetGlobal()
	if cfg == nil {
		return moduleHttp.ErrConfigurationMissing
	}
	moduleHttp.WriteJSON(resp, http.StatusOK, zoneListResponse{
		HttpResponse: moduleHttp.HttpResponse{},
		Zones:        configuredLogicZoneList(cfg),
	})
	return nil
}

type httpRouterLoginHandler struct{}

func (h *httpRouterLoginHandler) Handle(scene *ecs.Scene, req *http.Request, resp http.ResponseWriter) error {
	if err := validateRouterHTTP(req, resp); err != nil {
		return err
	}
	if req.Method != http.MethodPost {
		moduleHttp.WriteNotFound(resp)
		return nil
	}
	if req.Body == nil {
		return moduleHttp.ErrRequestMissing
	}
	var body moduleHttp.LoginRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Username) == "" || body.Password == "" {
		moduleHttp.WriteError(resp, http.StatusOK, http.StatusBadRequest, "400 Bad Request")
		return nil
	}
	if scene == nil {
		return fmt.Errorf("router: scene missing")
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return fmt.Errorf("router: message sender missing")
	}
	sender, ok := component.(interface {
		Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
	})
	if !ok {
		return fmt.Errorf("router: message sender missing")
	}
	centralActorID, ok := actor.ResolveSceneActor(centralZoneID, ecs.SceneTypeCentral, "")
	if !ok {
		return fmt.Errorf("router: central scene missing")
	}
	payload, err := central.MarshalR2CentralAccountLogin(&central.R2CentralAccountLogin{
		RpcId:    1,
		Username: body.Username,
		Password: body.Password,
	})
	if err != nil {
		return err
	}
	respPayload, err := sender.Call(req.Context(), centralActorID, central.MsgR2CentralAccountLogin, payload)
	if err != nil {
		return err
	}
	loginResp, err := central.UnmarshalCentral2RAccountLogin(respPayload)
	if err != nil {
		return err
	}
	moduleHttp.WriteJSON(resp, http.StatusOK, moduleHttp.HttpLoginResponse{
		HttpResponse: moduleHttp.HttpResponse{
			Error:   int(loginResp.Error),
			Message: loginResp.Message,
		},
		AccessToken: loginResp.AccessToken,
	})
	return nil
}

type httpLastZoneHandler struct{}

func (h *httpLastZoneHandler) Handle(_ *ecs.Scene, req *http.Request, resp http.ResponseWriter) error {
	if err := validateRouterHTTP(req, resp); err != nil {
		return err
	}
	if req.Method != http.MethodGet {
		moduleHttp.WriteNotFound(resp)
		return nil
	}
	accessToken, ok := accessTokenQuery(req)
	if !ok {
		moduleHttp.WriteJSON(resp, http.StatusOK, lastZoneResponse{
			HttpResponse: moduleHttp.HttpResponse{
				Error:   ERR_WithException,
				Message: "param invalid",
			},
		})
		return nil
	}

	if _, err := login.VerifyAccessToken(accessToken); err != nil {
		moduleHttp.WriteJSON(resp, http.StatusOK, lastZoneResponse{
			HttpResponse: moduleHttp.HttpResponse{
				Error:   accessTokenErrorCode(err),
				Message: "token invalid",
			},
		})
		return nil
	}

	cfg := config.GetGlobal()
	if cfg == nil {
		return moduleHttp.ErrConfigurationMissing
	}
	lastZone, ok := configuredLastLogicZone(cfg)
	if !ok {
		moduleHttp.WriteJSON(resp, http.StatusOK, lastZoneResponse{
			HttpResponse: moduleHttp.HttpResponse{
				Error:   ERR_GetRouterNoZones,
				Message: "no zones",
			},
		})
		return nil
	}

	moduleHttp.WriteJSON(resp, http.StatusOK, lastZoneResponse{
		HttpResponse: moduleHttp.HttpResponse{},
		LastOne:      lastZone,
	})
	return nil
}

func validateRouterHTTP(req *http.Request, resp http.ResponseWriter) error {
	if req == nil {
		return moduleHttp.ErrRequestMissing
	}
	if resp == nil {
		return moduleHttp.ErrResponseWriterMissing
	}
	return nil
}

func configuredLogicZoneList(cfg *config.Config) []routerZoneInfo {
	if cfg == nil {
		return []routerZoneInfo{}
	}
	zones := make([]routerZoneInfo, 0)
	for _, zone := range cfg.Zones {
		if !zone.IsLogic {
			continue
		}
		zones = append(zones, routerZoneInfo{
			Id:   int32(zone.ID),
			Name: zone.Name,
		})
	}
	return zones
}

func configuredLastLogicZone(cfg *config.Config) (*routerZoneInfo, bool) {
	if cfg == nil {
		return nil, false
	}
	var lastZone *routerZoneInfo
	for _, zone := range cfg.Zones {
		if !zone.IsLogic {
			continue
		}
		lastZone = &routerZoneInfo{
			Id:     int32(zone.ID),
			Name:   zone.Name,
			Status: 1,
		}
	}
	return lastZone, lastZone != nil
}

func accessTokenQuery(req *http.Request) (string, bool) {
	if req == nil || req.URL == nil {
		return "", false
	}
	values := req.URL.Query()
	tokens, ok := values["access_token"]
	if !ok {
		return "", false
	}
	if len(tokens) == 0 {
		return "", true
	}
	return tokens[0], true
}

func accessTokenErrorCode(err error) int {
	if err == login.ErrTokenExpired {
		return int(login.ERR_TokenExpiredError)
	}
	return int(login.ERR_TokenInvalidError)
}
