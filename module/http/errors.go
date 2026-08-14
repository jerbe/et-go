package http

import "errors"

const (
	// ERR_WithException 对齐 ET 全局逻辑异常起始值。
	ERR_WithException = 100000000
	// ERR_UsernameIsExistsError 对齐目标 HTTP 包错误码。
	ERR_UsernameIsExistsError = ERR_WithException + 16*1000 + 1
)

var (
	// ErrRequestMissing 表示 HTTP 请求对象缺失。
	ErrRequestMissing = errors.New("http: request missing")
	// ErrResponseWriterMissing 表示 HTTP 响应写入器缺失。
	ErrResponseWriterMissing = errors.New("http: response writer missing")
	// ErrConfigurationMissing 表示全局启动配置缺失。
	ErrConfigurationMissing = errors.New("http: global configuration missing")
	// ErrServerClosed 表示 HTTP 组件已经销毁。
	ErrServerClosed = errors.New("http: server component closed")
	// ErrTLSConfigurationInvalid 表示 TLS 证书配置不完整。
	ErrTLSConfigurationInvalid = errors.New("http: TLS configuration invalid")
	// ErrTLSCertificateLoad 表示 TLS 证书或私钥加载失败。
	ErrTLSCertificateLoad = errors.New("http: TLS certificate load failed")
)
