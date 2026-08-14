package ecs

// AwakeSystem 组件初始化接口。在 AddComponent 时自动调用。
type AwakeSystem interface {
	Awake()
}

// UpdateSystem 帧更新接口。每帧由 World 调度调用。
type UpdateSystem interface {
	Update()
}

// LateUpdateSystem 延迟更新接口。在所有 Update 之后调用。
type LateUpdateSystem interface {
	LateUpdate()
}

// DestroySystem 清理销毁接口。在 RemoveComponent 或 Dispose 时调用。
type DestroySystem interface {
	OnDestroy()
}

// SerializeSystem 序列化接口。保存/传输时调用。
type SerializeSystem interface {
	Serialize() ([]byte, error)
}

// DeserializeSystem 反序列化接口。加载/接收时调用。
type DeserializeSystem interface {
	Deserialize(data []byte) error
}

// TransferSystem 跨进程迁移接口。
type TransferSystem interface {
	// Transfer 返回迁移时需要序列化的数据。
	Transfer() ([]byte, error)
	// OnTransferIn 在目标进程接收迁移数据后调用。
	OnTransferIn(data []byte) error
}
