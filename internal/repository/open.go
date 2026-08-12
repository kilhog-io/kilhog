package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/repository/migration"
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
		slog.Info("running database migrations", "driver", cfg.Driver)
		if err := migration.NewRunner(store).Upgrade(ctx); err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("run database migrations: %w", err)
		}
		slog.Info("database migrations complete", "driver", cfg.Driver)
	}

	return &Repositories{
		Store:    store,
		Networks: NewNetworkRepository(store),
		Subnets:  NewSubnetRepository(store),
	}, nil
}

func (r *Repositories) Close() error {
	if r.Store == nil {
		return nil
	}
	return r.Store.Close()
}
