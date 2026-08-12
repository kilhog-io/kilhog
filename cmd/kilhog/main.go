package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/kilhog-io/kilhog/internal/handler"
	kilhoglog "github.com/kilhog-io/kilhog/internal/log"
	"github.com/kilhog-io/kilhog/internal/metrics"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func main() {
	if _, err := kilhoglog.InitFromEnv(); err != nil {
		slog.Error("invalid log configuration", "error", err)
		os.Exit(1)
	}

	host := envOrDefault("KILHOG_HOST", "0.0.0.0")
	port := envOrDefault("KILHOG_PORT", "8080")
	addr := fmt.Sprintf("%s:%s", host, port)

	cfg, err := db.ConfigFromEnv()
	if err != nil {
		slog.Error("database config failed", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	repos, err := repository.Open(ctx, cfg)
	if err != nil {
		slog.Error("database init failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := repos.Close(); err != nil {
			slog.Warn("database close failed", "error", err)
		}
	}()

	metricsProvider, err := metrics.Setup(ctx)
	if err != nil {
		slog.Error("metrics init failed", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := metricsProvider.Shutdown(shutdownCtx); err != nil {
			slog.Warn("metrics shutdown failed", "error", err)
		}
	}()

	refreshInterval, err := metrics.RefreshIntervalFromEnv()
	if err != nil {
		slog.Error("metrics refresh config failed", "error", err)
		os.Exit(1)
	}

	refreshCtx, stopRefresh := context.WithCancel(context.Background())
	defer stopRefresh()

	countSource := resourceCountSource(repos)
	if err := metricsProvider.Resources.Refresh(ctx, countSource); err != nil {
		slog.Error("metrics seed failed", "error", err)
		os.Exit(1)
	}
	slog.Info("metrics seeded",
		"networks", metricsProvider.Resources.NetworkCount(),
		"subnets", metricsProvider.Resources.SubnetCount(),
		"refresh_interval", refreshInterval.String(),
	)
	metricsProvider.Resources.StartRefresh(refreshCtx, refreshInterval, countSource)

	apiKey := os.Getenv("KILHOG_API_KEY")
	resourceMetrics := metricsProvider.Resources
	authCfg := service.AuthConfig{
		APIKey:         apiKey,
		BootstrapToken: os.Getenv("KILHOG_BOOTSTRAP_TOKEN"),
		PublicURL:      os.Getenv("KILHOG_PUBLIC_URL"),
		SessionTTL:     sessionTTLFromEnv(),
	}
	authService := service.NewAuthService(repos.Users, repos.IdentityPools, repos.Sessions, repos.OIDCStates, authCfg)
	userService := service.NewUserService(repos.Users)
	poolService := service.NewIdentityPoolService(repos.IdentityPools)

	server := &http.Server{
		Addr: addr,
		Handler: handler.NewRouter(handler.Dependencies{
			Store: repos.Store,
			NetworkService: service.NewNetworkService(
				repos.Networks,
				repos.Subnets,
				service.WithNetworkMetrics(resourceMetrics),
			),
			SubnetService: service.NewSubnetService(
				repos.Subnets,
				repos.Networks,
				service.WithSubnetMetrics(resourceMetrics),
			),
			AuthService:         authService,
			UserService:         userService,
			IdentityPoolService: poolService,
			APIKey:              apiKey,
			Metrics:             metricsProvider,
		}),
	}

	slog.Info("kilhog listening",
		"addr", addr,
		"db", cfg.Driver,
		"api_key", boolLabel(apiKey != ""),
		"public_url", authCfg.PublicURL,
	)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down")
	stopRefresh()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}

func resourceCountSource(repos *repository.Repositories) metrics.ResourceCountSource {
	return func(ctx context.Context) (metrics.ResourceCounts, error) {
		networks, err := repos.Networks.Count(ctx)
		if err != nil {
			return metrics.ResourceCounts{}, fmt.Errorf("count networks: %w", err)
		}
		subnets, err := repos.Subnets.Count(ctx)
		if err != nil {
			return metrics.ResourceCounts{}, fmt.Errorf("count subnets: %w", err)
		}
		return metrics.ResourceCounts{Networks: networks, Subnets: subnets}, nil
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func sessionTTLFromEnv() time.Duration {
	raw := os.Getenv("KILHOG_SESSION_TTL")
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
