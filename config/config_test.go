package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsMissingRequiredFile(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, "startmachineconfig.json", `[{"id":1,"innerIP":"127.0.0.1"}]`)

	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "startprocessconfig.json") {
		t.Fatalf("Load error = %v, want missing required process config", err)
	}
}

func TestLoadAndValidate(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, "startmachineconfig.json", `[{"id":1,"innerIP":"127.0.0.1","outerIP":"127.0.0.1"}]`)
	writeConfigFile(t, dir, "startprocessconfig.json", `[{"id":1,"machineId":1,"innerPort":10001}]`)
	writeConfigFile(t, dir, "startsceneconfig.json", `[{"id":1001,"processId":1,"zone":1,"sceneType":"Map","name":"Map1","outerPort":10002}]`)
	writeConfigFile(t, dir, "startzoneconfig.json", `[{"id":1,"name":"dev","dbName":"etgo","dbAddr":"mongodb://127.0.0.1:27017"}]`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestValidateRejectsMissingProcessScene(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Zones:     []StartZoneConfig{{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"}},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "no scene") {
		t.Fatalf("Validate error = %v, want no scene error", err)
	}
}

func TestValidateRejectsUnsupportedSceneType(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 99, ProcessID: 1, Zone: 1, SceneType: "UnknownScene", Name: "unknown"},
		},
		Zones: []StartZoneConfig{{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"}},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "unsupported sceneType") {
		t.Fatalf("Validate error = %v, want unsupported scene type error", err)
	}
}

func TestValidateMapTargets(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 1001, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Map1", MapTargets: []string{"Map2"}},
			{ID: 1002, ProcessID: 1, Zone: 1, SceneType: "Map", Name: "Map2"},
		},
		Zones: []StartZoneConfig{{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"}},
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate map targets error = %v", err)
	}

	cfg.Scenes[0].MapTargets = []string{"Map2", "Map2"}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "multiple targets") {
		t.Fatalf("Validate multiple map targets error = %v", err)
	}

	cfg.Scenes[0].MapTargets = []string{"Missing"}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "missing target") {
		t.Fatalf("Validate missing map target error = %v", err)
	}
}

func TestValidateRejectsNetworkSceneWithoutListenPort(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 9004, ProcessID: 1, Zone: 1, SceneType: "Gate", Name: "Gate"},
		},
		Zones: []StartZoneConfig{{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"}},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "no listen port") {
		t.Fatalf("Validate error = %v, want missing listen port error", err)
	}
}

func TestValidateRejectsNetInnerWithoutProcessPort(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 9005, ProcessID: 1, Zone: 1, SceneType: "NetInner", Name: "NetInner"},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "NetInner requires positive") {
		t.Fatalf("Validate error = %v, want NetInner inner port error", err)
	}
}

func TestValidateAcceptsProcessListenPortFallback(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1, InnerPort: 10001}},
		Scenes: []StartSceneConfig{
			{ID: 9003, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "Realm"},
		},
		Zones: []StartZoneConfig{{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"}},
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate error = %v, want process port fallback accepted", err)
	}
}

