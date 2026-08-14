package http

import (
	"time"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/timer"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeHTTP, initHTTPFiber)
}

func initHTTPFiber(f *fiber.Fiber) error {
	scene := f.Root()
	scene.AddComponent(&timer.TimerComponent{})
	scene.AddComponent(&coroutinelock.CoroutineLockComponent{})
	dbManager := &db.DBManagerComponent{}
	if cfg := config.GetGlobal(); cfg != nil {
		dbManager.SetConfig(cfg)
	}
	scene.AddComponent(dbManager)
	if config.GetGlobal() != nil {
		auditSink, err := NewDBManagerLoginAuditSink(dbManager, scene.Zone())
		if err != nil {
			return err
		}
		scene.AddComponent(&LoginAuditComponent{Sink: auditSink})
	}
	addr, err := network.ResolveSceneListenAddr(scene, false)
	if err != nil {
		return err
	}
	httpComponent := NewHttpComponent(addr)
	if cfg := config.GetGlobal(); cfg != nil {
		httpComponent.SetCORSAllowedOrigins(cfg.Security.CORSAllowedOrigins)
		if err := httpComponent.ConfigureTLS(
			cfg.Security.HTTPTLSCertFile,
			cfg.Security.HTTPTLSKeyFile,
			cfg.Security.HTTPRequireTLS,
		); err != nil {
			return err
		}
		if cfg.Security.LoginRateLimitPerMinute > 0 {
			limiter, err := NewDBManagerLoginRateLimiterComponent(
				dbManager,
				scene.Zone(),
				cfg.Security.LoginRateLimitPerMinute,
				time.Minute,
			)
			if err != nil {
				return err
			}
			scene.AddComponent(limiter)
		}
	}
	scene.AddComponent(httpComponent)
	if err := httpComponent.Start(); err != nil {
		return err
	}
	actor.UpdateSceneRegistry(scene)
	return nil
}
