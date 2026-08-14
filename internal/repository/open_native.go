//go:build !(js && wasm)

package repository

import (
	"context"
	"fmt"

	"github.com/kilhog-io/kilhog/internal/repository/d1"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/repository/postgres"
	"github.com/kilhog-io/kilhog/internal/repository/sqlite"
)

func openStore(ctx context.Context, cfg db.Config) (*db.Store, error) {
	switch cfg.Driver {
	case db.DialectSQLite:
		return sqlite.Open(ctx, cfg.DSN)
	case db.DialectPostgres:
		return postgres.Open(ctx, cfg.DSN)
	case db.DialectD1:
		return d1.Open(ctx, cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}
