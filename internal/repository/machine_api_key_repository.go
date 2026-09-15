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

type MachineAPIKeyRepository struct {
	store *db.Store
}

func NewMachineAPIKeyRepository(store *db.Store) *MachineAPIKeyRepository {
	return &MachineAPIKeyRepository{store: store}
}

var _ service.MachineAPIKeyRepository = (*MachineAPIKeyRepository)(nil)

const machineAPIKeyColumns = `uuid, machine_uuid, name, prefix, token_hash, expires_at, revoked_at, last_used_at, created_at`

func (r *MachineAPIKeyRepository) Create(ctx context.Context, key *model.MachineAPIKey) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO machine_api_keys (
				uuid, machine_uuid, name, prefix, token_hash, expires_at, revoked_at, last_used_at, created_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 9))

		now := time.Now().UTC()
		key.CreatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, key.UUID),
			uuidString(r.store.Dialect, key.MachineUUID),
			nullString(key.Name),
			key.Prefix,
			key.TokenHash,
			nullableTime(r.store.Dialect, key.ExpiresAt),
			nullableTime(r.store.Dialect, key.RevokedAt),
			nullableTime(r.store.Dialect, key.LastUsedAt),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert machine api key: %w", err)
		}
		return nil
	})
}

func (r *MachineAPIKeyRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachineAPIKey, error) {
	query := fmt.Sprintf(`SELECT %s FROM machine_api_keys WHERE uuid = %s`, machineAPIKeyColumns, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *MachineAPIKeyRepository) GetByPrefix(ctx context.Context, prefix string) (*model.MachineAPIKey, error) {
	query := fmt.Sprintf(`SELECT %s FROM machine_api_keys WHERE prefix = %s`, machineAPIKeyColumns, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, prefix))
}

func (r *MachineAPIKeyRepository) ListByMachine(ctx context.Context, machineUUID uuid.UUID) ([]*model.MachineAPIKey, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machine_api_keys WHERE machine_uuid = %s ORDER BY created_at
	`, machineAPIKeyColumns, placeholder(r.store.Dialect, 1))
	rows, err := r.store.DB.QueryContext(ctx, query, uuidString(r.store.Dialect, machineUUID))
	if err != nil {
		return nil, fmt.Errorf("list machine api keys: %w", err)
	}
	defer rows.Close()

	var keys []*model.MachineAPIKey
	for rows.Next() {
		key, err := scanMachineAPIKey(r.store.Dialect, rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machine api keys: %w", err)
	}
	return keys, nil
}

func (r *MachineAPIKeyRepository) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			UPDATE machine_api_keys SET revoked_at = %s WHERE uuid = %s
		`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))
		res, err := q.ExecContext(ctx, query, formatTime(r.store.Dialect, at.UTC()), uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("revoke machine api key: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("revoke machine api key rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachineAPIKeyNotFound
		}
		return nil
	})
}

func (r *MachineAPIKeyRepository) TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			UPDATE machine_api_keys SET last_used_at = %s WHERE uuid = %s
		`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))
		if _, err := q.ExecContext(ctx, query, formatTime(r.store.Dialect, at.UTC()), uuidString(r.store.Dialect, id)); err != nil {
			return fmt.Errorf("touch machine api key: %w", err)
		}
		return nil
	})
}

func (r *MachineAPIKeyRepository) scanOne(row *sql.Row) (*model.MachineAPIKey, error) {
	key, err := scanMachineAPIKey(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMachineAPIKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return key, nil
}

func scanMachineAPIKey(dialect db.Dialect, s scanner) (*model.MachineAPIKey, error) {
	var (
		rawUUID     any
		rawMachine  any
		name        sql.NullString
		prefix      string
		tokenHash   string
		expiresRaw  any
		revokedRaw  any
		lastUsedRaw any
		createdRaw  any
	)
	if err := s.Scan(&rawUUID, &rawMachine, &name, &prefix, &tokenHash, &expiresRaw, &revokedRaw, &lastUsedRaw, &createdRaw); err != nil {
		return nil, err
	}
	id, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}
	machineID, err := scanUUID(dialect, rawMachine)
	if err != nil {
		return nil, err
	}
	expiresAt, err := scanOptionalTime(expiresRaw)
	if err != nil {
		return nil, fmt.Errorf("scan expires_at: %w", err)
	}
	revokedAt, err := scanOptionalTime(revokedRaw)
	if err != nil {
		return nil, fmt.Errorf("scan revoked_at: %w", err)
	}
	lastUsedAt, err := scanOptionalTime(lastUsedRaw)
	if err != nil {
		return nil, fmt.Errorf("scan last_used_at: %w", err)
	}
	createdAt, err := scanTime(createdRaw)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	return &model.MachineAPIKey{
		UUID:        id,
		MachineUUID: machineID,
		Name:        name.String,
		Prefix:      prefix,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt,
		RevokedAt:   revokedAt,
		LastUsedAt:  lastUsedAt,
		CreatedAt:   createdAt,
	}, nil
}

func nullableTime(dialect db.Dialect, t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(dialect, *t)
}

func scanOptionalTime(raw any) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []byte:
		if len(v) == 0 {
			return nil, nil
		}
	case string:
		if v == "" {
			return nil, nil
		}
	}
	t, err := scanTime(raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
