package http

import (
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
)

type wrappedDuplicateAccountRepo struct{}

func (wrappedDuplicateAccountRepo) FindByUsername(context.Context, string) (*central.CAccount, error) {
	return nil, nil
}

func (wrappedDuplicateAccountRepo) CreateAccount(context.Context, string, string, string) (int64, error) {
	return 0, fmt.Errorf("storage conflict: %w", ErrUsernameAlreadyRegistered)
}

func (wrappedDuplicateAccountRepo) UpdatePassword(context.Context, int64, string, string) error {
	return nil
}

func TestRegisterHandler(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	scene.AddComponent(&repoComponent{repo: repo})

	body, _ := json.Marshal(RegisterRequest{Username: "user", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/register", bytesReader(body))
	rec := httptest.NewRecorder()
	if err := (&HttpPostRegisterHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	var resp HttpRegisterResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode err = %v", err)
	}
	if resp.Error != 0 || resp.Message != "" {
		t.Fatalf("unexpected resp %+v", resp)
	}
	account, err := repo.FindByUsername(context.Background(), "user")
	if err != nil {
		t.Fatalf("FindByUsername err = %v", err)
	}
	if account == nil || account.Id <= 1000000 {
		t.Fatalf("unexpected stored account %+v", account)
	}
	if account.PasswordAlgorithm != central.PasswordAlgorithmArgon2id {
		t.Fatalf("new account password algorithm = %q", account.PasswordAlgorithm)
	}
	valid, needsUpgrade, err := central.VerifyPassword("pass", account.PasswordHash, account.PasswordAlgorithm)
	if err != nil || !valid || needsUpgrade {
		t.Fatalf("new account password valid=%v upgrade=%v err=%v", valid, needsUpgrade, err)
	}

	body, _ = json.Marshal(RegisterRequest{Username: "user", Password: "pass"})
	req = httptest.NewRequest(nethttp.MethodPost, "/register", bytesReader(body))
	rec = httptest.NewRecorder()
	if err := (&HttpPostRegisterHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("duplicate err = %v", err)
	}
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode duplicate err = %v", err)
	}
	if resp.Error != ERR_UsernameIsExistsError || resp.Message != "Username is already exists" {
		t.Fatalf("unexpected duplicate resp %+v", resp)
	}

	id1, _ := repo.CreateAccount(context.Background(), "user-1", "x", central.PasswordAlgorithmMD5)
	id2, _ := repo.CreateAccount(context.Background(), "user-2", "y", central.PasswordAlgorithmMD5)
	if id2 <= id1 {
		t.Fatalf("id2=%d id1=%d", id2, id1)
	}
}

func TestRegisterHandlerRejectsEmptyCredentials(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	scene.AddComponent(&repoComponent{repo: newMemoryAccountRepo()})
	body, _ := json.Marshal(RegisterRequest{Username: "", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/register", bytesReader(body))
	rec := httptest.NewRecorder()

	if err := (&HttpPostRegisterHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle empty credentials error = %v", err)
	}
	var resp HttpRegisterResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode empty credentials response: %v", err)
	}
	if resp.Error != nethttp.StatusBadRequest {
		t.Fatalf("empty credentials error = %d, want %d", resp.Error, nethttp.StatusBadRequest)
	}
}

func TestRegisterHandlerRecognizesWrappedDuplicateError(t *testing.T) {
	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	scene.AddComponent(&repoComponent{repo: wrappedDuplicateAccountRepo{}})
	body, _ := json.Marshal(RegisterRequest{Username: "user", Password: "pass"})
	req := httptest.NewRequest(nethttp.MethodPost, "/register", bytesReader(body))
	rec := httptest.NewRecorder()

	if err := (&HttpPostRegisterHandler{}).Handle(scene, req, rec); err != nil {
		t.Fatalf("Handle wrapped duplicate error = %v", err)
	}
	var resp HttpRegisterResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode wrapped duplicate response: %v", err)
	}
	if resp.Error != ERR_UsernameIsExistsError {
		t.Fatalf("wrapped duplicate error = %d, want %d", resp.Error, ERR_UsernameIsExistsError)
	}
}
