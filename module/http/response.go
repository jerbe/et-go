package http

import (
	"encoding/json"
	"log/slog"
	nethttp "net/http"
)

// HttpResponse 表示通用 HTTP 响应。
type HttpResponse struct {
	RequestId string `json:"RequestId"`
	Error     int    `json:"Error"`
	Message   string `json:"Message"`
}

// HttpLoginResponse 表示登录响应。
type HttpLoginResponse struct {
	HttpResponse
	AccessToken string `json:"AccessToken,omitempty"`
}

// HttpRegisterResponse 表示注册响应。
type HttpRegisterResponse struct {
	HttpResponse
}

// AreaInfo 表示一个大区信息。
type AreaInfo struct {
	Id        int32  `json:"Id"`
	Name      string `json:"Name"`
	ServerURL string `json:"ServerURL"`
}

// HttpAreaListResponse 表示大区列表响应。
type HttpAreaListResponse struct {
	HttpResponse
	Areas []AreaInfo `json:"Areas"`
}

// WriteJSON 写入统一 JSON 响应。
func WriteJSON(w nethttp.ResponseWriter, statusCode int, data any) {
	if w == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("HTTP JSON encode failed", "err", err)
	}
}

// WriteError 写入统一错误响应。
func WriteError(w nethttp.ResponseWriter, statusCode int, errCode int, message string) {
	WriteJSON(w, statusCode, HttpResponse{
		Error:   errCode,
		Message: message,
	})
}

// WriteNotFound 返回与目标项目一致的 404 JSON 包体。
func WriteNotFound(w nethttp.ResponseWriter) {
	WriteError(w, nethttp.StatusOK, nethttp.StatusNotFound, "404 Page Not Found")
}

// WriteInternalServerError 返回与目标项目一致的 500 JSON 包体。
func WriteInternalServerError(w nethttp.ResponseWriter) {
	WriteError(w, nethttp.StatusOK, nethttp.StatusInternalServerError, "500 Internal Server Error")
}
