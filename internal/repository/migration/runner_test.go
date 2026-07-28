package migration_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kilhog-io/kilhog/internal/repository/migration"
	"github.com/kilhog-io/kilhog/internal/repository/sqlite"
)

func TestRunnerUpgradeIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	runner := migration.NewRunner(store)

	if err := runner.Upgrade(ctx); err != nil {
		t.Fatalf("first Upgrade() error = %v", err)
	}
	if err := runner.Upgrade(ctx); err != nil {
		t.Fatalf("second Upgrade() error = %v", err)
	}

	var count int
	if err := store.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration count = %d, want 1", count)
	}
}

func TestRunnerDowngrade(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")

	store, err := sqlite.Open(ctx, "file:"+dbPath+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	runner := migration.NewRunner(store)
	if err := runner.Upgrade(ctx); err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}
	if err := runner.Downgrade(ctx, 0); err != nil {
		t.Fatalf("Downgrade() error = %v", err)
	}

	var exists int
	err = store.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'networks'
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("check networks table: %v", err)
	}
	if exists != 0 {
		t.Fatalf("networks table still exists after downgrade")
	}
}
