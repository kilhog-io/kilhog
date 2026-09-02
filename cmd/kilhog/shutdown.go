package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/kilhog-io/kilhog/internal/repository/db"
)

// cloudRunTerminationGrace is how long Cloud Run waits after SIGTERM before
// SIGKILL. The process must finish HTTP drain, SQLite WAL checkpoint, and
// database close within this window.
//
// https://cloud.google.com/run/docs/samples/cloudrun-sigterm-handler
const cloudRunTerminationGrace = 10 * time.Second

// gracefulShutdown stops the HTTP server, checkpoints SQLite, and closes the
// database file. HTTP drain is capped so that DefaultSQLiteFlushTimeout remains
// before Cloud Run's SIGKILL.
func gracefulShutdown(server *http.Server, store *db.Store) error {
	slog.Info("shutting down")

	httpTimeout := cloudRunTerminationGrace - db.DefaultSQLiteFlushTimeout
	if httpTimeout < time.Second {
		httpTimeout = time.Second
	}

	httpCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	if server != nil {
		if err := server.Shutdown(httpCtx); err != nil {
			slog.Error("http server shutdown failed", "error", err)
			if closeErr := server.Close(); closeErr != nil {
				slog.Error("http server close failed", "error", closeErr)
			}
		}
	}

	if store == nil {
		return nil
	}

	flushCtx, flushCancel := context.WithTimeout(context.Background(), db.DefaultSQLiteFlushTimeout)
	defer flushCancel()

	if store.Dialect == db.DialectSQLite {
		slog.Info("synchronizing sqlite database")
	}
	if err := store.CloseContext(flushCtx); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	slog.Info("database closed")
	return nil
}
