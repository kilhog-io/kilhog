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

type SubnetRepository struct {
	store *db.Store
}

func NewSubnetRepository(store *db.Store) *SubnetRepository {
	return &SubnetRepository{store: store}
}

var _ service.SubnetRepository = (*SubnetRepository)(nil)

func (r *SubnetRepository) Create(ctx context.Context, subnet *model.Subnet) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		networkUUID, err := r.resolveNetworkUUID(ctx, q, subnet)
		if err != nil {
			return err
		}

		query := fmt.Sprintf(`
			INSERT INTO subnets (
				uuid, network_uuid, name, description, prefix, address,
				address_type, parent_kind, parent_uuid, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 11))

		now := time.Now().UTC()
		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, subnet.UUID),
			uuidString(r.store.Dialect, networkUUID),
			subnet.Name,
			nullString(subnet.Description),
			subnet.Prefix,
			subnet.Address,
			string(subnet.Type),
			string(subnet.Parent.Kind),
			uuidString(r.store.Dialect, subnet.Parent.UUID),
			now,
			now,
		); err != nil {
			return fmt.Errorf("insert subnet: %w", err)
		}

		return replaceTags(ctx, q, r.store.Dialect, model.ParentKindSubnet, subnet.UUID, subnet.Tags)
	})
}

func (r *SubnetRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Subnet, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, description, prefix, address, address_type, parent_kind, parent_uuid
		FROM subnets
		WHERE uuid = %s
	`, placeholder(r.store.Dialect, 1))

	subnet, err := scanSubnetRow(r.store.Dialect, r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSubnetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query subnet by uuid: %w", err)
	}

	tags, err := loadTags(ctx, r.store.DB, r.store.Dialect, model.ParentKindSubnet, subnet.UUID)
	if err != nil {
		return nil, err
	}
	subnet.Tags = tags

	return subnet, nil
}

func (r *SubnetRepository) GetByName(ctx context.Context, networkID uuid.UUID, name string) (*model.Subnet, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, description, prefix, address, address_type, parent_kind, parent_uuid
		FROM subnets
		WHERE network_uuid = %s AND name = %s
	`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))

	subnet, err := scanSubnetRow(
		r.store.Dialect,
		r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, networkID), name),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSubnetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query subnet by name: %w", err)
	}

	tags, err := loadTags(ctx, r.store.DB, r.store.Dialect, model.ParentKindSubnet, subnet.UUID)
	if err != nil {
		return nil, err
	}
	subnet.Tags = tags

	return subnet, nil
}

func (r *SubnetRepository) Update(ctx context.Context, subnet *model.Subnet) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		networkUUID, err := r.resolveNetworkUUID(ctx, q, subnet)
		if err != nil {
			return err
		}

		query := fmt.Sprintf(`
			UPDATE subnets
			SET network_uuid = %s,
				name = %s,
				description = %s,
				prefix = %s,
				address = %s,
				address_type = %s,
				parent_kind = %s,
				parent_uuid = %s,
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
			placeholder(r.store.Dialect, 10),
		)

		result, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, networkUUID),
			subnet.Name,
			nullString(subnet.Description),
			subnet.Prefix,
			subnet.Address,
			string(subnet.Type),
			string(subnet.Parent.Kind),
			uuidString(r.store.Dialect, subnet.Parent.UUID),
			time.Now().UTC(),
			uuidString(r.store.Dialect, subnet.UUID),
		)
		if err != nil {
			return fmt.Errorf("update subnet: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("subnet rows affected: %w", err)
		}
		if affected == 0 {
			return ErrSubnetNotFound
		}

		return replaceTags(ctx, q, r.store.Dialect, model.ParentKindSubnet, subnet.UUID, subnet.Tags)
	})
}

func (r *SubnetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		if err := deleteTagsForResource(ctx, q, r.store.Dialect, model.ParentKindSubnet, id); err != nil {
			return err
		}

		query := fmt.Sprintf("DELETE FROM subnets WHERE uuid = %s", placeholder(r.store.Dialect, 1))
		result, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete subnet: %w", err)
		}

		affected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("subnet rows affected: %w", err)
		}
		if affected == 0 {
			return ErrSubnetNotFound
		}

		return nil
	})
}

