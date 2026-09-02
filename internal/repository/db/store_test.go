package db_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/repository/sqlite"
)

func TestStoreWithWriteLockSerializesSQLiteWrites(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if _, err := store.DB.ExecContext(ctx, "CREATE TABLE counters (name TEXT PRIMARY KEY, value INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create counters table: %v", err)
	}
	if _, err := store.DB.ExecContext(ctx, "INSERT INTO counters (name, value) VALUES ('hits', 0)"); err != nil {
		t.Fatalf("seed counters table: %v", err)
	}

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.WithWriteTx(ctx, func(q db.Querier) error {
				var current int
				if err := q.QueryRowContext(ctx, "SELECT value FROM counters WHERE name = 'hits'").Scan(&current); err != nil {
					return err
				}
				_, err := q.ExecContext(ctx, "UPDATE counters SET value = ? WHERE name = 'hits'", current+1)
				return err
			})
			if err != nil {
				t.Errorf("WithWriteTx() error = %v", err)
			}
		}()
	}
	wg.Wait()

	var got int
	if err := store.DB.QueryRowContext(ctx, "SELECT value FROM counters WHERE name = 'hits'").Scan(&got); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if got != 10 {
		t.Fatalf("counter = %d, want 10", got)
	}
}

func TestStoreFlushCheckpointsWAL(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.DB.ExecContext(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.DB.ExecContext(ctx, "INSERT INTO items (name) VALUES ('alpha')"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	walPath := dbPath + "-wal"
	info, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("expected WAL file after write: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("WAL file is empty before flush")
	}

	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	info, err = os.Stat(walPath)
	if err == nil && info.Size() != 0 {
		t.Fatalf("WAL size after flush = %d, want 0", info.Size())
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
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
	if name != "alpha" {
		t.Fatalf("name = %q, want alpha", name)
	}
}

func TestStoreCloseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := store.Flush(ctx); err != nil {
		t.Fatalf("Flush() after Close() error = %v", err)
	}
}

func TestStoreFlushNoOpWhenNotSQLite(t *testing.T) {
	store := db.NewStore(nil, db.DialectPostgres)
	if err := store.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
