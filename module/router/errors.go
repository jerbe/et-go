package router

const (
	// ERR_WithException 对齐 ET 全局逻辑异常起始值。
	ERR_WithException = 100000000
	// ERR_GetRouterNoZones 对齐目标 Router 包错误码。
	ERR_GetRouterNoZones = ERR_WithException + 15*1000 + 8
)
