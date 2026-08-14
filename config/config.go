package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StartMachineConfig 机器配置
type StartMachineConfig struct {
	ID          int    `json:"id"`
	InnerIP     string `json:"innerIP"`
	OuterIP     string `json:"outerIP"`
	WatcherPort int    `json:"watcherPort"`
}

// StartProcessConfig 进程配置
type StartProcessConfig struct {
	ID        int `json:"id"`
	MachineID int `json:"machineId"`
	InnerPort int `json:"innerPort"`
	// Peers 是当前 Process 需要连接的远端 Process。
	// Address 必须是远端 NetInner 的内网地址，Secret 用于握手认证。
	Peers []StartProcessPeerConfig `json:"peers,omitempty"`
}

// StartProcessPeerConfig 描述一个跨进程 Actor peer。
type StartProcessPeerConfig struct {
	ProcessID int    `json:"processId"`
	Address   string `json:"address"`
	Secret    string `json:"secret"`
}

// StartSceneConfig 场景配置
type StartSceneConfig struct {
	ID        int    `json:"id"`
	ProcessID int    `json:"processId"`
	Zone      int    `json:"zone"`
	SceneType string `json:"sceneType"`
	Name      string `json:"name"`
	OuterPort int    `json:"outerPort"`
	// MapTargets 是 Map 场景可转移到的目标 Scene 名称。
	// 当前 C2M_TransferMap 不携带目标名，因此最多配置一个目标。
	MapTargets []string `json:"mapTargets,omitempty"`
	// NavMeshFile 是 Map 场景使用的导航网格资源路径。
	NavMeshFile string `json:"navMeshFile,omitempty"`
}

// StartZoneConfig 分区配置
type StartZoneConfig struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	DBName    string `json:"dbName"`
	DBAddr    string `json:"dbAddr"`
	ServerURL string `json:"serverURL"`
	IsLogic   bool   `json:"isLogic"`
}

// StartAreaConfig 大区配置
type StartAreaConfig struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ServerURL string `json:"serverURL"`
}

// StartAccessTokenKeyConfig 描述一个可用于签名 AccessToken 的密钥。
type StartAccessTokenKeyConfig struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// StartSecurityConfig 描述进程共享的认证安全配置。
type StartSecurityConfig struct {
	// AccessTokenFormat 控制 AccessToken 生成格式：signed（默认）或
	// legacy（显式兼容原 ET SimpleToken）。
	AccessTokenFormat       string                      `json:"accessTokenFormat,omitempty"`
	AccessTokenCurrentKeyID string                      `json:"accessTokenCurrentKeyId"`
	AccessTokenKeys         []StartAccessTokenKeyConfig `json:"accessTokenKeys,omitempty"`
	LegacyTokenKey          string                      `json:"legacyTokenKey,omitempty"`
	AllowLegacyTokens       bool                        `json:"allowLegacyTokens,omitempty"`
	CORSAllowedOrigins      []string                    `json:"corsAllowedOrigins,omitempty"`
	// HTTPRequireTLS 要求 HTTP Scene 使用 TLS；启用时必须同时配置证书和私钥。
	HTTPRequireTLS bool `json:"httpRequireTLS,omitempty"`
	// HTTPTLSCertFile/HTTPTLSKeyFile 是 HTTP Scene 的 TLS 证书和私钥路径。
	HTTPTLSCertFile string `json:"httpTLSCertFile,omitempty"`
	HTTPTLSKeyFile  string `json:"httpTLSKeyFile,omitempty"`
	// LoginRateLimitPerMinute 为 HTTP 登录启用按来源和账号组合的固定窗口限流。
	// 小于等于 0 表示没有安装限流组件，生产配置应显式给出正数。
	LoginRateLimitPerMinute int `json:"loginRateLimitPerMinute,omitempty"`
}

// Config 全局配置容器
type Config struct {
	Machines  []StartMachineConfig
	Processes []StartProcessConfig
	Scenes    []StartSceneConfig
	Zones     []StartZoneConfig
	Areas     []StartAreaConfig
	Security  StartSecurityConfig
}

