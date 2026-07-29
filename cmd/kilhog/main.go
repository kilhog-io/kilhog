package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kilhog-io/kilhog/internal/handler"
	kilhoglog "github.com/kilhog-io/kilhog/internal/log"
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

	apiKey := os.Getenv("KILHOG_API_KEY")

	server := &http.Server{
		Addr: addr,
		Handler: handler.NewRouter(handler.Dependencies{
			Store:          repos.Store,
			NetworkService: service.NewNetworkService(repos.Networks, repos.Subnets),
			SubnetService:  service.NewSubnetService(repos.Subnets, repos.Networks),
			APIKey:         apiKey,
		}),
	}

	if apiKey != "" {
		slog.Info("kilhog listening", "addr", addr, "db", cfg.Driver, "api_key", "enabled")
	} else {
		slog.Info("kilhog listening", "addr", addr, "db", cfg.Driver, "api_key", "disabled")
	}

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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
