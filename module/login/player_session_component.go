package login

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
)

// PlayerSessionComponent 保存玩家到 Session 的引用。
type PlayerSessionComponent struct {
	ecs.BaseComponent
	Session *network.Session
}

// Type 返回组件名称。
func (c *PlayerSessionComponent) Type() string { return "PlayerSessionComponent" }
