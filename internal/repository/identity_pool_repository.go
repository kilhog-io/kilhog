package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type IdentityPoolRepository struct {
	store *db.Store
}

func NewIdentityPoolRepository(store *db.Store) *IdentityPoolRepository {
	return &IdentityPoolRepository{store: store}
}

var _ service.IdentityPoolRepository = (*IdentityPoolRepository)(nil)

func (r *IdentityPoolRepository) Create(ctx context.Context, pool *model.IdentityPool) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		scopesJSON, err := json.Marshal(pool.Scopes)
		if err != nil {
			return fmt.Errorf("marshal scopes: %w", err)
		}

		query := fmt.Sprintf(`
			INSERT INTO oidc_identity_pools (
				uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 10))

		now := time.Now().UTC()
		pool.CreatedAt = now
		pool.UpdatedAt = now
		pool.HasClientSecret = pool.ClientSecret != ""

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, pool.UUID),
			pool.Name,
			pool.Slug,
			pool.Issuer,
			pool.ClientID,
			nullString(pool.ClientSecret),
			string(scopesJSON),
			boolToStore(r.store.Dialect, pool.Enabled),
			formatTime(r.store.Dialect, now),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert identity pool: %w", err)
		}
		return nil
	})
}

func (r *IdentityPoolRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.IdentityPool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
		FROM oidc_identity_pools WHERE uuid = %s
	`, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *IdentityPoolRepository) GetBySlug(ctx context.Context, slug string) (*model.IdentityPool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
		FROM oidc_identity_pools WHERE slug = %s
	`, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, slug))
}

func (r *IdentityPoolRepository) GetByIssuer(ctx context.Context, issuer string) (*model.IdentityPool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
		FROM oidc_identity_pools WHERE issuer = %s
	`, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, issuer))
}

func (r *IdentityPoolRepository) Update(ctx context.Context, pool *model.IdentityPool) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		scopesJSON, err := json.Marshal(pool.Scopes)
		if err != nil {
			return fmt.Errorf("marshal scopes: %w", err)
		}

		query := fmt.Sprintf(`
			UPDATE oidc_identity_pools
			SET name = %s,
			    slug = %s,
			    issuer = %s,
			    client_id = %s,
			    client_secret = %s,
			    scopes = %s,
			    enabled = %s,
			    updated_at = %s
			WHERE uuid = %s
		`,
			placeholder(r.store.Dialect, 1),
			placeholder(r.store.Dialect, 2),
			placeholder(r.store.Dialect, 3),
			placeholder(r.store.Dialect, 4),
			placeholder(r.store.Dialect, 5),
			placeholder(r.store.Dialect, 6),
			placeholder(r.store.Dialect, 7),
			placeholder(r.store.Dialect, 8),
			placeholder(r.store.Dialect, 9),
		)

		now := time.Now().UTC()
		pool.UpdatedAt = now
		pool.HasClientSecret = pool.ClientSecret != ""

		res, err := q.ExecContext(ctx, query,
			pool.Name,
			pool.Slug,
			pool.Issuer,
			pool.ClientID,
			nullString(pool.ClientSecret),
			string(scopesJSON),
			boolToStore(r.store.Dialect, pool.Enabled),
			formatTime(r.store.Dialect, now),
			uuidString(r.store.Dialect, pool.UUID),
		)
		if err != nil {
			return fmt.Errorf("update identity pool: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update identity pool rows: %w", err)
		}
		if n == 0 {
			return service.ErrIdentityPoolNotFound
		}
		return nil
	})
}

func (r *IdentityPoolRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM oidc_identity_pools WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete identity pool: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete identity pool rows: %w", err)
		}
		if n == 0 {
			return service.ErrIdentityPoolNotFound
		}
		return nil
	})
}

func (r *IdentityPoolRepository) List(ctx context.Context) ([]*model.IdentityPool, error) {
	query := `
		SELECT uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
		FROM oidc_identity_pools
		ORDER BY name
	`
	return r.scanMany(ctx, query)
}

func (r *IdentityPoolRepository) ListEnabled(ctx context.Context) ([]*model.IdentityPool, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, slug, issuer, client_id, client_secret, scopes, enabled, created_at, updated_at
		FROM oidc_identity_pools
		WHERE enabled = %s
		ORDER BY name
	`, placeholder(r.store.Dialect, 1))
	rows, err := r.store.DB.QueryContext(ctx, query, boolToStore(r.store.Dialect, true))
	if err != nil {
		return nil, fmt.Errorf("list enabled identity pools: %w", err)
	}
	defer rows.Close()
	return collectIdentityPools(r.store.Dialect, rows)
}

func (r *IdentityPoolRepository) CountEnabled(ctx context.Context) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM oidc_identity_pools WHERE enabled = %s`, placeholder(r.store.Dialect, 1))
	var n int
	if err := r.store.DB.QueryRowContext(ctx, query, boolToStore(r.store.Dialect, true)).Scan(&n); err != nil {
		return 0, fmt.Errorf("count enabled identity pools: %w", err)
	}
	return n, nil
}

func (r *IdentityPoolRepository) scanMany(ctx context.Context, query string) ([]*model.IdentityPool, error) {
	rows, err := r.store.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list identity pools: %w", err)
	}
	defer rows.Close()
	return collectIdentityPools(r.store.Dialect, rows)
}

func (r *IdentityPoolRepository) scanOne(row *sql.Row) (*model.IdentityPool, error) {
	pool, err := scanIdentityPool(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrIdentityPoolNotFound
	}
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func collectIdentityPools(dialect db.Dialect, rows *sql.Rows) ([]*model.IdentityPool, error) {
	var pools []*model.IdentityPool
	for rows.Next() {
		pool, err := scanIdentityPool(dialect, rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity pools: %w", err)
	}
	return pools, nil
}

func scanIdentityPool(dialect db.Dialect, s scanner) (*model.IdentityPool, error) {
	var (
		rawUUID      any
		name         string
		slug         string
		issuer       string
		clientID     string
		clientSecret sql.NullString
		scopesRaw    string
		enabledRaw   any
		createdRaw   any
		updatedRaw   any
	)
	if err := s.Scan(&rawUUID, &name, &slug, &issuer, &clientID, &clientSecret, &scopesRaw, &enabledRaw, &createdRaw, &updatedRaw); err != nil {
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

	var scopes []string
	if err := json.Unmarshal([]byte(scopesRaw), &scopes); err != nil {
		return nil, fmt.Errorf("unmarshal scopes: %w", err)
	}

	return &model.IdentityPool{
		UUID:            id,
		Name:            name,
		Slug:            slug,
		Issuer:          issuer,
		ClientID:        clientID,
		ClientSecret:    clientSecret.String,
		Scopes:          scopes,
		Enabled:         enabled,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
		HasClientSecret: clientSecret.Valid && clientSecret.String != "",
	}, nil
}
