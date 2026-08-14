package http

import (
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
)

// RegisterRequest 表示 HTTP 注册请求体。
type RegisterRequest struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// HttpPostRegisterHandler 处理 `/register`。
type HttpPostRegisterHandler struct{}

// Handle 执行注册逻辑。
func (h *HttpPostRegisterHandler) Handle(scene *ecs.Scene, req *nethttp.Request, resp nethttp.ResponseWriter) error {
	if req == nil {
		return ErrRequestMissing
	}
	if resp == nil {
		return ErrResponseWriterMissing
	}
	if req.Body == nil {
		return ErrRequestMissing
	}
	if req.Method != nethttp.MethodPost {
		WriteNotFound(resp)
		return nil
	}
	ctx := req.Context()
	var body RegisterRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Username) == "" || body.Password == "" {
		WriteError(resp, nethttp.StatusOK, nethttp.StatusBadRequest, "400 Bad Request")
		return nil
	}
	repository, err := accountRepositoryFromScene(scene)
	if err != nil {
		return err
	}
	passwordHash, err := central.HashPassword(body.Password)
	if err != nil {
		return err
	}
	_, err = repository.CreateAccount(ctx, body.Username, passwordHash, central.PasswordAlgorithmArgon2id)
	if err != nil {
		if errors.Is(err, ErrUsernameAlreadyRegistered) {
			WriteError(resp, nethttp.StatusOK, ERR_UsernameIsExistsError, "Username is already exists")
			return nil
		}
		return err
	}
	WriteJSON(resp, nethttp.StatusOK, HttpRegisterResponse{
		HttpResponse: HttpResponse{},
	})
	return nil
}
