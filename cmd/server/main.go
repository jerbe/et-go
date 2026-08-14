package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/db/migrations"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	_ "github.com/jerbe/et-go/engine/network/peer"
	"github.com/jerbe/et-go/internal/log"
	_ "github.com/jerbe/et-go/module/actorlocation"
	_ "github.com/jerbe/et-go/module/central"
	_ "github.com/jerbe/et-go/module/http"
	_ "github.com/jerbe/et-go/module/lockstep"
	loginmodule "github.com/jerbe/et-go/module/login"
	_ "github.com/jerbe/et-go/module/map_"
	_ "github.com/jerbe/et-go/module/router"
)

var (
	processID int
	configDir string
	logLevel  string
)

func init() {
	flag.IntVar(&processID, "process", 1, "进程 ID")
	flag.StringVar(&configDir, "config", "data/config/json", "配置文件目录")
	flag.StringVar(&logLevel, "log-level", "info", "日志级别: debug, info, warn, error")
}

func main() {
	flag.Parse()

	logger := log.New(logLevel)
	logger.Info("ET-Go 服务器启动中...", "process", processID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime, err := bootstrapServer(ctx, logger, processID, configDir)
	if err != nil {
		logger.Error("ET-Go 服务器启动失败", "process", processID, "err", err)
		os.Exit(1)
	}
	defer runtime.Shutdown()

	logger.Info("ET-Go 服务器启动完成", "process", processID, "fibers", runtime.fiberManager.Count())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	sig := <-quit
	fmt.Printf("\n收到信号 %v，服务器关闭中...\n", sig)
	cancel()
}

type serverRuntime struct {
	world               *ecs.World
	fiberManager        *fiber.Manager
	revocationDBManager *db.DBManagerComponent
}

func (r *serverRuntime) Shutdown() {
	if r == nil {
		return
	}
	if r.fiberManager != nil {
		r.fiberManager.StopAll()
	}
	if r.world != nil {
		r.world.Shutdown()
	}
	if r.revocationDBManager != nil {
		r.revocationDBManager.OnDestroy()
	}
}

func bootstrapServer(ctx context.Context, logger *log.Logger, processID int, configDir string) (*serverRuntime, error) {
	cfg, err := config.Load(configDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(processID); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}
	topology, err := cfg.RuntimeTopology(processID)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime topology: %w", err)
	}
	logRuntimeTopology(logger, topology)
	if err := runConfiguredMigrations(ctx, cfg, processID, logger); err != nil {
		return nil, err
	}
	revocationDBManager, revocationStore, err := buildAccessTokenRevocationStore(cfg, processID)
	if err != nil {
		return nil, err
	}
	if err := configureAccessTokenWithRevocation(cfg, processID, revocationStore, revocationStore != nil); err != nil {
		if revocationDBManager != nil {
			revocationDBManager.OnDestroy()
		}
		return nil, err
	}
	config.SetGlobal(cfg)

	world := ecs.NewWorld()
	fiberManager := fiber.NewManager(ctx, world, logger)

	runtime := &serverRuntime{
		world:               world,
		fiberManager:        fiberManager,
		revocationDBManager: revocationDBManager,
	}

	created, err := createConfiguredFibers(fiberManager, cfg, processID, logger)
	if err != nil {
		runtime.Shutdown()
		return nil, err
	}
	if err := configureMapTargets(created, cfg, processID); err != nil {
		runtime.Shutdown()
		return nil, err
	}

	return runtime, nil
}

func logRuntimeTopology(logger *log.Logger, topology config.RuntimeTopology) {
	if logger == nil {
		return
	}
	logger.Info(
		"启动拓扑摘要",
		"process", topology.ProcessID,
		"machine", topology.MachineID,
		"machine_inner_ip", topology.MachineInnerIP,
		"machine_outer_ip", topology.MachineOuterIP,
		"inner_port", topology.InnerPort,
		"implicit_net_inner", topology.ImplicitNetInner,
		"peer_count", len(topology.Peers),
		"scene_count", len(topology.Scenes),
	)
	for _, peer := range topology.Peers {
		logger.Info(
			"启动拓扑 peer",
			"process", topology.ProcessID,
			"peer_process", peer.ProcessID,
			"address", peer.Address,
		)
	}
	for _, scene := range topology.Scenes {
		logger.Info(
			"启动拓扑 scene",
			"scene_id", scene.ID,
			"scene_name", scene.Name,
			"scene_type", scene.SceneType,
			"zone", scene.Zone,
			"outer_port", scene.OuterPort,
		)
	}
}

