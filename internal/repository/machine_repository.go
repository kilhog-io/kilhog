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

type MachineRepository struct {
	store *db.Store
}

func NewMachineRepository(store *db.Store) *MachineRepository {
	return &MachineRepository{store: store}
}

var _ service.MachineRepository = (*MachineRepository)(nil)

const machineColumns = `uuid, machine_pool_uuid, name, description, enabled, provider_uuid, subject, subject_prefix, claims, created_at, updated_at`

func (r *MachineRepository) Create(ctx context.Context, machine *model.Machine) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		claimsJSON, err := json.Marshal(machine.Claims)
		if err != nil {
			return fmt.Errorf("marshal claims: %w", err)
		}
		if machine.Claims == nil {
			claimsJSON = []byte("[]")
		}
		query := fmt.Sprintf(`
			INSERT INTO machines (
				uuid, machine_pool_uuid, name, description, enabled, provider_uuid, subject, subject_prefix, claims, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 11))

		now := time.Now().UTC()
		machine.CreatedAt = now
		machine.UpdatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, machine.UUID),
			uuidString(r.store.Dialect, machine.MachinePoolUUID),
			machine.Name,
			nullString(machine.Description),
			boolToStore(r.store.Dialect, machine.Enabled),
			nullableUUID(r.store.Dialect, machine.ProviderUUID),
			nullString(machine.Subject),
			nullString(machine.SubjectPrefix),
			string(claimsJSON),
			formatTime(r.store.Dialect, now),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert machine: %w", err)
		}
		return nil
	})
}

func (r *MachineRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Machine, error) {
	query := fmt.Sprintf(`SELECT %s FROM machines WHERE uuid = %s`, machineColumns, placeholder(r.store.Dialect, 1))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *MachineRepository) GetByName(ctx context.Context, poolUUID uuid.UUID, name string) (*model.Machine, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machines WHERE machine_pool_uuid = %s AND name = %s
	`, machineColumns, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))
	return r.scanOne(r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, poolUUID), name))
}

func (r *MachineRepository) Update(ctx context.Context, machine *model.Machine) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		claimsJSON, err := json.Marshal(machine.Claims)
		if err != nil {
			return fmt.Errorf("marshal claims: %w", err)
		}
		if machine.Claims == nil {
			claimsJSON = []byte("[]")
		}
		query := fmt.Sprintf(`
			UPDATE machines
			SET name = %s, description = %s, enabled = %s, provider_uuid = %s, subject = %s, subject_prefix = %s, claims = %s, updated_at = %s
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
		machine.UpdatedAt = now
		res, err := q.ExecContext(ctx, query,
			machine.Name,
			nullString(machine.Description),
			boolToStore(r.store.Dialect, machine.Enabled),
			nullableUUID(r.store.Dialect, machine.ProviderUUID),
			nullString(machine.Subject),
			nullString(machine.SubjectPrefix),
			string(claimsJSON),
			formatTime(r.store.Dialect, now),
			uuidString(r.store.Dialect, machine.UUID),
			uuidString(r.store.Dialect, machine.MachinePoolUUID),
		)
		if err != nil {
			return fmt.Errorf("update machine: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update machine rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachineNotFound
		}
		return nil
	})
}

func (r *MachineRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM machines WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete machine: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete machine rows: %w", err)
		}
		if n == 0 {
			return service.ErrMachineNotFound
		}
		return nil
	})
}

func (r *MachineRepository) ListByPool(ctx context.Context, poolUUID uuid.UUID) ([]*model.Machine, error) {
	query := fmt.Sprintf(`
		SELECT %s FROM machines WHERE machine_pool_uuid = %s ORDER BY name
	`, machineColumns, placeholder(r.store.Dialect, 1))
	rows, err := r.store.DB.QueryContext(ctx, query, uuidString(r.store.Dialect, poolUUID))
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	defer rows.Close()
	return collectMachines(r.store.Dialect, rows)
}

func (r *MachineRepository) ListEnabledByProvider(ctx context.Context, providerUUID uuid.UUID) ([]*model.Machine, error) {
	query := fmt.Sprintf(`
		SELECT m.uuid, m.machine_pool_uuid, m.name, m.description, m.enabled, m.provider_uuid, m.subject, m.subject_prefix, m.claims, m.created_at, m.updated_at
		FROM machines m
		INNER JOIN machine_pools pool ON pool.uuid = m.machine_pool_uuid
		WHERE m.provider_uuid = %s AND m.enabled = %s AND pool.enabled = %s
		ORDER BY m.name
	`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2), placeholder(r.store.Dialect, 3))
	rows, err := r.store.DB.QueryContext(ctx, query, uuidString(r.store.Dialect, providerUUID), boolToStore(r.store.Dialect, true), boolToStore(r.store.Dialect, true))
	if err != nil {
		return nil, fmt.Errorf("list machines by provider: %w", err)
	}
	defer rows.Close()
	return collectMachines(r.store.Dialect, rows)
}

func (r *MachineRepository) scanOne(row *sql.Row) (*model.Machine, error) {
	machine, err := scanMachine(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrMachineNotFound
	}
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func collectMachines(dialect db.Dialect, rows *sql.Rows) ([]*model.Machine, error) {
	var list []*model.Machine
	for rows.Next() {
		machine, err := scanMachine(dialect, rows)
		if err != nil {
			return nil, err
		}
		list = append(list, machine)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate machines: %w", err)
	}
	return list, nil
}

func scanMachine(dialect db.Dialect, s scanner) (*model.Machine, error) {
	var (
		rawUUID     any
		rawPool     any
		name        string
		desc        sql.NullString
		enabledRaw  any
		rawProvider any
		subject     sql.NullString
		prefix      sql.NullString
		claimsRaw   string
		createdRaw  any
		updatedRaw  any
	)
	if err := s.Scan(&rawUUID, &rawPool, &name, &desc, &enabledRaw, &rawProvider, &subject, &prefix, &claimsRaw, &createdRaw, &updatedRaw); err != nil {
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
	providerID, err := scanOptionalUUID(dialect, rawProvider)
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
	var claims []model.ClaimCondition
	if claimsRaw != "" {
		if err := json.Unmarshal([]byte(claimsRaw), &claims); err != nil {
			return nil, fmt.Errorf("unmarshal claims: %w", err)
		}
	}
	return &model.Machine{
		UUID:            id,
		MachinePoolUUID: poolID,
		Name:            name,
		Description:     desc.String,
		Enabled:         enabled,
		ProviderUUID:    providerID,
		Subject:         subject.String,
		SubjectPrefix:   prefix.String,
		Claims:          claims,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}
