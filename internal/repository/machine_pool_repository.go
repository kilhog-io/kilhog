package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type MachinePoolRepository struct {
	store *db.Store
}

func NewMachinePoolRepository(store *db.Store) *MachinePoolRepository {
	return &MachinePoolRepository{store: store}
}

var _ service.MachinePoolRepository = (*MachinePoolRepository)(nil)

func (r *MachinePoolRepository) Create(ctx context.Context, pool *model.MachinePool) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO machine_pools (
				uuid, name, slug, description, enabled, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 7))

		now := time.Now().UTC()
		pool.CreatedAt = now
		pool.UpdatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, pool.UUID),
			pool.Name,
			pool.Slug,
			nullString(pool.Description),
			boolToStore(r.store.Dialect, pool.Enabled),
			formatTime(r.store.Dialect, now),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert machine pool: %w", err)
		}
		return nil
	})
}

func (r *MachinePoolRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachinePool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, description, enabled, created_at, updated_at
		FROM machine_pools WHERE uuid = %s
	`, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *MachinePoolRepository) GetBySlug(ctx context.Context, slug string) (*model.MachinePool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, description, enabled, created_at, updated_at
		FROM machine_pools WHERE slug = %s
	`, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, slug))
}

func (r *MachinePoolRepository) Update(ctx context.Context, pool *model.MachinePool) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			UPDATE machine_pools
			SET name = %s, slug = %s, description = %s, enabled = %s, updated_at = %s
			WHERE uuid = %s
		`,
			placeholder(r.store.Dialect, 1),
			placeholder(r.store.Dialect, 2),
			placeholder(r.store.Dialect, 3),
			placeholder(r.store.Dialect, 4),
			placeholder(r.store.Dialect, 5),
			placeholder(r.store.Dialect, 6),
		)
		now := time.Now().UTC()
		pool.UpdatedAt = now
		res, err := q.ExecContext(ctx, query,
			pool.Name,
			pool.Slug,
			nullString(pool.Description),
			boolToStore(r.store.Dialect, pool.Enabled),
			formatTime(r.store.Dialect, now),
			uuidString(r.store.Dialect, pool.UUID),
		)
		if err != nil {
			return fmt.Errorf("update machine pool: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update machine pool rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachinePoolNotFound
		}
		return nil
	})
}

func (r *MachinePoolRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM machine_pools WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete machine pool: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete machine pool rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachinePoolNotFound
		}
		return nil
	})
}

func (r *MachinePoolRepository) List(ctx context.Context) ([]*model.MachinePool, error) {
	query := `
		SELECT uuid, name, slug, description, enabled, created_at, updated_at
		FROM machine_pools
		ORDER BY name
	`
	rows, err := r.store.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list machine pools: %w", err)
	}
	defer rows.Close()

	var pools []*model.MachinePool
	for rows.Next() {
		pool, err := scanMachinePool(r.store.Dialect, rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine pools: %w", err)
	}
	return pools, nil
}

func (r *MachinePoolRepository) CountEnabled(ctx context.Context) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM machine_pools WHERE enabled = %s`, placeholder(r.store.Dialect, 1))
	var n int
	if err := r.store.DB.QueryRowContext(ctx, query, boolToStore(r.store.Dialect, true)).Scan(&n); err != nil {
		return 0, fmt.Errorf("count enabled machine pools: %w", err)
	}
	return n, nil
}

func (r *MachinePoolRepository) scanOne(row *sql.Row) (*model.MachinePool, error) {
	pool, err := scanMachinePool(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMachinePoolNotFound
	}
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func scanMachinePool(dialect db.Dialect, s scanner) (*model.MachinePool, error) {
	var (
		rawUUID    any
		name       string
		slug       string
		desc       sql.NullString
		enabledRaw any
		createdRaw any
		updatedRaw any
	)
	if err := s.Scan(&rawUUID, &name, &slug, &desc, &enabledRaw, &createdRaw, &updatedRaw); err != nil {
		return nil, err
	}
	id, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}
	enabled, err := scanBool(enabledRaw)
	if err != nil {
		return nil, err
	}
	createdAt, err := scanTime(createdRaw)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	updatedAt, err := scanTime(updatedRaw)
	if err != nil {
		return nil, fmt.Errorf("scan updated_at: %w", err)
	}
	return &model.MachinePool{
		UUID:        id,
		Name:        name,
		Slug:        slug,
		Description: desc.String,
		Enabled:     enabled,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}
