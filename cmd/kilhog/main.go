package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/kilhog-io/kilhog/internal/handler"
)

func main() {
	host := envOrDefault("KILHOG_HOST", "0.0.0.0")
	port := envOrDefault("KILHOG_PORT", "8080")
	addr := fmt.Sprintf("%s:%s", host, port)

	server := &http.Server{
		Addr:    addr,
		Handler: handler.NewRouter(),
	}

	log.Printf("kilhog listening on %s", addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
