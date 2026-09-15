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

type MachineProviderRepository struct {
	store *db.Store
}

func NewMachineProviderRepository(store *db.Store) *MachineProviderRepository {
	return &MachineProviderRepository{store: store}
}

var _ service.MachineProviderRepository = (*MachineProviderRepository)(nil)

const machineProviderColumns = `uuid, machine_pool_uuid, name, issuer, audiences, jwks_mode, jwks_uri, jwks, enabled, created_at, updated_at`

func (r *MachineProviderRepository) Create(ctx context.Context, provider *model.MachineIdentityProvider) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		audiencesJSON, err := json.Marshal(provider.Audiences)
		if err != nil {
			return fmt.Errorf("marshal audiences: %w", err)
		}
		query := fmt.Sprintf(`
			INSERT INTO machine_identity_providers (
				uuid, machine_pool_uuid, name, issuer, audiences, jwks_mode, jwks_uri, jwks, enabled, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 11))

		now := time.Now().UTC()
		provider.CreatedAt = now
		provider.UpdatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, provider.UUID),
			uuidString(r.store.Dialect, provider.MachinePoolUUID),
			provider.Name,
			provider.Issuer,
			string(audiencesJSON),
			string(provider.JWKSMode),
			nullString(provider.JWKSURI),
			nullString(string(provider.JWKS)),
			boolToStore(r.store.Dialect, provider.Enabled),
			formatTime(r.store.Dialect, now),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert machine identity provider: %w", err)
		}
		return nil
	})
}

func (r *MachineProviderRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachineIdentityProvider, error) {
	query := fmt.Sprintf(`SELECT %s FROM machine_identity_providers WHERE uuid = %s`, machineProviderColumns, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *MachineProviderRepository) GetByName(ctx context.Context, poolUUID uuid.UUID, name string) (*model.MachineIdentityProvider, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machine_identity_providers
		WHERE machine_pool_uuid = %s AND name = %s
	`, machineProviderColumns, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, poolUUID), name))
}

func (r *MachineProviderRepository) GetByIssuer(ctx context.Context, poolUUID uuid.UUID, issuer string) (*model.MachineIdentityProvider, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machine_identity_providers
		WHERE machine_pool_uuid = %s AND issuer = %s
	`, machineProviderColumns, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, poolUUID), issuer))
}

func (r *MachineProviderRepository) Update(ctx context.Context, provider *model.MachineIdentityProvider) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		audiencesJSON, err := json.Marshal(provider.Audiences)
		if err != nil {
			return fmt.Errorf("marshal audiences: %w", err)
		}
		query := fmt.Sprintf(`
			UPDATE machine_identity_providers
			SET name = %s, issuer = %s, audiences = %s, jwks_mode = %s, jwks_uri = %s, jwks = %s, enabled = %s, updated_at = %s
			WHERE uuid = %s AND machine_pool_uuid = %s
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
			placeholder(r.store.Dialect, 10),
		)
		now := time.Now().UTC()
		provider.UpdatedAt = now
		res, err := q.ExecContext(ctx, query,
			provider.Name,
			provider.Issuer,
			string(audiencesJSON),
			string(provider.JWKSMode),
			nullString(provider.JWKSURI),
			nullString(string(provider.JWKS)),
			boolToStore(r.store.Dialect, provider.Enabled),
			formatTime(r.store.Dialect, now),
			uuidString(r.store.Dialect, provider.UUID),
			uuidString(r.store.Dialect, provider.MachinePoolUUID),
		)
		if err != nil {
			return fmt.Errorf("update machine identity provider: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update machine identity provider rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachineProviderNotFound
		}
		return nil
	})
}

func (r *MachineProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM machine_identity_providers WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete machine identity provider: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete machine identity provider rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachineProviderNotFound
		}
		return nil
	})
}

func (r *MachineProviderRepository) ListByPool(ctx context.Context, poolUUID uuid.UUID) ([]*model.MachineIdentityProvider, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machine_identity_providers
		WHERE machine_pool_uuid = %s
		ORDER BY name
	`, machineProviderColumns, placeholder(r.store.Dialect, 1))
	rows, err := r.store.DB.QueryContext(ctx, query, uuidString(r.store.Dialect, poolUUID))
	if err != nil {
		return nil, fmt.Errorf("list machine identity providers: %w", err)
	}
	defer rows.Close()
	return collectMachineProviders(r.store.Dialect, rows)
}

func (r *MachineProviderRepository) ListEnabledByIssuer(ctx context.Context, issuer string) ([]*model.MachineIdentityProvider, error) {
	query := fmt.Sprintf(`
		SELECT p.uuid, p.machine_pool_uuid, p.name, p.issuer, p.audiences, p.jwks_mode, p.jwks_uri, p.jwks, p.enabled, p.created_at, p.updated_at
		FROM machine_identity_providers p
		INNER JOIN machine_pools pool ON pool.uuid = p.machine_pool_uuid
		WHERE p.issuer = %s AND p.enabled = %s AND pool.enabled = %s
		ORDER BY p.name
	`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2), placeholder(r.store.Dialect, 3))
	rows, err := r.store.DB.QueryContext(ctx, query, issuer, boolToStore(r.store.Dialect, true), boolToStore(r.store.Dialect, true))
	if err != nil {
		return nil, fmt.Errorf("list machine identity providers by issuer: %w", err)
	}
	defer rows.Close()
	return collectMachineProviders(r.store.Dialect, rows)
}

func (r *MachineProviderRepository) scanOne(row *sql.Row) (*model.MachineIdentityProvider, error) {
	provider, err := scanMachineProvider(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMachineProviderNotFound
	}
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func collectMachineProviders(dialect db.Dialect, rows *sql.Rows) ([]*model.MachineIdentityProvider, error) {
	var list []*model.MachineIdentityProvider
	for rows.Next() {
		provider, err := scanMachineProvider(dialect, rows)
		if err != nil {
			return nil, err
		}
		list = append(list, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine identity providers: %w", err)
	}
	return list, nil
}

func scanMachineProvider(dialect db.Dialect, s scanner) (*model.MachineIdentityProvider, error) {
	var (
		rawUUID      any
		rawPool      any
		name         string
		issuer       string
		audiencesRaw string
		mode         string
		jwksURI      sql.NullString
		jwks         sql.NullString
		enabledRaw   any
		createdRaw   any
		updatedRaw   any
	)
	if err := s.Scan(&rawUUID, &rawPool, &name, &issuer, &audiencesRaw, &mode, &jwksURI, &jwks, &enabledRaw, &createdRaw, &updatedRaw); err != nil {
		return nil, err
	}
	id, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}
	poolID, err := scanUUID(dialect, rawPool)
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
	var audiences []string
	if err := json.Unmarshal([]byte(audiencesRaw), &audiences); err != nil {
		return nil, fmt.Errorf("unmarshal audiences: %w", err)
	}
	var jwksRaw json.RawMessage
	if jwks.Valid && jwks.String != "" {
		jwksRaw = json.RawMessage(jwks.String)
	}
	return &model.MachineIdentityProvider{
		UUID:            id,
		MachinePoolUUID: poolID,
		Name:            name,
		Issuer:          issuer,
		Audiences:       audiences,
		JWKSMode:        model.JWKSMode(mode),
		JWKSURI:         jwksURI.String,
		JWKS:            jwksRaw,
		Enabled:         enabled,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}
