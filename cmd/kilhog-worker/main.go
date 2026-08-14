//go:build js && wasm

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/kilhog-io/kilhog/internal/handler"
	kilhoglog "github.com/kilhog-io/kilhog/internal/log"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
	"github.com/syumai/workers"
	"github.com/syumai/workers/cloudflare"
)

func main() {
	var (
		once    sync.Once
		router  http.Handler
		initErr error
	)

	// D1 bindings and Worker env vars are only available during request handling,
	// so database open + migrations run lazily on the first request.
	workers.Serve(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			router, initErr = buildRouter()
		})
		if initErr != nil {
			slog.Error("worker init failed", "error", initErr)
			http.Error(w, "worker initialization failed", http.StatusInternalServerError)
			return
		}
		router.ServeHTTP(w, r)
	}))
}

func buildRouter() (http.Handler, error) {
	if _, err := kilhoglog.InitFromEnv(); err != nil {
		// Fall back to cloudflare env if os.Getenv was empty at cold start.
		if level := cloudflare.Getenv("KILHOG_LOG_LEVEL"); level != "" {
			_ = os.Setenv("KILHOG_LOG_LEVEL", level)
			if _, err := kilhoglog.InitFromEnv(); err != nil {
				return nil, fmt.Errorf("invalid log configuration: %w", err)
			}
		} else {
			return nil, fmt.Errorf("invalid log configuration: %w", err)
		}
	}

	cfg, err := workerDBConfig()
	if err != nil {
		return nil, fmt.Errorf("database config: %w", err)
	}

	repos, err := repository.Open(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("database init: %w", err)
	}

	apiKey := envOrDefault("KILHOG_API_KEY", "")
	authCfg := service.AuthConfig{
		APIKey:         apiKey,
		BootstrapToken: envOrDefault("KILHOG_BOOTSTRAP_TOKEN", ""),
		PublicURL:      envOrDefault("KILHOG_PUBLIC_URL", ""),
		SessionTTL:     sessionTTLFromEnv(),
	}
	authService := service.NewAuthService(repos.Users, repos.IdentityPools, repos.Sessions, repos.OIDCStates, authCfg)
	userService := service.NewUserService(repos.Users)
	poolService := service.NewIdentityPoolService(repos.IdentityPools)

	slog.Info("kilhog worker ready", "db", cfg.Driver, "api_key", boolLabel(apiKey != ""))

	return handler.NewRouter(handler.Dependencies{
		Store:               repos.Store,
		NetworkService:      service.NewNetworkService(repos.Networks, repos.Subnets),
		SubnetService:       service.NewSubnetService(repos.Subnets, repos.Networks),
		AuthService:         authService,
		UserService:         userService,
		IdentityPoolService: poolService,
		APIKey:              apiKey,
	}), nil
}

func workerDBConfig() (db.Config, error) {
	driverRaw := envOrDefault("KILHOG_DB_DRIVER", string(db.DialectD1))
	driver, err := db.ParseDialect(driverRaw)
	if err != nil {
		return db.Config{}, err
	}
	if driver != db.DialectD1 {
		return db.Config{}, fmt.Errorf("cloudflare worker only supports KILHOG_DB_DRIVER=d1, got %q", driver)
	}

	autoMigrate := true
	if raw := envOrDefault("KILHOG_AUTO_MIGRATE", "true"); raw != "" {
		autoMigrate, err = strconv.ParseBool(raw)
		if err != nil {
			return db.Config{}, fmt.Errorf("parse KILHOG_AUTO_MIGRATE: %w", err)
		}
	}

	return db.Config{
		Driver:      driver,
		DSN:         envOrDefault("KILHOG_DB_DSN", "DB"),
		AutoMigrate: autoMigrate,
	}, nil
}

func envOrDefault(key, fallback string) string {
	if value := cloudflare.Getenv(key); value != "" {
		return value
	}
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func sessionTTLFromEnv() time.Duration {
	raw := envOrDefault("KILHOG_SESSION_TTL", "")
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		return time.Duration(secs) * time.Second
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	slog.Warn("invalid KILHOG_SESSION_TTL, using default", "value", raw)
	return 0
}

func boolLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "missing"
}