// Load 从指定目录加载启动所需 JSON 配置。
//
// 机器、进程、场景和 Zone 是启动拓扑的必需配置；缺失任何一个文件都必须
// 直接报错，不能把空配置当成可运行状态。Area 只在配置 HTTP/Router 对外
// Area 列表时需要，因此保持可选，但文件一旦存在就必须能够正确解析。
func Load(dir string) (*Config, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("config: directory is empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("config: stat directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("config: path is not a directory: %q", dir)
	}

	cfg := &Config{}

	loaders := []struct {
		file     string
		dest     any
		required bool
	}{
		{file: "startmachineconfig.json", dest: &cfg.Machines, required: true},
		{file: "startprocessconfig.json", dest: &cfg.Processes, required: true},
		{file: "startsceneconfig.json", dest: &cfg.Scenes, required: true},
		{file: "startzoneconfig.json", dest: &cfg.Zones, required: true},
		{file: "startareaconfig.json", dest: &cfg.Areas},
		{file: "startsecurityconfig.json", dest: &cfg.Security},
	}

	for _, l := range loaders {
		path := filepath.Join(dir, l.file)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) && !l.required {
				continue
			}
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("config: required file missing: %s", path)
			}
			return nil, fmt.Errorf("config: stat %s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: read %s: %w", path, err)
		}
		if err := json.Unmarshal(data, l.dest); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}

	return cfg, nil
}

