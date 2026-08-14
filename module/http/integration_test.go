package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
)

func TestHTTPComponentIntegration(t *testing.T) {
	old := config.GetGlobal()
	defer config.SetGlobal(old)
	config.SetGlobal(&config.Config{
		Zones: []config.StartZoneConfig{{ID: 1}},
	})

	scene := ecs.NewScene(ecs.SceneTypeHTTP, 1, "http")
	repo := newMemoryAccountRepo()
	scene.AddComponent(&repoComponent{repo: repo})
	component := NewHttpComponent("127.0.0.1:0")
	scene.AddComponent(component)
	if err := component.Start(); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	defer component.OnDestroy()

	time.Sleep(30 * time.Millisecond)

	resp, err := http.Post("http://"+component.Addr()+"/register", "application/json", strings.NewReader(`{"Username":"user","Password":"pass"}`))
	if err != nil {
		t.Fatalf("register err = %v", err)
	}
	defer resp.Body.Close()
	var registerResp HttpRegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResp); err != nil {
		t.Fatalf("Decode register err = %v", err)
	}
	if registerResp.Error != 0 || registerResp.Message != "" {
		t.Fatalf("unexpected register resp %+v", registerResp)
	}
	if account, err := repo.FindByUsername(context.Background(), "user"); err != nil || account == nil {
		t.Fatalf("expected stored account, account=%+v err=%v", account, err)
	}

	resp, err = http.Post("http://"+component.Addr()+"/login", "application/json", strings.NewReader(`{"Username":"user","Password":"pass"}`))
	if err != nil {
		t.Fatalf("login err = %v", err)
	}
	defer resp.Body.Close()
	var loginResp HttpLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("Decode login err = %v", err)
	}
	if loginResp.AccessToken == "" {
		t.Fatal("expected AccessToken")
	}
}

func TestHTTPComponentDoesNotRestartAfterDestroy(t *testing.T) {
	component := NewHttpComponent("127.0.0.1:0")
	component.OnDestroy()

	if err := component.Start(); err != ErrServerClosed {
		t.Fatalf("Start after destroy error = %v, want %v", err, ErrServerClosed)
	}
}
