package central

const (
	// MsgR2CentralAccountLogin 表示 Realm 到 Central 的账号验证请求。
	MsgR2CentralAccountLogin uint16 = 50001
	// MsgCentral2RAccountLogin 表示 Central 到 Realm 的账号验证响应。
	MsgCentral2RAccountLogin uint16 = 50002
)

// R2CentralAccountLogin 表示账号登录校验请求。
type R2CentralAccountLogin struct {
	RpcId    uint32 `json:"rpc_id"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Central2RAccountLogin 表示账号登录校验响应。
type Central2RAccountLogin struct {
	RpcId       uint32 `json:"rpc_id"`
	Error       int32  `json:"error"`
	Message     string `json:"message,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
}
