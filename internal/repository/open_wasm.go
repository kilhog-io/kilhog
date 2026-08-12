//go:build js && wasm

package repository

import (
	"context"
	"fmt"

	"github.com/kilhog-io/kilhog/internal/repository/d1"
	"github.com/kilhog-io/kilhog/internal/repository/db"
)

func openStore(ctx context.Context, cfg db.Config) (*db.Store, error) {
	switch cfg.Driver {
	case db.DialectD1:
		return d1.Open(ctx, cfg.DSN)
	case db.DialectSQLite, db.DialectPostgres:
		return nil, fmt.Errorf("database driver %q is not available on Cloudflare Workers; use d1", cfg.Driver)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}
