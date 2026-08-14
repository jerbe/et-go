package router

import (
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"testing"
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

func TestRouterManagerFiberInitAlignment(t *testing.T) {
	world := ecs.NewWorld()
	manager := fiber.NewManager(context.Background(), world, nil)
	oldConfig := config.GetGlobal()
	config.SetGlobal(&config.Config{
		Machines:  []config.StartMachineConfig{{ID: 1, InnerIP: "127.0.0.1", OuterIP: "127.0.0.1"}},
		Processes: []config.StartProcessConfig{{ID: 1, MachineID: 1}},
		Scenes:    []config.StartSceneConfig{{ID: 9001, ProcessID: 1, Zone: 1, SceneType: "Router", Name: "Router", OuterPort: 0}},
		Zones:     []config.StartZoneConfig{{ID: 1, Name: "test", DBName: "test", DBAddr: "mongodb://127.0.0.1:27017"}},
	})
	t.Cleanup(func() { config.SetGlobal(oldConfig) })
	t.Cleanup(func() {
		manager.StopAll()
		world.Shutdown()
	})

	routerFiber := manager.Create(ecs.SceneTypeRouter, 1, 1, nil)
	if routerFiber == nil {
		t.Fatal("expected router manager fiber")
	}

	scene := routerFiber.Root()
	if _, ok := scene.GetComponent("DBManagerComponent"); ok {
		t.Fatal("router manager should not install DBManagerComponent")
	}
	if _, ok := scene.GetComponent("MailBox"); !ok {
		t.Fatal("router manager should install MailBox")
	}
	if _, ok := scene.GetComponent("MessageSender"); !ok {
		t.Fatal("router manager should install MessageSender")
	}

	component, ok := scene.GetComponent("HttpComponent")
	if !ok || component == nil {
		t.Fatal("router manager should install HttpComponent")
	}
	httpComponent, ok := component.(interface {
		Addr() string
	})
	if !ok {
		t.Fatal("unexpected HttpComponent type")
	}

	baseURL := "http://" + httpComponent.Addr()
	var (
		resp *nethttp.Response
		err  error
	)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resp, err = nethttp.Get(baseURL + "/register")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("request router manager route err = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response err = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response err = %v", err)
	}
	if resp.StatusCode != nethttp.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, nethttp.StatusOK)
	}
	if payload["Error"] != float64(404) || payload["Message"] != "404 Page Not Found" {
		t.Fatalf("unexpected /register response %v", payload)
	}
}