func configureAccessToken(cfg *config.Config, processID int) error {
	return configureAccessTokenWithRevocation(cfg, processID, nil, false)
}

func configureAccessTokenWithRevocation(
	cfg *config.Config,
	processID int,
	revocationStore loginmodule.AccessTokenRevocationStore,
	requireRevocation bool,
) error {
	if cfg == nil {
		return errors.New("security config is nil")
	}
	if !processUsesAccessToken(cfg, processID) {
		return nil
	}
	if processUsesHTTP(cfg, processID) && len(cfg.Security.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("configure access token: HTTP process requires security corsAllowedOrigins")
	}
	keys := make([]loginmodule.AccessTokenKey, 0, len(cfg.Security.AccessTokenKeys))
	for _, key := range cfg.Security.AccessTokenKeys {
		keys = append(keys, loginmodule.AccessTokenKey{
			ID:     key.ID,
			Secret: key.Secret,
		})
	}
	if err := loginmodule.ConfigureAccessToken(loginmodule.AccessTokenConfig{
		CurrentKeyID:      cfg.Security.AccessTokenCurrentKeyID,
		Keys:              keys,
		LegacyKey:         cfg.Security.LegacyTokenKey,
		AllowLegacy:       cfg.Security.AllowLegacyTokens,
		GenerateLegacy:    strings.EqualFold(strings.TrimSpace(cfg.Security.AccessTokenFormat), "legacy"),
		ExpireDuration:    0,
		RevocationStore:   revocationStore,
		RequireRevocation: requireRevocation,
	}); err != nil {
		return fmt.Errorf("configure access token: %w", err)
	}
	return nil
}

func buildAccessTokenRevocationStore(
	cfg *config.Config,
	processID int,
) (*db.DBManagerComponent, loginmodule.AccessTokenRevocationStore, error) {
	if cfg == nil || !processUsesAccessToken(cfg, processID) {
		return nil, nil, nil
	}
	const centralZoneID = 1
	foundCentralZone := false
	for _, zone := range cfg.Zones {
		if zone.ID == centralZoneID {
			foundCentralZone = true
			break
		}
	}
	if !foundCentralZone {
		return nil, nil, fmt.Errorf("configure access token: central zone %d is not configured", centralZoneID)
	}
	manager := &db.DBManagerComponent{}
	manager.SetConfig(cfg)
	store, err := loginmodule.NewDBManagerAccessTokenRevocationStore(manager, centralZoneID)
	if err != nil {
		manager.OnDestroy()
		return nil, nil, fmt.Errorf("configure access token revocation store: %w", err)
	}
	return manager, store, nil
}

func processUsesHTTP(cfg *config.Config, processID int) bool {
	if cfg == nil {
		return false
	}
	for _, scene := range cfg.Scenes {
		if scene.ProcessID == processID &&
			strings.EqualFold(strings.TrimSpace(scene.SceneType), "http") {
			return true
		}
	}
	return false
}

func processUsesAccessToken(cfg *config.Config, processID int) bool {
	if cfg == nil {
		return false
	}
	for _, scene := range cfg.Scenes {
		if scene.ProcessID != processID {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(scene.SceneType)) {
		case "central", "realm", "http", "router":
			return true
		}
	}
	return false
}

