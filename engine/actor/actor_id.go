package actor

// ActorID 标识一个 Actor 的全局唯一地址。
// 结构：(ProcessID, FiberID, InstanceID)
type ActorID struct {
	ProcessID  int
	FiberID    int64
	InstanceID int64
}

// Address 返回 Actor 的地址部分（不含 InstanceID）
func (a ActorID) Address() Address {
	return Address{ProcessID: a.ProcessID, FiberID: a.FiberID}
}

// IsValid 检查 ActorID 是否有效
func (a ActorID) IsValid() bool {
	return a.ProcessID > 0 && a.FiberID > 0 && a.InstanceID > 0
}

// Address 标识一个 Fiber 级别的地址（进程 + Fiber）
type Address struct {
	ProcessID int
	FiberID   int64
}
