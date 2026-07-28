package repository

import (
	"context"
	"fmt"

	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/repository/migration"
	"github.com/kilhog-io/kilhog/internal/repository/postgres"
	"github.com/kilhog-io/kilhog/internal/repository/sqlite"
	"github.com/kilhog-io/kilhog/internal/service"
)

type Repositories struct {
	Store    *db.Store
	Networks service.NetworkRepository
	Subnets  service.SubnetRepository
}

func Open(ctx context.Context, cfg db.Config) (*Repositories, error) {
	store, err := openStore(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.AutoMigrate {
		if err := migration.NewRunner(store).Upgrade(ctx); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("run database migrations: %w", err)
		}
	}

	return &Repositories{
		Store:    store,
		Networks: NewNetworkRepository(store),
		Subnets:  NewSubnetRepository(store),
	}, nil
}

func openStore(ctx context.Context, cfg db.Config) (*db.Store, error) {
	switch cfg.Driver {
	case db.DialectSQLite:
		return sqlite.Open(ctx, cfg.DSN)
	case db.DialectPostgres:
		return postgres.Open(ctx, cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}
}

func (r *Repositories) Close() error {
	if r.Store == nil {
		return nil
	}
	return r.Store.Close()
}
