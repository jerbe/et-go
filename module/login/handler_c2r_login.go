package login

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/module/actorlocation"
)

var realmCloseDelay = time.Second

const loginAccountLockTimeout = 60_000

// HandleC2RLogin 处理 Realm 登录请求。
func HandleC2RLogin(scene *ecs.Scene, session *network.Session, req *C2RLogin) (response *R2CLogin, err error) {
	if req == nil {
		return nil, ErrInvalidLoginRequest
	}

	accountID, err := VerifyAccessToken(req.AccessToken)
	if err != nil {
		closeRealmSessionLater(session)
		return &R2CLogin{
			RpcId:   req.RpcId,
			Error:   tokenErrorCode(err),
			Message: "token invalid",
		}, nil
	}

	lockCleanup, err := acquireAccountLoginLock(scene, accountID)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if lockCleanup != nil && !success {
			if cleanupErr := lockCleanup(); err == nil {
				err = cleanupErr
			}
		}
	}()

	gates, err := gatesFromScene(scene, req.ZoneId)
	if err != nil {
		return nil, err
	}
	endpoint := gates[int(accountID%int64(len(gates)))]

	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return nil, ErrMessageSenderMissing
	}
	sender, ok := component.(interface {
		Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
	})
	if !ok {
		return nil, ErrMessageSenderMissing
	}

	payload, err := marshalR2GGateAssign(&R2GGateAssign{
		RpcId:     req.RpcId,
		AccountId: accountID,
	})
	if err != nil {
		return nil, err
	}
	respPayload, err := sender.Call(sessionContext(session), endpoint.ActorID, MsgR2GGateAssign, payload)
	if err != nil {
		return nil, err
	}

	gateResp, err := unmarshalG2RGateAssign(respPayload)
	if err != nil {
		return nil, err
	}
	if gateResp.Error != 0 {
		return &R2CLogin{
			RpcId:   req.RpcId,
			Error:   gateResp.Error,
			Message: gateResp.Message,
		}, nil
	}
	if strings.TrimSpace(gateResp.Token) == "" {
		return nil, ErrGateAssignmentInvalid
	}
	if gateResp.GateId > 0 && gateResp.GateId != endpoint.GateId {
		return nil, ErrGateAssignmentInvalid
	}
	success = true

	closeRealmSessionLater(session)

	gateID := gateResp.GateId
	if gateID == 0 {
		gateID = endpoint.GateId
	}

	return &R2CLogin{
		RpcId:   req.RpcId,
		Address: endpoint.Address,
		GateId:  gateID,
		Token:   gateResp.Token,
	}, nil
}

func sessionContext(session *network.Session) context.Context {
	if session == nil {
		return context.Background()
	}
	return session.Context()
}

type accountLocationProxy interface {
	Lock(locationType int, key int64, actorID actor.ActorID, timeMs int) error
	Unlock(locationType int, key int64, oldActorID, newActorID actor.ActorID) error
}

func tokenErrorCode(err error) int32 {
	switch err {
	case ErrTokenExpired:
		return ERR_TokenExpiredError
	default:
		return ERR_TokenInvalidError
	}
}

func closeRealmSessionLater(session *network.Session) {
	if session == nil || realmCloseDelay <= 0 {
		return
	}
	time.AfterFunc(realmCloseDelay, session.Close)
}

func acquireAccountLoginLock(scene *ecs.Scene, accountID int64) (func() error, error) {
	if scene == nil {
		return nil, ErrLocationProxyMissing
	}
	proxy := locationProxyFromScene(scene)
	if proxy == nil {
		return nil, ErrLocationProxyMissing
	}
	lockActorID := actor.SceneActorID(scene)
	if !lockActorID.IsValid() {
		return nil, ErrLocationProxyMissing
	}
	if err := proxy.Lock(int(actorlocation.LocationTypeAccount), accountID, lockActorID, loginAccountLockTimeout); err != nil {
		return nil, err
	}
	return func() error {
		return proxy.Unlock(int(actorlocation.LocationTypeAccount), accountID, lockActorID, lockActorID)
	}, nil
}

