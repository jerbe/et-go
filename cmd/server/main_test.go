package main

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	etlog "github.com/jerbe/et-go/internal/log"
	httpmodule "github.com/jerbe/et-go/module/http"
	loginmodule "github.com/jerbe/et-go/module/login"
	mapmodule "github.com/jerbe/et-go/module/map_"
)

func TestParseSceneType(t *testing.T) {
	tests := []struct {
		input string
		want  ecs.SceneType
		ok    bool
	}{
		{input: "Central", want: ecs.SceneTypeCentral, ok: true},
		{input: "routernode", want: ecs.SceneTypeRouterNode, ok: true},
		{input: " HTTP ", want: ecs.SceneTypeHTTP, ok: true},
		{input: "unknown", want: ecs.SceneTypeNone, ok: false},
	}

	for _, tt := range tests {
		got, ok := parseSceneType(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("parseSceneType(%q) = (%v, %v), want (%v, %v)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestLogRuntimeTopologyDoesNotPanicWithoutLogger(t *testing.T) {
	logRuntimeTopology(nil, config.RuntimeTopology{ProcessID: 1})
}

func TestCreateConfiguredFibers(t *testing.T) {
	logger := etlog.New("error")
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, logger)
	defer manager.StopAll()
	defer world.Shutdown()

	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 3, SceneType: "Central", Name: "central-1"},
			{ID: 2, ProcessID: 2, Zone: 9, SceneType: "Map", Name: "map-2"},
		},
	}
	config.SetGlobal(cfg)

	created, err := createConfiguredFibers(manager, cfg, 1, logger)
	if err != nil {
		t.Fatalf("createConfiguredFibers err = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created fibers = %d, want 1", len(created))
	}
	if manager.Count() != 1 {
		t.Fatalf("manager fiber count = %d, want 1", manager.Count())
	}
	if created[0].SceneType() != ecs.SceneTypeCentral {
		t.Fatalf("scene type = %v, want %v", created[0].SceneType(), ecs.SceneTypeCentral)
	}
	if created[0].Zone() != 3 {
		t.Fatalf("zone = %d, want 3", created[0].Zone())
	}
	if _, ok := created[0].Root().GetComponent("MailBox"); !ok {
		t.Fatal("central fiber should install mailbox through fiber init")
	}
}

func TestCreateConfiguredFibersAddsImplicitNetInnerForPeers(t *testing.T) {
	logger := etlog.New("error")
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, logger)
	defer manager.StopAll()
	defer world.Shutdown()

	port := reserveServerTestUDPPort(t)
	cfg := &config.Config{
		Machines: []config.StartMachineConfig{
			{ID: 1, InnerIP: "127.0.0.1"},
			{ID: 2, InnerIP: "127.0.0.1"},
		},
		Processes: []config.StartProcessConfig{
			{
				ID:        1,
				MachineID: 1,
				InnerPort: port,
				Peers: []config.StartProcessPeerConfig{
					{ProcessID: 2, Address: "127.0.0.1:1", Secret: "shared-secret"},
				},
			},
			{ID: 2, MachineID: 2, InnerPort: 10002},
		},
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Main", Name: "main"},
		},
	}
	config.SetGlobal(cfg)
	defer config.SetGlobal(nil)

	created, err := createConfiguredFibers(manager, cfg, 1, logger)
	if err != nil {
		t.Fatalf("createConfiguredFibers error = %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created fibers = %d, want implicit NetInner plus Main", len(created))
	}
	if _, ok := created[0].Root().GetComponent("ProcessPeerComponent"); !ok {
		t.Fatal("first fiber should be implicit NetInner with ProcessPeerComponent")
	}
}

func TestConfiguredMigrationZoneIDsIncludesCentralTokenZone(t *testing.T) {
	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 2, SceneType: "Realm", Name: "Realm"},
		},
	}
	got := configuredMigrationZoneIDs(cfg, 1)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("migration zones = %v, want [1 2]", got)
	}
}

func TestCreateConfiguredFibersRejectsUnknownScene(t *testing.T) {
	logger := etlog.New("error")
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, logger)
	defer manager.StopAll()
	defer world.Shutdown()

	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 99, ProcessID: 1, Zone: 1, SceneType: "UnknownScene"},
		},
	}

	if _, err := createConfiguredFibers(manager, cfg, 1, logger); err == nil {
		t.Fatal("expected error for unknown scene type")
	}
}