func TestValidateRejectsOuterPortFallbackWhenProcessHasPeers(t *testing.T) {
	cfg := &Config{
		Machines: []StartMachineConfig{
			{ID: 1, InnerIP: "127.0.0.1"},
			{ID: 2, InnerIP: "127.0.0.1"},
		},
		Processes: []StartProcessConfig{
			{
				ID:        1,
				MachineID: 1,
				InnerPort: 10001,
				Peers: []StartProcessPeerConfig{
					{ProcessID: 2, Address: "127.0.0.1:10002", Secret: "shared-secret"},
				},
			},
			{
				ID:        2,
				MachineID: 2,
				InnerPort: 10002,
				Peers: []StartProcessPeerConfig{
					{ProcessID: 1, Address: "127.0.0.1:10001", Secret: "shared-secret"},
				},
			},
		},
		Scenes: []StartSceneConfig{
			{ID: 9004, ProcessID: 1, Zone: 1, SceneType: "Gate", Name: "Gate", OuterPort: 0},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "must define outerPort") {
		t.Fatalf("Validate error = %v, want explicit outer port requirement", err)
	}
}

func TestValidatePeerTopology(t *testing.T) {
	newConfig := func() *Config {
		return &Config{
			Machines: []StartMachineConfig{
				{ID: 1, InnerIP: "127.0.0.1"},
				{ID: 2, InnerIP: "127.0.0.1"},
			},
			Processes: []StartProcessConfig{
				{
					ID:        1,
					MachineID: 1,
					InnerPort: 10001,
					Peers: []StartProcessPeerConfig{
						{ProcessID: 2, Address: "127.0.0.1:10002", Secret: "shared-secret"},
					},
				},
				{
					ID:        2,
					MachineID: 2,
					InnerPort: 10002,
					Peers: []StartProcessPeerConfig{
						{ProcessID: 1, Address: "127.0.0.1:10001", Secret: "shared-secret"},
					},
				},
			},
			Scenes: []StartSceneConfig{
				{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Main", Name: "main"},
			},
			Zones: []StartZoneConfig{
				{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
			},
		}
	}

	t.Run("valid", func(t *testing.T) {
		if err := newConfig().Validate(1); err != nil {
			t.Fatalf("Validate error = %v", err)
		}
	})

	t.Run("missing target process", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes = cfg.Processes[:1]
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "references missing process") {
			t.Fatalf("Validate error = %v, want missing peer process", err)
		}
	})

	t.Run("empty secret", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[0].Peers[0].Secret = " "
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "secret is empty") {
			t.Fatalf("Validate error = %v, want empty peer secret", err)
		}
	})

	t.Run("self peer", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[0].Peers[0].ProcessID = 1
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "cannot peer with itself") {
			t.Fatalf("Validate error = %v, want self peer rejection", err)
		}
	})

	t.Run("duplicate peer", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[0].Peers = append(cfg.Processes[0].Peers,
			StartProcessPeerConfig{ProcessID: 2, Address: "127.0.0.1:10002", Secret: "shared-secret"})
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "duplicate peer") {
			t.Fatalf("Validate error = %v, want duplicate peer rejection", err)
		}
	})

	t.Run("peer requires inner port", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[0].InnerPort = 0
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "with peers must define innerPort") {
			t.Fatalf("Validate error = %v, want inner port requirement", err)
		}
	})

	t.Run("requires reciprocal peer", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[1].Peers = nil
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "reciprocal peer definition") {
			t.Fatalf("Validate error = %v, want reciprocal peer error", err)
		}
	})

	t.Run("requires matching peer secret", func(t *testing.T) {
		cfg := newConfig()
		cfg.Processes[1].Peers[0].Secret = "different-secret"
		if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "secret does not match") {
			t.Fatalf("Validate error = %v, want peer secret mismatch", err)
		}
	})
}

func TestRuntimeTopologyIsDeterministicAndReportsImplicitNetInner(t *testing.T) {
	cfg := &Config{
		Machines: []StartMachineConfig{
			{ID: 1, InnerIP: "10.0.0.1", OuterIP: "203.0.113.1"},
		},
		Processes: []StartProcessConfig{
			{
				ID:        1,
				MachineID: 1,
				InnerPort: 10001,
				Peers: []StartProcessPeerConfig{
					{ProcessID: 3, Address: "10.0.0.3:10003"},
					{ProcessID: 2, Address: "10.0.0.2:10002"},
				},
			},
		},
		Scenes: []StartSceneConfig{
			{ID: 1002, ProcessID: 1, Zone: 2, SceneType: "Map", Name: "Map2"},
			{ID: 1001, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "Realm"},
		},
	}

	topology, err := cfg.RuntimeTopology(1)
	if err != nil {
		t.Fatalf("RuntimeTopology error = %v", err)
	}
	if topology.MachineID != 1 ||
		topology.MachineInnerIP != "10.0.0.1" ||
		topology.MachineOuterIP != "203.0.113.1" ||
		topology.InnerPort != 10001 {
		t.Fatalf("topology machine/process fields = %+v", topology)
	}
	if !topology.ImplicitNetInner {
		t.Fatal("topology should report implicit NetInner")
	}
	if len(topology.Peers) != 2 || topology.Peers[0].ProcessID != 2 || topology.Peers[1].ProcessID != 3 {
		t.Fatalf("topology peers = %+v, want sorted process ids 2,3", topology.Peers)
	}
	if len(topology.Scenes) != 2 || topology.Scenes[0].ID != 1001 || topology.Scenes[1].ID != 1002 {
		t.Fatalf("topology scenes = %+v, want sorted scene ids 1001,1002", topology.Scenes)
	}
}

func TestRuntimeTopologyRejectsUnknownProcess(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.RuntimeTopology(99); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("RuntimeTopology error = %v, want unknown process error", err)
	}
}

