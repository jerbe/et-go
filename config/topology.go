package config

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimePeerSummary 描述当前 Process 启动时需要连接的远端 peer。
type RuntimePeerSummary struct {
	ProcessID int
	Address   string
}

// RuntimeSceneSummary 描述当前 Process 的一个显式 Scene 配置。
type RuntimeSceneSummary struct {
	ID        int
	ProcessID int
	Zone      int
	SceneType string
	Name      string
	OuterPort int
}

// RuntimeTopology 是经过配置解析后的当前 Process 启动拓扑摘要。
//
// 它只包含配置事实，不创建连接、Fiber 或数据库客户端；启动器可以在
// 创建运行时对象前输出该摘要，避免把日志当作运行时状态的猜测。
type RuntimeTopology struct {
	ProcessID        int
	MachineID        int
	MachineInnerIP   string
	MachineOuterIP   string
	InnerPort        int
	ImplicitNetInner bool
	Peers            []RuntimePeerSummary
	Scenes           []RuntimeSceneSummary
}

// RuntimeTopology 返回当前 Process 的确定性启动拓扑。
func (c *Config) RuntimeTopology(processID int) (RuntimeTopology, error) {
	if c == nil {
		return RuntimeTopology{}, fmt.Errorf("config: config is nil")
	}
	if processID <= 0 {
		return RuntimeTopology{}, fmt.Errorf("config: process id must be positive: %d", processID)
	}

	var process *StartProcessConfig
	for index := range c.Processes {
		if c.Processes[index].ID == processID {
			process = &c.Processes[index]
			break
		}
	}
	if process == nil {
		return RuntimeTopology{}, fmt.Errorf("config: process %d is not configured", processID)
	}

	var machine *StartMachineConfig
	for index := range c.Machines {
		if c.Machines[index].ID == process.MachineID {
			machine = &c.Machines[index]
			break
		}
	}
	if machine == nil {
		return RuntimeTopology{}, fmt.Errorf(
			"config: process %d references missing machine %d",
			processID,
			process.MachineID,
		)
	}

	peers := make([]RuntimePeerSummary, 0, len(process.Peers))
	for _, peer := range process.Peers {
		peers = append(peers, RuntimePeerSummary{
			ProcessID: peer.ProcessID,
			Address:   strings.TrimSpace(peer.Address),
		})
	}
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].ProcessID < peers[j].ProcessID
	})

	scenes := make([]RuntimeSceneSummary, 0)
	hasExplicitNetInner := false
	for _, scene := range c.Scenes {
		if scene.ProcessID != processID {
			continue
		}
		sceneType := strings.TrimSpace(scene.SceneType)
		if strings.EqualFold(sceneType, "netinner") {
			hasExplicitNetInner = true
		}
		scenes = append(scenes, RuntimeSceneSummary{
			ID:        scene.ID,
			ProcessID: scene.ProcessID,
			Zone:      scene.Zone,
			SceneType: sceneType,
			Name:      strings.TrimSpace(scene.Name),
			OuterPort: scene.OuterPort,
		})
	}
	sort.Slice(scenes, func(i, j int) bool {
		if scenes[i].ID != scenes[j].ID {
			return scenes[i].ID < scenes[j].ID
		}
		return scenes[i].Name < scenes[j].Name
	})

	return RuntimeTopology{
		ProcessID:        processID,
		MachineID:        process.MachineID,
		MachineInnerIP:   strings.TrimSpace(machine.InnerIP),
		MachineOuterIP:   strings.TrimSpace(machine.OuterIP),
		InnerPort:        process.InnerPort,
		ImplicitNetInner: len(process.Peers) > 0 && !hasExplicitNetInner,
		Peers:            peers,
		Scenes:           scenes,
	}, nil
}
