package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kilhog-io/kilhog/internal/repository/sqlite"
)

func TestGracefulShutdownStopsHTTPAndClosesSQLite(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if _, err := store.DB.ExecContext(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.DB.ExecContext(ctx, "INSERT INTO items (name) VALUES ('kept')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(ln)
	}()

	baseURL := "http://" + ln.Addr().String()
	waitUntilReady(t, baseURL)

	if err := gracefulShutdown(server, store); err != nil {
		t.Fatalf("gracefulShutdown() error = %v", err)
	}

	select {
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not stop")
	}

	resp, err := http.Get(baseURL)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		t.Fatal("expected connection error after shutdown")
	}

	if store.DB != nil {
		t.Fatal("store.DB still open after shutdown")
	}

	walPath := dbPath + "-wal"
	if info, err := os.Stat(walPath); err == nil && info.Size() != 0 {
		t.Fatalf("WAL size after shutdown = %d, want 0", info.Size())
	}

	reopened, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var name string
	if err := reopened.DB.QueryRowContext(ctx, "SELECT name FROM items WHERE id = 1").Scan(&name); err != nil {
		t.Fatalf("read after reopen: %v", err)
	}
	if name != "kept" {
		t.Fatalf("name = %q, want kept", name)
	}
}

func waitUntilReady(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get(url)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