func gatesFromScene(scene *ecs.Scene, zoneID int32) ([]GateEndpoint, error) {
	if scene == nil {
		return nil, ErrGateRegistryMissing
	}
	if zoneID == 0 {
		zoneID = int32(scene.Zone())
	}

	registry := gateRegistryFromScene(scene)
	if registry != nil {
		gates := registry.GetGates(zoneID)
		if len(gates) > 0 {
			if err := validateGateEndpoints(gates); err != nil {
				return nil, err
			}
			return gates, nil
		}
	}

	gates, err := resolveConfiguredGates(int(zoneID))
	if err != nil {
		return nil, err
	}
	if len(gates) == 0 {
		return nil, ErrGateRegistryMissing
	}
	if registry != nil {
		registry.SetGates(zoneID, gates)
	}
	return gates, nil
}

func gateRegistryFromScene(scene *ecs.Scene) *GateRegistryComponent {
	if scene == nil {
		return nil
	}
	component, ok := scene.GetComponent("GateRegistryComponent")
	if !ok || component == nil {
		return nil
	}
	registry, ok := component.(*GateRegistryComponent)
	if !ok {
		return nil
	}
	return registry
}

func locationProxyFromScene(scene *ecs.Scene) accountLocationProxy {
	if scene == nil {
		return nil
	}
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return nil
	}
	proxy, ok := component.(accountLocationProxy)
	if !ok {
		return nil
	}
	return proxy
}

func resolveConfiguredGates(zone int) ([]GateEndpoint, error) {
	refs := actor.ResolveSceneActors(zone, ecs.SceneTypeGate)
	if len(refs) == 0 {
		return nil, ErrGateRegistryMissing
	}

	cfg := config.GetGlobal()
	if cfg == nil {
		return nil, fmt.Errorf("login: configuration missing")
	}

	gates := make([]GateEndpoint, 0)
	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.Zone != zone || !strings.EqualFold(strings.TrimSpace(sceneCfg.SceneType), ecs.SceneTypeGate.String()) {
			continue
		}
		ref, ok := matchRuntimeScene(refs, int64(sceneCfg.ID), sceneCfg.Name)
		if !ok {
			continue
		}
		address, err := resolveConfiguredSceneAddr(cfg, sceneCfg, true)
		if err != nil {
			return nil, err
		}
		gates = append(gates, GateEndpoint{
			GateId:  int64(sceneCfg.ID),
			Address: address,
			ActorID: ref.ActorID,
		})
	}
	if len(gates) == 0 {
		return nil, ErrGateRegistryMissing
	}
	if err := validateGateEndpoints(gates); err != nil {
		return nil, err
	}
	return gates, nil
}

func matchRuntimeScene(refs []actor.SceneRef, sceneID int64, name string) (actor.SceneRef, bool) {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	for _, ref := range refs {
		if sceneID > 0 && ref.SceneID == sceneID {
			return ref, true
		}
		if lowerName != "" && strings.ToLower(strings.TrimSpace(ref.Name)) == lowerName {
			return ref, true
		}
	}
	return actor.SceneRef{}, false
}

func resolveConfiguredSceneAddr(cfg *config.Config, sceneCfg config.StartSceneConfig, preferInner bool) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("login: configuration missing")
	}
	var machine *config.StartMachineConfig
	for processIndex := range cfg.Processes {
		process := cfg.Processes[processIndex]
		if process.ID != sceneCfg.ProcessID {
			continue
		}
		for machineIndex := range cfg.Machines {
			if cfg.Machines[machineIndex].ID == process.MachineID {
				machine = &cfg.Machines[machineIndex]
				break
			}
		}
		break
	}
	if machine == nil {
		return "", fmt.Errorf("login: process %d machine configuration missing", sceneCfg.ProcessID)
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
		return "", fmt.Errorf("login: Gate scene %d machine address missing", sceneCfg.ID)
	}

	port := sceneCfg.OuterPort
	for _, process := range cfg.Processes {
		if process.ID != sceneCfg.ProcessID {
			continue
		}
		if port <= 0 {
			port = process.InnerPort
		}
		break
	}
	if port <= 0 {
		return "", fmt.Errorf("login: Gate scene %d advertised port missing", sceneCfg.ID)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func validateGateEndpoints(gates []GateEndpoint) error {
	if len(gates) == 0 {
		return ErrGateRegistryMissing
	}
	for _, gate := range gates {
		if gate.GateId <= 0 || !gate.ActorID.IsValid() || strings.TrimSpace(gate.Address) == "" {
			return fmt.Errorf("login: invalid Gate endpoint")
		}
	}
	return nil
}
