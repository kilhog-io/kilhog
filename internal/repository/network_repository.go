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

var (
	ErrNetworkNotFound = service.ErrNetworkNotFound
	ErrSubnetNotFound  = service.ErrSubnetNotFound
)

type NetworkRepository struct {
	store *db.Store
}

func NewNetworkRepository(store *db.Store) *NetworkRepository {
	return &NetworkRepository{store: store}
}

var _ service.NetworkRepository = (*NetworkRepository)(nil)

func (r *NetworkRepository) Create(ctx context.Context, network *model.Network) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO networks (uuid, name, description, created_at, updated_at)
			VALUES (%s)
		`, placeholders(r.store.Dialect, 5))

		now := time.Now().UTC()
		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, network.UUID),
			network.Name,
			nullString(network.Description),
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert network: %w", err)
		}

		return replaceTags(ctx, q, r.store.Dialect, model.ParentKindNetwork, network.UUID, network.Tags)
	})
}

func (r *NetworkRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Network, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, description
		FROM networks
		WHERE uuid = %s
	`, placeholder(r.store.Dialect, 1))

	var (
		rawUUID     any
		name        string
		description sql.NullString
	)

	err := r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)).Scan(&rawUUID, &name, &description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetworkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query network by uuid: %w", err)
	}

	parsedUUID, err := scanUUID(r.store.Dialect, rawUUID)
	if err != nil {
		return nil, err
	}

	tags, err := loadTags(ctx, r.store.DB, r.store.Dialect, model.ParentKindNetwork, parsedUUID)
	if err != nil {
		return nil, err
	}

	return &model.Network{
		UUID:        parsedUUID,
		Name:        name,
		Description: description.String,
		Tags:        tags,
	}, nil
}

func (r *NetworkRepository) GetByName(ctx context.Context, name string) (*model.Network, error) {
	query := `
		SELECT uuid, name, description
		FROM networks
		WHERE name = ` + placeholder(r.store.Dialect, 1)

	var (
		rawUUID     any
		rowName     string
		description sql.NullString
	)

	err := r.store.DB.QueryRowContext(ctx, query, name).Scan(&rawUUID, &rowName, &description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNetworkNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query network by name: %w", err)
	}

	parsedUUID, err := scanUUID(r.store.Dialect, rawUUID)
	if err != nil {
		return nil, err
	}

	tags, err := loadTags(ctx, r.store.DB, r.store.Dialect, model.ParentKindNetwork, parsedUUID)
	if err != nil {
		return nil, err
	}

	return &model.Network{
		UUID:        parsedUUID,
		Name:        rowName,
		Description: description.String,
		Tags:        tags,
	}, nil
}

func (r *NetworkRepository) Update(ctx context.Context, network *model.Network) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			UPDATE networks
			SET name = %s, description = %s, updated_at = %s
			WHERE uuid = %s
		`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2), placeholder(r.store.Dialect, 3), placeholder(r.store.Dialect, 4))

		result, err := q.ExecContext(ctx, query,
			network.Name,
			nullString(network.Description),
			time.Now().UTC(),
			uuidString(r.store.Dialect, network.UUID),
		)
		if err != nil {
			return fmt.Errorf("update network: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("network rows affected: %w", err)
		}
		if affected == 0 {
			return ErrNetworkNotFound
		}

		return replaceTags(ctx, q, r.store.Dialect, model.ParentKindNetwork, network.UUID, network.Tags)
	})
}

func (r *NetworkRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		if err := deleteTagsForResource(ctx, q, r.store.Dialect, model.ParentKindNetwork, id); err != nil {
			return err
		}

		query := fmt.Sprintf("DELETE FROM networks WHERE uuid = %s", placeholder(r.store.Dialect, 1))
		result, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete network: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("network rows affected: %w", err)
		}
		if affected == 0 {
			return ErrNetworkNotFound
		}

		return nil
	})
}

func (r *NetworkRepository) List(ctx context.Context) ([]*model.Network, error) {
	rows, err := r.store.DB.QueryContext(ctx, `
		SELECT uuid, name, description
		FROM networks
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	defer rows.Close()

	networks := make([]*model.Network, 0)
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var (
			rawUUID     any
			name        string
			description sql.NullString
		)
		if err := rows.Scan(&rawUUID, &name, &description); err != nil {
			return nil, fmt.Errorf("scan network: %w", err)
		}

		parsedUUID, err := scanUUID(r.store.Dialect, rawUUID)
		if err != nil {
			return nil, err
		}

		networks = append(networks, &model.Network{
			UUID:        parsedUUID,
			Name:        name,
			Description: description.String,
		})
		ids = append(ids, parsedUUID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate networks: %w", err)
	}

	tagMap, err := loadTagsForResources(ctx, r.store.DB, r.store.Dialect, model.ParentKindNetwork, ids)
	if err != nil {
		return nil, err
	}
	for _, network := range networks {
		network.Tags = tagMap[network.UUID]
	}

	return networks, nil
}

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
