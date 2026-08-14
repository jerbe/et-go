package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
	"github.com/jerbe/et-go/module/login"
)

// LoginRequest 表示 HTTP 登录请求体。
type LoginRequest struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// HttpPostLoginHandler 处理 `/login`。
type HttpPostLoginHandler struct{}

// Handle 执行登录逻辑。
func (h *HttpPostLoginHandler) Handle(scene *ecs.Scene, req *nethttp.Request, resp nethttp.ResponseWriter) error {
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
	var body LoginRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Username) == "" || body.Password == "" {
		if err := recordLoginAudit(scene, req, body.Username, 0, false, "invalid_request"); err != nil {
			return err
		}
		WriteError(resp, nethttp.StatusOK, nethttp.StatusBadRequest, "400 Bad Request")
		return nil
	}
	if scene != nil {
		if component, ok := scene.GetComponent("LoginRateLimiterComponent"); ok {
			limiter, valid := component.(interface {
				AllowContext(context.Context, string) (bool, error)
			})
			if !valid || limiter == nil {
				return ErrLoginRateLimiterInvalid
			}
			allowed, err := limiter.AllowContext(req.Context(), loginRateLimitKey(req.RemoteAddr, body.Username))
			if err != nil {
				return err
			}
			if !allowed {
				if err := recordLoginAudit(scene, req, body.Username, 0, false, "rate_limited"); err != nil {
					return err
				}
				WriteError(resp, nethttp.StatusOK, nethttp.StatusTooManyRequests, "429 Too Many Requests")
				return nil
			}
		}
	}
	repository, err := accountRepositoryFromScene(scene)
	if err != nil {
		return errors.Join(err, recordLoginAudit(scene, req, body.Username, 0, false, "repository_unavailable"))
	}
	account, err := repository.FindByUsername(ctx, body.Username)
	if err != nil {
		return errors.Join(err, recordLoginAudit(scene, req, body.Username, 0, false, "account_lookup_failed"))
	}
	if account == nil {
		if err := recordLoginAudit(scene, req, body.Username, 0, false, "invalid_credentials"); err != nil {
			return err
		}
		WriteJSON(resp, nethttp.StatusOK, HttpLoginResponse{
			HttpResponse: HttpResponse{
				Error:   int(login.ERR_UsernameOrPasswordIncorrectError),
				Message: central.ErrUsernameOrPasswordIncorrect.Error(),
			},
		})
		return nil
	}
	valid, needsUpgrade, err := central.VerifyPassword(body.Password, account.PasswordHash, account.PasswordAlgorithm)
	if err != nil {
		return errors.Join(err, recordLoginAudit(scene, req, body.Username, account.Id, false, "password_verification_failed"))
	}
	if !valid {
		if err := recordLoginAudit(scene, req, body.Username, account.Id, false, "invalid_credentials"); err != nil {
			return err
		}
		WriteJSON(resp, nethttp.StatusOK, HttpLoginResponse{
			HttpResponse: HttpResponse{
				Error:   int(login.ERR_UsernameOrPasswordIncorrectError),
				Message: central.ErrUsernameOrPasswordIncorrect.Error(),
			},
		})
		return nil
	}
	if needsUpgrade {
		passwordHash, err := central.HashPassword(body.Password)
		if err != nil {
			return errors.Join(err, recordLoginAudit(scene, req, body.Username, account.Id, false, "password_upgrade_hash_failed"))
		}
		if err := repository.UpdatePassword(ctx, account.Id, passwordHash, central.PasswordAlgorithmArgon2id); err != nil {
			return errors.Join(err, recordLoginAudit(scene, req, body.Username, account.Id, false, "password_upgrade_persist_failed"))
		}
	}
	token, err := login.GenerateAccessToken(account.Id)
	if err != nil {
		return errors.Join(err, recordLoginAudit(scene, req, body.Username, account.Id, false, "token_generation_failed"))
	}
	if err := recordLoginAudit(scene, req, body.Username, account.Id, true, "success"); err != nil {
		return err
	}
	WriteJSON(resp, nethttp.StatusOK, HttpLoginResponse{
		HttpResponse: HttpResponse{},
		AccessToken:  token,
	})
	return nil
}