func runConfiguredMigrations(ctx context.Context, cfg *config.Config, processID int, logger *log.Logger) error {
	if cfg == nil {
		return errors.New("migration config is nil")
	}
	if !processUsesDatabase(cfg, processID) && !processUsesAccessToken(cfg, processID) {
		return nil
	}

	for _, zoneID := range configuredMigrationZoneIDs(cfg, processID) {
		var zoneCfg *config.StartZoneConfig
		for index := range cfg.Zones {
			if cfg.Zones[index].ID == zoneID {
				zoneCfg = &cfg.Zones[index]
				break
			}
		}
		if zoneCfg == nil {
			return fmt.Errorf("migration zone config missing: %d", zoneID)
		}

		client, err := db.New(ctx, zoneCfg.DBAddr, zoneCfg.DBName)
		if err != nil {
			return fmt.Errorf("run migrations for zone %d: connect database: %w", zoneID, err)
		}
		err = db.RunMigrations(ctx, client, migrations.All())
		closeErr := client.Close(ctx)
		if err != nil {
			return fmt.Errorf("run migrations for zone %d: %w", zoneID, err)
		}
		if closeErr != nil {
			return fmt.Errorf("run migrations for zone %d: close database: %w", zoneID, closeErr)
		}
		if logger != nil {
			logger.Info("数据库 migration 已完成", "zone", zoneID)
		}
	}
	return nil
}

func configuredMigrationZoneIDs(cfg *config.Config, processID int) []int {
	if cfg == nil || processID <= 0 {
		return nil
	}
	zoneIDs := make(map[int]struct{})
	for _, scene := range cfg.Scenes {
		if scene.ProcessID == processID {
			zoneIDs[scene.Zone] = struct{}{}
		}
	}
	if processUsesAccessToken(cfg, processID) {
		zoneIDs[1] = struct{}{}
	}
	result := make([]int, 0, len(zoneIDs))
	for zoneID := range zoneIDs {
		result = append(result, zoneID)
	}
	sort.Ints(result)
	return result
}

func processUsesDatabase(cfg *config.Config, processID int) bool {
	for _, scene := range cfg.Scenes {
		if scene.ProcessID != processID {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(scene.SceneType)) {
		case "central", "realm", "map", "http", "router":
			return true
		}
	}
	return false
}

func createConfiguredFibers(manager *fiber.Manager, cfg *config.Config, processID int, logger *log.Logger) ([]*fiber.Fiber, error) {
	if manager == nil {
		return nil, errors.New("fiber manager is nil")
	}
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	created := make([]*fiber.Fiber, 0)
	handler := makeSceneMessageHandler(logger)

	var currentProcess config.StartProcessConfig
	processConfigured := false
	for _, processCfg := range cfg.Processes {
		if processCfg.ID == processID {
			currentProcess = processCfg
			processConfigured = true
			break
		}
	}
	if len(cfg.Processes) > 0 && !processConfigured {
		return nil, fmt.Errorf("process %d is not configured", processID)
	}

	hasNetInner := false
	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.ProcessID == processID &&
			strings.EqualFold(strings.TrimSpace(sceneCfg.SceneType), "netinner") {
			hasNetInner = true
			break
		}
	}
	if len(currentProcess.Peers) > 0 && !hasNetInner {
		current := manager.Create(ecs.SceneTypeNetInner, 0, processID, handler)
		if current == nil {
			return nil, fmt.Errorf("create implicit NetInner fiber failed for process %d", processID)
		}
		actor.UpdateSceneRegistry(current.Root())
		created = append(created, current)
		if logger != nil {
			logger.Info(
				"隐式 NetInner Fiber 已创建",
				"process", processID,
				"fiber_id", current.ID(),
				"inner_port", currentProcess.InnerPort,
			)
		}
	}

	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.ProcessID != processID {
			continue
		}

		sceneType, ok := parseSceneType(sceneCfg.SceneType)
		if !ok {
			return nil, fmt.Errorf("unsupported scene type %q for scene %d", sceneCfg.SceneType, sceneCfg.ID)
		}

		current := manager.CreateConfigured(sceneType, sceneCfg.Zone, processID, int64(sceneCfg.ID), sceneCfg.Name, handler)
		if current == nil {
			return nil, fmt.Errorf("create fiber failed for scene %d (%s)", sceneCfg.ID, sceneCfg.SceneType)
		}
		actor.UpdateSceneRegistry(current.Root())
		created = append(created, current)

		if logger != nil {
			logger.Info(
				"场景 Fiber 已创建",
				"scene_id", sceneCfg.ID,
				"scene_name", sceneCfg.Name,
				"scene_type", sceneCfg.SceneType,
				"zone", sceneCfg.Zone,
				"fiber_id", current.ID(),
			)
		}
	}

	if len(created) == 0 {
		return nil, fmt.Errorf("process %d has no scene configuration", processID)
	}

	return created, nil
}

