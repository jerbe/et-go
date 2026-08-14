package http

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
	"github.com/jerbe/et-go/module/login"
)

type memoryAccountRepo struct {
	mu      sync.Mutex
	nextID  int64
	account map[string]*central.CAccount
}

func newMemoryAccountRepo() *memoryAccountRepo {
	return &memoryAccountRepo{
		nextID:  1000000,
		account: make(map[string]*central.CAccount),
	}
}

func (r *memoryAccountRepo) FindByUsername(_ context.Context, username string) (*central.CAccount, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account, ok := r.account[username]; ok {
		copy := *account
		return &copy, nil
	}
	return nil, nil
}

func (r *memoryAccountRepo) CreateAccount(_ context.Context, username string, passwordHash string, algorithm string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.account[username]; ok {
		return 0, ErrUsernameAlreadyRegistered
	}
	r.nextID++
	r.account[username] = &central.CAccount{
		Id:                r.nextID,
		Username:          username,
		PasswordHash:      passwordHash,
		PasswordAlgorithm: algorithm,
	}
	return r.nextID, nil
}

func (r *memoryAccountRepo) UpdatePassword(_ context.Context, accountID int64, passwordHash string, algorithm string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, account := range r.account {
		if account.Id == accountID {
			account.PasswordHash = passwordHash
			account.PasswordAlgorithm = algorithm
			return nil
		}
	}
	return central.ErrAccountStoreMissing
}

type repoComponent struct {
	ecs.BaseComponent
	repo AccountRepository
}

func (c *repoComponent) Type() string { return "HTTPAccountRepositoryComponent" }
func (c *repoComponent) Repository() AccountRepository {
	return c.repo
}

func TestLoginHandler(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	id, err := repo.CreateAccount(context.Background(), "user", central.MD5Hash("pass"), central.PasswordAlgorithmMD5)
	if err != nil {
		t.Fatalf("CreateAccount err = %v", err)
	}
	scene.AddComponent(&repoComponent{repo: repo})

	body, _ := json.Marshal(LoginRequest{Username: "user", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	rec := httptest.NewRecorder()
	if err := (&HttpPostLoginHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var resp HttpLoginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode err = %v", err)
	}
	accountID, err := login.VerifyAccessToken(resp.AccessToken)
	if err != nil || accountID != id {
		t.Fatalf("token accountID=%d err=%v", accountID, err)
	}
	account, err := repo.FindByUsername(context.Background(), "user")
	if err != nil {
		t.Fatalf("FindByUsername after login err = %v", err)
	}
	if account == nil || account.PasswordAlgorithm != central.PasswordAlgorithmArgon2id {
		t.Fatalf("legacy password was not upgraded: %+v", account)
	}

	body, _ = json.Marshal(LoginRequest{Username: "user", Password: "bad"})
	req = httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	rec = httptest.NewRecorder()
	if err := (&HttpPostLoginHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle wrong password err = %v", err)
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode err = %v", err)
	}
	if resp.Error != int(login.ERR_UsernameOrPasswordIncorrectError) {
		t.Fatalf("resp.Error = %d", resp.Error)
	}
	if resp.Message != "Username or password is incorrect" {
		t.Fatalf("resp.Message = %q", resp.Message)
	}
}

func TestLoginHandlerRejectsEmptyCredentials(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	scene.AddComponent(&repoComponent{repo: newMemoryAccountRepo()})
	body, _ := json.Marshal(LoginRequest{Username: " ", Password: ""})
	req := httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	rec := httptest.NewRecorder()

	if err := (&HttpPostLoginHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle empty credentials error = %v", err)
	}
	var resp HttpLoginResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode empty credentials response: %v", err)
	}
	if resp.Error != nethttp.StatusBadRequest {
		t.Fatalf("empty credentials error = %d, want %d", resp.Error, nethttp.StatusBadRequest)
	}
}

func TestLoginHandlerEnforcesConfiguredRateLimit(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	if _, err := repo.CreateAccount(context.Background(), "user", central.MD5Hash("pass"), central.PasswordAlgorithmMD5); err != nil {
		t.Fatalf("CreateAccount err = %v", err)
	}
	scene.AddComponent(&repoComponent{repo: repo})
	limiter, err := NewLoginRateLimiterComponent(1, 0)
	if err != nil {
		t.Fatalf("NewLoginRateLimiterComponent error = %v", err)
	}
	scene.AddComponent(limiter)

	body, _ := json.Marshal(LoginRequest{Username: "user", Password: "pass"})
	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
		req.RemoteAddr = "192.0.2.10:1234"
		rec := httptest.NewRecorder()
		if err := (&HttpPostLoginHandler{}).Handle(scene, req, rec); err != nil {
			t.Fatalf("Handle attempt %d error = %v", attempt+1, err)
		}
		var resp HttpLoginResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("Decode attempt %d error = %v", attempt+1, err)
		}
		if attempt == 0 && resp.Error != 0 {
			t.Fatalf("first login response = %+v, want success", resp)
		}
		if attempt == 1 && resp.Error != nethttp.StatusTooManyRequests {
			t.Fatalf("second login response = %+v, want rate limit %d", resp, nethttp.StatusTooManyRequests)
		}
	}
}

func TestLoginHandlerRecordsAuditEvents(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	if _, err := repo.CreateAccount(context.Background(), "user", central.MD5Hash("pass"), central.PasswordAlgorithmMD5); err != nil {
		t.Fatalf("CreateAccount err = %v", err)
	}
	scene.AddComponent(&repoComponent{repo: repo})
	sink := &recordingLoginAuditSink{}
	scene.AddComponent(&LoginAuditComponent{Sink: sink})

	body, _ := json.Marshal(LoginRequest{Username: "user", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	req.RemoteAddr = "192.0.2.20:4321"
	if err := (&HttpPostLoginHandler{}).Handle(scene, req, httptest.NewRecorder()); err != nil {
		t.Fatalf("successful login error = %v", err)
	}

	body, _ = json.Marshal(LoginRequest{Username: "user", Password: "bad"})
	req = httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	req.RemoteAddr = "192.0.2.20:4321"
	if err := (&HttpPostLoginHandler{}).Handle(scene, req, httptest.NewRecorder()); err != nil {
		t.Fatalf("failed login error = %v", err)
	}

	events := sink.snapshot()
	if len(events) != 2 {
		t.Fatalf("audit events = %+v, want two events", events)
	}
	if !events[0].Success || events[0].Reason != "success" || events[0].AccountID <= 0 {
		t.Fatalf("success audit event = %+v", events[0])
	}
	if events[1].Success || events[1].Reason != "invalid_credentials" {
		t.Fatalf("failure audit event = %+v", events[1])
	}
	if events[0].RemoteAddr != "192.0.2.20:4321" {
		t.Fatalf("audit remote address = %q", events[0].RemoteAddr)
	}
}

func TestLoginHandlerPropagatesAuditFailure(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	if _, err := repo.CreateAccount(context.Background(), "user", central.MD5Hash("pass"), central.PasswordAlgorithmMD5); err != nil {
		t.Fatalf("CreateAccount err = %v", err)
	}
	scene.AddComponent(&repoComponent{repo: repo})
	sink := &recordingLoginAuditSink{err: errors.New("audit unavailable")}
	scene.AddComponent(&LoginAuditComponent{Sink: sink})

	body, _ := json.Marshal(LoginRequest{Username: "user", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/login", bytesReader(body))
	if err := (&HttpPostLoginHandler{}).Handle(scene, req, httptest.NewRecorder()); err == nil {
		t.Fatal("Handle should return audit persistence error")
	}
}