func TestValidateSecurityKeyRing(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Main", Name: "main"},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
		Security: StartSecurityConfig{
			AccessTokenCurrentKeyID: "primary",
			AccessTokenKeys: []StartAccessTokenKeyConfig{
				{ID: "primary", Secret: "01234567890123456789012345678901"},
				{ID: "previous", Secret: "abcdefghijklmnopqrstuvwxyz123456"},
			},
			LegacyTokenKey:    "legacy-key",
			AllowLegacyTokens: true,
		},
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate security error = %v", err)
	}

	cfg.Security.AccessTokenCurrentKeyID = "missing"
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Validate missing current key error = %v", err)
	}
}

func TestLoadAndValidateSecurityConfig(t *testing.T) {
	dir := t.TempDir()
	writeConfigFile(t, dir, "startmachineconfig.json", `[{"id":1,"innerIP":"127.0.0.1"}]`)
	writeConfigFile(t, dir, "startprocessconfig.json", `[{"id":1,"machineId":1}]`)
	writeConfigFile(t, dir, "startsceneconfig.json", `[{"id":1,"processId":1,"zone":1,"sceneType":"Main","name":"main"}]`)
	writeConfigFile(t, dir, "startzoneconfig.json", `[{"id":1,"name":"dev","dbName":"etgo","dbAddr":"mongodb://127.0.0.1:27017"}]`)
	writeConfigFile(t, dir, "startsecurityconfig.json", `{
		"accessTokenCurrentKeyId":"primary",
		"accessTokenKeys":[
			{"id":"primary","secret":"01234567890123456789012345678901"},
			{"id":"previous","secret":"abcdefghijklmnopqrstuvwxyz123456"}
		],
		"legacyTokenKey":"migration-only-key",
		"allowLegacyTokens":true,
		"corsAllowedOrigins":["https://game.example"]
	}`)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
	if cfg.Security.AccessTokenCurrentKeyID != "primary" ||
		len(cfg.Security.AccessTokenKeys) != 2 ||
		len(cfg.Security.CORSAllowedOrigins) != 1 {
		t.Fatalf("security config = %+v, want loaded key ring and CORS allowlist", cfg.Security)
	}
}

func TestValidateRejectsWildcardCORSOrigin(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Main", Name: "main"},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
		Security: StartSecurityConfig{
			AccessTokenCurrentKeyID: "primary",
			AccessTokenKeys: []StartAccessTokenKeyConfig{
				{ID: "primary", Secret: "01234567890123456789012345678901"},
			},
			CORSAllowedOrigins: []string{"*"},
		},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "corsAllowedOrigins") {
		t.Fatalf("Validate error = %v, want wildcard CORS rejection", err)
	}
}

func TestValidateRejectsPartialHTTPTLSConfiguration(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Main", Name: "main"},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
		Security: StartSecurityConfig{
			AccessTokenCurrentKeyID: "primary",
			AccessTokenKeys: []StartAccessTokenKeyConfig{
				{ID: "primary", Secret: "01234567890123456789012345678901"},
			},
			HTTPRequireTLS:  true,
			HTTPTLSCertFile: "server.crt",
		},
	}
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "httpTLSKeyFile") {
		t.Fatalf("Validate TLS error = %v, want both certificate and key requirement", err)
	}
}

func TestValidateAcceptsExplicitLegacyAccessTokenFormat(t *testing.T) {
	cfg := &Config{
		Machines:  []StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1"}},
		Processes: []StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes: []StartSceneConfig{
			{ID: 1, ProcessID: 1, Zone: 1, SceneType: "Realm", Name: "realm", OuterPort: 10001},
		},
		Zones: []StartZoneConfig{
			{ID: 1, Name: "dev", DBName: "etgo", DBAddr: "mongodb://127.0.0.1:27017"},
		},
		Security: StartSecurityConfig{
			AccessTokenFormat: "legacy",
			LegacyTokenKey:    "whosyourdaddy",
			AllowLegacyTokens: true,
		},
	}
	if err := cfg.Validate(1); err != nil {
		t.Fatalf("Validate legacy security error = %v", err)
	}

	cfg.Security.AllowLegacyTokens = false
	if err := cfg.Validate(1); err == nil || !strings.Contains(err.Error(), "accessTokenFormat") {
		t.Fatalf("Validate legacy without allow error = %v", err)
	}
}

func writeConfigFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