func configureMapTargets(created []*fiber.Fiber, cfg *config.Config, processID int) error {
	if cfg == nil {
		return errors.New("map target config is nil")
	}
	byName := make(map[string]*fiber.Fiber)
	for _, current := range created {
		if current == nil || current.SceneType() != ecs.SceneTypeMap {
			continue
		}
		byName[strings.ToLower(strings.TrimSpace(current.Root().Name()))] = current
	}
	for _, sceneCfg := range cfg.Scenes {
		if sceneCfg.ProcessID != processID || !strings.EqualFold(strings.TrimSpace(sceneCfg.SceneType), "map") {
			continue
		}
		if len(sceneCfg.MapTargets) == 0 {
			continue
		}
		current, ok := byName[strings.ToLower(strings.TrimSpace(sceneCfg.Name))]
		if !ok {
			return fmt.Errorf("map scene %q was not created", sceneCfg.Name)
		}
		component, ok := current.Root().GetComponent("MapUnitManagerComponent")
		if !ok || component == nil {
			return fmt.Errorf("map scene %q missing MapUnitManagerComponent", sceneCfg.Name)
		}
		manager, ok := component.(interface {
			SetTarget(string, actor.ActorID) error
		})
		if !ok {
			return fmt.Errorf("map scene %q has invalid MapUnitManagerComponent", sceneCfg.Name)
		}
		for _, targetName := range sceneCfg.MapTargets {
			target, ok := byName[strings.ToLower(strings.TrimSpace(targetName))]
			if !ok {
				return fmt.Errorf("map scene %q target %q was not created", sceneCfg.Name, targetName)
			}
			if err := manager.SetTarget(target.Root().Name(), actor.SceneActorID(target.Root())); err != nil {
				return fmt.Errorf("map scene %q target %q invalid: %w", sceneCfg.Name, targetName, err)
			}
		}
	}
	return nil
}

func makeSceneMessageHandler(logger *log.Logger) fiber.MessageHandler {
	return func(f *fiber.Fiber, msg fiber.Message) {
		if f == nil {
			return
		}
		responsePayload, err := actor.DispatchFiberMessage(f.Root(), msg)
		if msg.Reply != nil {
			msg.Reply <- fiber.MessageResponse{
				Payload: responsePayload,
				Err:     err,
			}
		}
		if err != nil && logger != nil {
			logger.Warn(
				"Fiber 消息分发失败",
				"fiber_id", f.ID(),
				"scene_type", f.SceneType().String(),
				"to", msg.To,
				"msg_id", msg.MsgID,
				"err", err,
			)
		}
	}
}

func parseSceneType(raw string) (ecs.SceneType, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "main":
		return ecs.SceneTypeMain, true
	case "launch":
		return ecs.SceneTypeLaunch, true
	case "netinner":
		return ecs.SceneTypeNetInner, true
	case "netclient":
		return ecs.SceneTypeNetClient, true
	case "location":
		return ecs.SceneTypeLocation, true
	case "router":
		return ecs.SceneTypeRouter, true
	case "routernode":
		return ecs.SceneTypeRouterNode, true
	case "realm":
		return ecs.SceneTypeRealm, true
	case "gate":
		return ecs.SceneTypeGate, true
	case "lockstep":
		return ecs.SceneTypeLockStep, true
	case "match":
		return ecs.SceneTypeMatch, true
	case "room":
		return ecs.SceneTypeRoom, true
	case "http":
		return ecs.SceneTypeHTTP, true
	case "map":
		return ecs.SceneTypeMap, true
	case "central":
		return ecs.SceneTypeCentral, true
	default:
		return ecs.SceneTypeNone, false
	}
}
