package gamelogin

const (
	// MsgG2GameLogin 表示 Gate 到 Game 的登录查询请求。
	MsgG2GameLogin uint16 = 21501
	// MsgGame2GLogin 表示 Game 到 Gate 的登录查询响应。
	MsgGame2GLogin uint16 = 21502
)

// G2GameLogin 表示 Gate 请求玩家登录信息。
type G2GameLogin struct {
	RpcId     uint32 `json:"rpc_id"`
	AccountId int64  `json:"account_id"`
}

// Game2GLogin 表示 Game 返回玩家登录信息。
type Game2GLogin struct {
	RpcId     uint32 `json:"rpc_id"`
	Error     int32  `json:"error"`
	Message   string `json:"message,omitempty"`
	AccountId int64  `json:"account_id,omitempty"`
	ZoneId    int32  `json:"zone_id,omitempty"`
	PlayerId  int64  `json:"player_id,omitempty"`
}
