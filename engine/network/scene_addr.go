package network

import (
	"fmt"
	"strings"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
)

var (
	// ErrSceneConfigMissing 表示运行时 Scene 没有匹配启动配置。
	ErrSceneConfigMissing = fmt.Errorf("network: scene config missing")
	// ErrSceneMachineMissing 表示 Scene 所属 Process/Machine 配置缺失。
	ErrSceneMachineMissing = fmt.Errorf("network: scene machine config missing")
	// ErrSceneAddressMissing 表示 Scene 没有可用监听地址或端口。
	ErrSceneAddressMissing = fmt.Errorf("network: scene listen address missing")
)

// ResolveSceneListenAddr 根据场景配置解析监听地址。
func ResolveSceneListenAddr(scene *ecs.Scene, preferInner bool) (string, error) {
	sceneCfg, cfg := resolveSceneConfig(scene)
	if sceneCfg == nil || cfg == nil {
		return "", ErrSceneConfigMissing
	}

	host := ""
	if machine := resolveSceneMachine(cfg, sceneCfg.ProcessID); machine != nil {
		switch {
		case preferInner && machine.InnerIP != "":
			host = machine.InnerIP
		case !preferInner && machine.OuterIP != "":
			host = machine.OuterIP
		case preferInner && machine.OuterIP != "":
			host = machine.OuterIP
		case !preferInner && machine.InnerIP != "":
			host = machine.InnerIP
		default:
			return "", ErrSceneMachineMissing
		}
	} else {
		return "", ErrSceneMachineMissing
	}

	port := sceneCfg.OuterPort
	if port <= 0 {
		for _, process := range cfg.Processes {
			if process.ID == sceneCfg.ProcessID && process.InnerPort > 0 {
				port = process.InnerPort
				break
			}
		}
	}
	if port < 0 {
		return "", ErrSceneAddressMissing
	}
	return fmt.Sprintf("%s:%d", host, port), nil
}

func resolveSceneConfig(scene *ecs.Scene) (*config.StartSceneConfig, *config.Config) {
	cfg := config.GetGlobal()
	if cfg == nil || scene == nil {
		return nil, cfg
	}

	if scene.ID() > 0 {
		for index := range cfg.Scenes {
			if int64(cfg.Scenes[index].ID) == scene.ID() {
				return &cfg.Scenes[index], cfg
			}
		}
	}

	sceneType := strings.ToLower(scene.SceneType().String())
	for index := range cfg.Scenes {
		if strings.ToLower(cfg.Scenes[index].SceneType) != sceneType {
			continue
		}
		if cfg.Scenes[index].Zone != scene.Zone() {
			continue
		}
		return &cfg.Scenes[index], cfg
	}
	return nil, cfg
}

func resolveSceneMachine(cfg *config.Config, processID int) *config.StartMachineConfig {
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
