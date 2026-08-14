package login

const (
	// MsgC2RLogin 表示客户端到 Realm 的登录请求。
	MsgC2RLogin uint16 = 2001
	// MsgR2CLogin 表示 Realm 到客户端的登录响应。
	MsgR2CLogin uint16 = 2002
	// MsgC2GLoginGate 表示客户端到 Gate 的登录请求。
	MsgC2GLoginGate uint16 = 2105
	// MsgG2CLoginGate 表示 Gate 到客户端的登录响应。
	MsgG2CLoginGate uint16 = 2106
	// MsgC2GPing 表示客户端 Ping 请求。
	MsgC2GPing uint16 = 2103
	// MsgG2CPing 表示 Gate 到客户端的 Ping 响应。
	MsgG2CPing uint16 = 2104
	// MsgR2GGateAssign 表示 Realm 到 Gate 的 Token 分配请求。
	MsgR2GGateAssign uint16 = 22001
	// MsgG2RGateAssign 表示 Gate 到 Realm 的 Token 分配响应。
	MsgG2RGateAssign uint16 = 22002
	// MsgG2MSessionDisconnect 表示 Gate 到地图的断线通知。
	MsgG2MSessionDisconnect uint16 = 22003
)

// C2RLogin 表示 Realm 登录请求。
type C2RLogin struct {
	RpcId       uint32 `json:"rpc_id"`
	AccessToken string `json:"access_token"`
	ZoneId      int32  `json:"zone_id"`
}

// R2CLogin 表示 Realm 登录响应。
type R2CLogin struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
	Address string `json:"address,omitempty"`
	GateId  int64  `json:"gate_id,omitempty"`
	Token   string `json:"token,omitempty"`
}

// R2GGateAssign 表示 Realm 请求 Gate 生成登录 Token。
type R2GGateAssign struct {
	RpcId     uint32 `json:"rpc_id"`
	AccountId int64  `json:"account_id"`
}

// G2RGateAssign 表示 Gate 返回分配结果。
type G2RGateAssign struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
	GateId  int64  `json:"gate_id,omitempty"`
	Token   string `json:"token,omitempty"`
}

// C2GLoginGate 表示客户端使用 Gate Token 登录。
type C2GLoginGate struct {
	RpcId  uint32 `json:"rpc_id"`
	Token  string `json:"token"`
	GateId int64  `json:"gate_id"`
}

// G2CLoginGate 表示 Gate 登录响应。
type G2CLoginGate struct {
	RpcId          uint32 `json:"rpc_id"`
	Error          int32  `json:"error"`
	Message        string `json:"message,omitempty"`
	PlayerId       int64  `json:"player_id,omitempty"`
	CharacterCount int64  `json:"character_count,omitempty"`
}

// C2GPing 表示客户端心跳请求。
type C2GPing struct {
	RpcId uint32 `json:"rpc_id"`
}

// G2CPing 表示心跳响应。
type G2CPing struct {
	RpcId   uint32 `json:"rpc_id"`
	Error   int32  `json:"error"`
	Message string `json:"message,omitempty"`
	Time    int64  `json:"time"`
}

// G2MSessionDisconnect 表示 Gate 通知地图连接断开。
type G2MSessionDisconnect struct {
	RpcId uint32 `json:"rpc_id"`
}
