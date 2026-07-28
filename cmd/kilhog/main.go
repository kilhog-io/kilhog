package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kilhog-io/kilhog/internal/handler"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func main() {
	host := envOrDefault("KILHOG_HOST", "0.0.0.0")
	port := envOrDefault("KILHOG_PORT", "8080")
	addr := fmt.Sprintf("%s:%s", host, port)

	cfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatalf("database config failed: %v", err)
	}

	ctx := context.Background()
	repos, err := repository.Open(ctx, cfg)
	if err != nil {
		log.Fatalf("database init failed: %v", err)
	}
	defer func() {
		if err := repos.Close(); err != nil {
			log.Printf("database close failed: %v", err)
		}
	}()

	server := &http.Server{
		Addr: addr,
		Handler: handler.NewRouter(handler.Dependencies{
			Store:          repos.Store,
			NetworkService: service.NewNetworkService(repos.Networks, repos.Subnets),
			SubnetService:  service.NewSubnetService(repos.Subnets, repos.Networks),
		}),
	}

	log.Printf("kilhog listening on %s (db=%s)", addr, cfg.Driver)

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