// Validate 校验当前逻辑 Process 能够被安全启动。
func (c *Config) Validate(processID int) error {
	if c == nil {
		return fmt.Errorf("config: config is nil")
	}
	if processID <= 0 {
		return fmt.Errorf("config: process id must be positive: %d", processID)
	}

	machines := make(map[int]StartMachineConfig, len(c.Machines))
	for _, machine := range c.Machines {
		if machine.ID <= 0 {
			return fmt.Errorf("config: machine id must be positive: %d", machine.ID)
		}
		if _, exists := machines[machine.ID]; exists {
			return fmt.Errorf("config: duplicate machine id: %d", machine.ID)
		}
		if strings.TrimSpace(machine.InnerIP) == "" && strings.TrimSpace(machine.OuterIP) == "" {
			return fmt.Errorf("config: machine %d has no innerIP or outerIP", machine.ID)
		}
		machines[machine.ID] = machine
	}

	processes := make(map[int]StartProcessConfig, len(c.Processes))
	for _, process := range c.Processes {
		if process.ID <= 0 {
			return fmt.Errorf("config: process id must be positive: %d", process.ID)
		}
		if process.InnerPort < 0 {
			return fmt.Errorf("config: process %d innerPort must not be negative: %d", process.ID, process.InnerPort)
		}
		if len(process.Peers) > 0 && process.InnerPort == 0 {
			return fmt.Errorf("config: process %d with peers must define innerPort", process.ID)
		}
		if _, exists := processes[process.ID]; exists {
			return fmt.Errorf("config: duplicate process id: %d", process.ID)
		}
		if _, exists := machines[process.MachineID]; !exists {
			return fmt.Errorf("config: process %d references missing machine %d", process.ID, process.MachineID)
		}
		peerIDs := make(map[int]struct{}, len(process.Peers))
		for _, peer := range process.Peers {
			if peer.ProcessID <= 0 {
				return fmt.Errorf("config: process %d peer processId must be positive: %d", process.ID, peer.ProcessID)
			}
			if peer.ProcessID == process.ID {
				return fmt.Errorf("config: process %d cannot peer with itself", process.ID)
			}
			if strings.TrimSpace(peer.Address) == "" {
				return fmt.Errorf("config: process %d peer %d address is empty", process.ID, peer.ProcessID)
			}
			if strings.TrimSpace(peer.Secret) == "" {
				return fmt.Errorf("config: process %d peer %d secret is empty", process.ID, peer.ProcessID)
			}
			if _, exists := peerIDs[peer.ProcessID]; exists {
				return fmt.Errorf("config: process %d has duplicate peer %d", process.ID, peer.ProcessID)
			}
			peerIDs[peer.ProcessID] = struct{}{}
		}
		processes[process.ID] = process
	}
	if _, exists := processes[processID]; !exists {
		return fmt.Errorf("config: process %d is not configured", processID)
	}

	for _, process := range c.Processes {
		for _, peer := range process.Peers {
			targetProcess, exists := processes[peer.ProcessID]
			if !exists {
				return fmt.Errorf("config: process %d peer %d references missing process", process.ID, peer.ProcessID)
			}
			reciprocal, reciprocalFound := findPeer(targetProcess.Peers, process.ID)
			if !reciprocalFound {
				return fmt.Errorf(
					"config: process %d peer %d requires reciprocal peer definition",
					process.ID,
					peer.ProcessID,
				)
			}
			if strings.TrimSpace(reciprocal.Secret) != strings.TrimSpace(peer.Secret) {
				return fmt.Errorf(
					"config: process %d peer %d secret does not match reciprocal definition",
					process.ID,
					peer.ProcessID,
				)
			}
		}
	}

	netInnerProcesses := make(map[int]struct{})
	for _, scene := range c.Scenes {
		if strings.EqualFold(strings.TrimSpace(scene.SceneType), "netinner") {
			netInnerProcesses[scene.ProcessID] = struct{}{}
		}
	}

	zones := make(map[int]StartZoneConfig, len(c.Zones))
	for _, zone := range c.Zones {
		if zone.ID <= 0 {
			return fmt.Errorf("config: zone id must be positive: %d", zone.ID)
		}
		if _, exists := zones[zone.ID]; exists {
			return fmt.Errorf("config: duplicate zone id: %d", zone.ID)
		}
		if strings.TrimSpace(zone.Name) == "" {
			return fmt.Errorf("config: zone %d name is empty", zone.ID)
		}
		if strings.TrimSpace(zone.DBAddr) == "" {
			return fmt.Errorf("config: zone %d dbAddr is empty", zone.ID)
		}
		if strings.TrimSpace(zone.DBName) == "" {
			return fmt.Errorf("config: zone %d dbName is empty", zone.ID)
		}
		zones[zone.ID] = zone
	}

	sceneIDs := make(map[int]struct{}, len(c.Scenes))
	sceneNames := make(map[string]StartSceneConfig, len(c.Scenes))
	sceneCount := 0
	for _, scene := range c.Scenes {
		if scene.ID <= 0 {
			return fmt.Errorf("config: scene id must be positive: %d", scene.ID)
		}
		if _, exists := sceneIDs[scene.ID]; exists {
			return fmt.Errorf("config: duplicate scene id: %d", scene.ID)
		}
		sceneIDs[scene.ID] = struct{}{}
		if _, exists := processes[scene.ProcessID]; !exists {
			return fmt.Errorf("config: scene %d references missing process %d", scene.ID, scene.ProcessID)
		}
		if _, exists := zones[scene.Zone]; !exists {
			return fmt.Errorf("config: scene %d references missing zone %d", scene.ID, scene.Zone)
		}
		if strings.TrimSpace(scene.SceneType) == "" {
			return fmt.Errorf("config: scene %d sceneType is empty", scene.ID)
		}
		if !supportedSceneType(scene.SceneType) {
			return fmt.Errorf("config: scene %d has unsupported sceneType %q", scene.ID, scene.SceneType)
		}
		if strings.EqualFold(strings.TrimSpace(scene.SceneType), "netinner") &&
			processes[scene.ProcessID].InnerPort <= 0 {
			return fmt.Errorf("config: scene %d NetInner requires positive Process %d innerPort", scene.ID, scene.ProcessID)
		}
		if strings.TrimSpace(scene.Name) == "" {
			return fmt.Errorf("config: scene %d name is empty", scene.ID)
		}
		if scene.OuterPort < 0 {
			return fmt.Errorf("config: scene %d outerPort must not be negative: %d", scene.ID, scene.OuterPort)
		}
		if sceneRequiresListenPort(scene.SceneType) {
			process := processes[scene.ProcessID]
			if scene.OuterPort == 0 && process.InnerPort == 0 {
				return fmt.Errorf("config: scene %d (%s) has no listen port", scene.ID, scene.SceneType)
			}
			if scene.OuterPort == 0 {
				if _, hasNetInner := netInnerProcesses[scene.ProcessID]; hasNetInner || len(process.Peers) > 0 {
					return fmt.Errorf("config: scene %d (%s) must define outerPort when Process %d owns NetInner", scene.ID, scene.SceneType, scene.ProcessID)
				}
			}
		}
		sceneNameKey := fmt.Sprintf("%d:%s", scene.Zone, strings.ToLower(strings.TrimSpace(scene.Name)))
		if _, exists := sceneNames[sceneNameKey]; exists {
			return fmt.Errorf("config: duplicate scene name in zone %d: %q", scene.Zone, scene.Name)
		}
		sceneNames[sceneNameKey] = scene
		if !strings.EqualFold(strings.TrimSpace(scene.SceneType), "map") && len(scene.MapTargets) > 0 {
			return fmt.Errorf("config: non-map scene %d cannot define mapTargets", scene.ID)
		}
		if len(scene.MapTargets) > 1 {
			return fmt.Errorf("config: map scene %d has multiple targets but transfer protocol has no target name", scene.ID)
		}
		if scene.ProcessID == processID {
			sceneCount++
		}
	}
	if sceneCount == 0 {
		return fmt.Errorf("config: process %d has no scene", processID)
	}

	for _, scene := range c.Scenes {
		for _, targetName := range scene.MapTargets {
			targetName = strings.TrimSpace(targetName)
			if targetName == "" {
				return fmt.Errorf("config: map scene %d has empty target name", scene.ID)
			}
			target, ok := sceneNames[fmt.Sprintf("%d:%s", scene.Zone, strings.ToLower(targetName))]
			if !ok {
				return fmt.Errorf("config: map scene %d references missing target %q in zone %d", scene.ID, targetName, scene.Zone)
			}
			if !strings.EqualFold(strings.TrimSpace(target.SceneType), "map") {
				return fmt.Errorf("config: map scene %d target %q is not a map scene", scene.ID, targetName)
			}
			if target.ID == scene.ID {
				return fmt.Errorf("config: map scene %d cannot target itself", scene.ID)
			}
			if target.ProcessID != scene.ProcessID {
				return fmt.Errorf("config: map scene %d target %q is on process %d; cross-process target discovery is not configured", scene.ID, targetName, target.ProcessID)
			}
		}
	}

	areaIDs := make(map[int]struct{}, len(c.Areas))
	for _, area := range c.Areas {
		if area.ID <= 0 {
			return fmt.Errorf("config: area id must be positive: %d", area.ID)
		}
		if _, exists := areaIDs[area.ID]; exists {
			return fmt.Errorf("config: duplicate area id: %d", area.ID)
		}
		if strings.TrimSpace(area.Name) == "" {
			return fmt.Errorf("config: area %d name is empty", area.ID)
		}
		areaIDs[area.ID] = struct{}{}
	}

	if err := validateSecurity(c.Security); err != nil {
		return err
	}

	return nil
}