func TestCreateConfiguredHTTPFiberInstallsSharedSecurityComponents(t *testing.T) {
	logger := etlog.New("error")
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, logger)
	defer manager.StopAll()
	defer world.Shutdown()

	port := reserveServerTestTCPPort(t)
	cfg := &config.Config{
		Machines: []config.StartMachineConfig{
			{ID: 1, InnerIP: "127.0.0.1"},
		},
		Processes: []config.StartProcessConfig{
			{ID: 1, MachineID: 1},
		},
		Scenes: []config.StartSceneConfig{
			{ID: 9001, ProcessID: 1, Zone: 1, SceneType: "HTTP", Name: "http", OuterPort: port},
		},
		Zones: []config.StartZoneConfig{
			{ID: 1, Name: "central", DBAddr: "mongodb://127.0.0.1:27017", DBName: "etgo"},
		},
		Security: config.StartSecurityConfig{
			AccessTokenCurrentKeyID: "primary",
			AccessTokenKeys: []config.StartAccessTokenKeyConfig{
				{ID: "primary", Secret: "01234567890123456789012345678901"},
			},
			CORSAllowedOrigins:      []string{"https://game.example"},
			LoginRateLimitPerMinute: 1,
		},
	}
	old := config.GetGlobal()
	config.SetGlobal(cfg)
	defer config.SetGlobal(old)

	created, err := createConfiguredFibers(manager, cfg, 1, logger)
	if err != nil {
		t.Fatalf("createConfiguredFibers error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created fibers = %d, want 1", len(created))
	}
	scene := created[0].Root()
	if component, ok := scene.GetComponent("LoginAuditComponent"); !ok || component == nil {
		t.Fatal("HTTP fiber should install LoginAuditComponent")
	}
	component, ok := scene.GetComponent("LoginRateLimiterComponent")
	if !ok || component == nil {
		t.Fatal("HTTP fiber should install shared LoginRateLimiterComponent")
	}
	if _, ok := component.(*httpmodule.DBManagerLoginRateLimiterComponent); !ok {
		t.Fatalf("LoginRateLimiterComponent type = %T, want DBManagerLoginRateLimiterComponent", component)
	}
}

func reserveServerTestUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("reserve UDP port error = %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatalf("release UDP port error = %v", err)
	}
	return port
}

func reserveServerTestTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP port error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release TCP port error = %v", err)
	}
	return port
}

func TestConfigureMapTargets(t *testing.T) {
	logger := etlog.New("error")
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, logger)
	defer manager.StopAll()
	defer world.Shutdown()

	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1001, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Map1", MapTargets: []string{"Map2"}},
			{ID: 1002, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Map2"},
		},
	}
	created, err := createConfiguredFibers(manager, cfg, 1, logger)
	if err != nil {
		t.Fatalf("create map fibers error = %v", err)
	}
	if err := configureMapTargets(created, cfg, 1); err != nil {
		t.Fatalf("configureMapTargets error = %v", err)
	}

	var source, target *fiber.Fiber
	for _, current := range created {
		switch current.Root().Name() {
		case "Map1":
			source = current
		case "Map2":
			target = current
		}
	}
	if source == nil || target == nil {
		t.Fatal("expected both map fibers")
	}
	component, ok := source.Root().GetComponent("MapUnitManagerComponent")
	if !ok {
		t.Fatal("source map manager missing")
	}
	mapManager, ok := component.(*mapmodule.MapUnitManagerComponent)
	if !ok {
		t.Fatal("source map manager type mismatch")
	}
	got, ok := mapManager.ResolveTarget("Map2")
	if !ok || got != actor.SceneActorID(target.Root()) {
		t.Fatalf("target actor = %v, ok=%v, want %v", got, ok, actor.SceneActorID(target.Root()))
	}
}

func TestConfigureAccessTokenRequiresHTTPSecurityConfig(t *testing.T) {
	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "HTTP", Name: "http"},
		},
	}
	if err := configureAccessToken(cfg, 1); err == nil ||
		!strings.Contains(err.Error(), "corsAllowedOrigins") {
		t.Fatalf("configureAccessToken error = %v, want HTTP CORS configuration error", err)
	}
}

func TestConfigureAccessTokenInstallsSignedKeyRing(t *testing.T) {
	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "realm"},
		},
		Security: config.StartSecurityConfig{
			AccessTokenCurrentKeyID: "primary",
			AccessTokenKeys: []config.StartAccessTokenKeyConfig{
				{ID: "primary", Secret: "01234567890123456789012345678901"},
			},
		},
	}
	defer loginmodule.ConfigureLegacyAccessTokenForTests()

	if err := configureAccessToken(cfg, 1); err != nil {
		t.Fatalf("configureAccessToken error = %v", err)
	}
	token, err := loginmodule.GenerateAccessToken(1001)
	if err != nil {
		t.Fatalf("GenerateAccessToken error = %v", err)
	}
	if !strings.HasPrefix(token, "v2.primary.") {
		t.Fatalf("token = %q, want signed primary token", token)
	}
}

func TestConfigureAccessTokenInstallsExplicitLegacyFormat(t *testing.T) {
	cfg := &config.Config{
		Scenes: []config.StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "realm"},
		},
		Security: config.StartSecurityConfig{
			AccessTokenFormat: "legacy",
			LegacyTokenKey:    "whosyourdaddy",
			AllowLegacyTokens: true,
		},
	}
	defer loginmodule.ConfigureLegacyAccessTokenForTests()

	if err := configureAccessToken(cfg, 1); err != nil {
		t.Fatalf("configureAccessToken legacy error = %v", err)
	}
	token, err := loginmodule.GenerateAccessToken(1001)
	if err != nil {
		t.Fatalf("GenerateAccessToken legacy error = %v", err)
	}
	if strings.HasPrefix(token, "v2.") {
		t.Fatalf("token = %q, want explicit legacy format", token)
	}
	if accountID, err := loginmodule.VerifyAccessToken(token); err != nil || accountID != 1001 {
		t.Fatalf("VerifyAccessToken legacy = (%d, %v), want (1001, nil)", accountID, err)
	}
}