func (r *SubnetRepository) ListByNetwork(ctx context.Context, networkID uuid.UUID) ([]*model.Subnet, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, description, prefix, address, address_type, parent_kind, parent_uuid
		FROM subnets
		WHERE network_uuid = %s
		ORDER BY name
	`, placeholder(r.store.Dialect, 1))

	return r.listSubnets(ctx, query, uuidString(r.store.Dialect, networkID))
}

func (r *SubnetRepository) ListByParent(ctx context.Context, parent model.Parent) ([]*model.Subnet, error) {
	query := fmt.Sprintf(`
		SELECT uuid, name, description, prefix, address, address_type, parent_kind, parent_uuid
		FROM subnets
		WHERE parent_kind = %s AND parent_uuid = %s
		ORDER BY name
	`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))

	return r.listSubnets(ctx, query, string(parent.Kind), uuidString(r.store.Dialect, parent.UUID))
}

func (r *SubnetRepository) listSubnets(ctx context.Context, query string, args ...any) ([]*model.Subnet, error) {
	rows, err := r.store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subnets: %w", err)
	}
	defer rows.Close()

	subnets := make([]*model.Subnet, 0)
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		subnet, err := scanSubnetRows(r.store.Dialect, rows)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, subnet)
		ids = append(ids, subnet.UUID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subnets: %w", err)
	}

	tagMap, err := loadTagsForResources(ctx, r.store.DB, r.store.Dialect, model.ParentKindSubnet, ids)
	if err != nil {
		return nil, err
	}
	for _, subnet := range subnets {
		subnet.Tags = tagMap[subnet.UUID]
	}

	return subnets, nil
}

func (r *SubnetRepository) resolveNetworkUUID(ctx context.Context, q db.Querier, subnet *model.Subnet) (uuid.UUID, error) {
	switch subnet.Parent.Kind {
	case model.ParentKindNetwork:
		return subnet.Parent.UUID, nil
	case model.ParentKindSubnet:
		query := fmt.Sprintf(`
			SELECT network_uuid
			FROM subnets
			WHERE uuid = %s
		`, placeholder(r.store.Dialect, 1))

		var rawUUID any
		err := q.QueryRowContext(ctx, query, uuidString(r.store.Dialect, subnet.Parent.UUID)).Scan(&rawUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("parent subnet not found")
		}
		if err != nil {
			return uuid.Nil, fmt.Errorf("resolve network from parent subnet: %w", err)
		}

		return scanUUID(r.store.Dialect, rawUUID)
	default:
		return uuid.Nil, fmt.Errorf("unsupported parent kind %q", subnet.Parent.Kind)
	}
}

func scanSubnetRow(dialect db.Dialect, row *sql.Row) (*model.Subnet, error) {
	var (
		rawUUID     any
		name        string
		description sql.NullString
		prefix      int
		address     string
		addressType string
		parentKind  string
		rawParent   any
	)

	if err := row.Scan(&rawUUID, &name, &description, &prefix, &address, &addressType, &parentKind, &rawParent); err != nil {
		return nil, err
	}

	return buildSubnet(dialect, rawUUID, name, description, prefix, address, addressType, parentKind, rawParent)
}

func scanSubnetRows(dialect db.Dialect, rows *sql.Rows) (*model.Subnet, error) {
	var (
		rawUUID     any
		name        string
		description sql.NullString
		prefix      int
		address     string
		addressType string
		parentKind  string
		rawParent   any
	)

	if err := rows.Scan(&rawUUID, &name, &description, &prefix, &address, &addressType, &parentKind, &rawParent); err != nil {
		return nil, fmt.Errorf("scan subnet: %w", err)
	}

	return buildSubnet(dialect, rawUUID, name, description, prefix, address, addressType, parentKind, rawParent)
}

func buildSubnet(
	dialect db.Dialect,
	rawUUID any,
	name string,
	description sql.NullString,
	prefix int,
	address string,
	addressType string,
	parentKind string,
	rawParent any,
) (*model.Subnet, error) {
	parsedUUID, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}

	parentUUID, err := scanUUID(dialect, rawParent)
	if err != nil {
		return nil, err
	}

	return &model.Subnet{
		UUID:        parsedUUID,
		Name:        name,
		Description: description.String,
		Prefix:      prefix,
		Address:     address,
		Type:        model.AddressType(addressType),
		Parent: model.Parent{
			Kind: model.ParentKind(parentKind),
			UUID: parentUUID,
		},
	}, nil
}
