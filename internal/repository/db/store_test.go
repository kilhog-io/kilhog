package db_test

import (
	"context"
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