func validateSecurity(security StartSecurityConfig) error {
	if strings.TrimSpace(security.AccessTokenCurrentKeyID) == "" &&
		len(security.AccessTokenKeys) == 0 &&
		strings.TrimSpace(security.LegacyTokenKey) == "" &&
		strings.TrimSpace(security.AccessTokenFormat) == "" &&
		!security.AllowLegacyTokens &&
		!security.HTTPRequireTLS &&
		strings.TrimSpace(security.HTTPTLSCertFile) == "" &&
		strings.TrimSpace(security.HTTPTLSKeyFile) == "" &&
		security.LoginRateLimitPerMinute <= 0 {
		return nil
	}
	format := strings.ToLower(strings.TrimSpace(security.AccessTokenFormat))
	if format == "" {
		format = "signed"
	}
	if format != "signed" && format != "legacy" {
		return fmt.Errorf("config: security accessTokenFormat must be signed or legacy")
	}
	if format == "signed" && strings.TrimSpace(security.AccessTokenCurrentKeyID) == "" {
		return fmt.Errorf("config: security accessTokenCurrentKeyId is empty")
	}
	if format == "signed" && len(security.AccessTokenKeys) == 0 {
		return fmt.Errorf("config: security accessTokenKeys is empty")
	}
	if format == "legacy" && (!security.AllowLegacyTokens || strings.TrimSpace(security.LegacyTokenKey) == "") {
		return fmt.Errorf("config: legacy accessTokenFormat requires allowLegacyTokens and legacyTokenKey")
	}
	seen := make(map[string]struct{}, len(security.AccessTokenKeys))
	currentFound := false
	for _, key := range security.AccessTokenKeys {
		keyID := strings.TrimSpace(key.ID)
		if keyID == "" {
			return fmt.Errorf("config: security access token key id is empty")
		}
		if _, exists := seen[keyID]; exists {
			return fmt.Errorf("config: duplicate security access token key id: %s", keyID)
		}
		if len([]byte(key.Secret)) < 32 {
			return fmt.Errorf("config: security access token key %s secret must be at least 32 bytes", keyID)
		}
		seen[keyID] = struct{}{}
		if keyID == strings.TrimSpace(security.AccessTokenCurrentKeyID) {
			currentFound = true
		}
	}
	if len(security.AccessTokenKeys) > 0 && strings.TrimSpace(security.AccessTokenCurrentKeyID) != "" && !currentFound {
		return fmt.Errorf("config: security current access token key %q is not configured", security.AccessTokenCurrentKeyID)
	}
	if security.AllowLegacyTokens && strings.TrimSpace(security.LegacyTokenKey) == "" {
		return fmt.Errorf("config: security legacyTokenKey is required when allowLegacyTokens is true")
	}
	for _, origin := range security.CORSAllowedOrigins {
		if strings.TrimSpace(origin) == "" || strings.TrimSpace(origin) == "*" {
			return fmt.Errorf("config: security corsAllowedOrigins contains invalid origin")
		}
	}
	certFile := strings.TrimSpace(security.HTTPTLSCertFile)
	keyFile := strings.TrimSpace(security.HTTPTLSKeyFile)
	if (certFile == "") != (keyFile == "") {
		return fmt.Errorf("config: security HTTP TLS requires both httpTLSCertFile and httpTLSKeyFile")
	}
	if security.HTTPRequireTLS && (certFile == "" || keyFile == "") {
		return fmt.Errorf("config: security httpRequireTLS requires httpTLSCertFile and httpTLSKeyFile")
	}
	if security.LoginRateLimitPerMinute < 0 {
		return fmt.Errorf("config: security loginRateLimitPerMinute must not be negative")
	}
	return nil
}

func sceneRequiresListenPort(sceneType string) bool {
	switch strings.ToLower(strings.TrimSpace(sceneType)) {
	case "realm", "gate", "http", "router", "routernode":
		return true
	default:
		return false
	}
}

func supportedSceneType(sceneType string) bool {
	switch strings.ToLower(strings.TrimSpace(sceneType)) {
	case "main", "launch", "netinner", "netclient",
		"location", "router", "routernode", "realm", "gate",
		"lockstep", "match", "room", "http", "map", "central":
		return true
	default:
		return false
	}
}

func findPeer(peers []StartProcessPeerConfig, processID int) (StartProcessPeerConfig, bool) {
	for _, peer := range peers {
		if peer.ProcessID == processID {
			return peer, true
		}
	}
	return StartProcessPeerConfig{}, false
}
