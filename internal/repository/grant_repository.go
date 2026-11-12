package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type GrantRepository struct {
	store *db.Store
}

func NewGrantRepository(store *db.Store) *GrantRepository {
	return &GrantRepository{store: store}
}

var _ service.GrantRepository = (*GrantRepository)(nil)

const grantSelectColumns = `uuid, principal_kind, local_user_uuid, identity_pool_uuid, oidc_subject, oidc_group,
	machine_uuid, machine_pool_uuid, resource_kind, resource_uuid, network_uuid, capability,
	can_create, can_read, can_update, can_delete, owner, created_at, updated_at`

func (r *GrantRepository) Create(ctx context.Context, grant *model.Grant) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		return insertGrant(ctx, q, r.store.Dialect, grant)
	})
}

func (r *GrantRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Grant, error) {
	query := fmt.Sprintf(`SELECT %s FROM grants WHERE uuid = %s`, grantSelectColumns, placeholder(r.store.Dialect, 1))
	grant, err := scanGrant(r.store.Dialect, r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGrantNotFound
	}
	if err != nil {
		return nil, err
	}
	return grant, nil
}

func (r *GrantRepository) Update(ctx context.Context, grant *model.Grant) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		return updateGrantRow(ctx, q, r.store.Dialect, grant)
	})
}

func (r *GrantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM grants WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete grant: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete grant rows: %w", err)
		}
		if n == 0 {
			return service.ErrGrantNotFound
		}
		return nil
	})
}

func (r *GrantRepository) ListByResource(ctx context.Context, kind model.GrantResourceKind, resourceUUID uuid.UUID) ([]*model.Grant, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM grants WHERE resource_kind = %s AND resource_uuid = %s ORDER BY created_at`,
		grantSelectColumns,
		placeholder(r.store.Dialect, 1),
		placeholder(r.store.Dialect, 2),
	)
	rows, err := r.store.DB.QueryContext(ctx, query, string(kind), uuidString(r.store.Dialect, resourceUUID))
	if err != nil {
		return nil, fmt.Errorf("list grants by resource: %w", err)
	}
	defer rows.Close()
	return collectGrants(r.store.Dialect, rows)
}

func (r *GrantRepository) ListPlatform(ctx context.Context) ([]*model.Grant, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM grants WHERE resource_kind = %s ORDER BY created_at`,
		grantSelectColumns,
		placeholder(r.store.Dialect, 1),
	)
	rows, err := r.store.DB.QueryContext(ctx, query, string(model.GrantResourcePlatform))
	if err != nil {
		return nil, fmt.Errorf("list platform grants: %w", err)
	}
	defer rows.Close()
	return collectGrants(r.store.Dialect, rows)
}

func (r *GrantRepository) ListForSubjects(ctx context.Context, subjects []model.GrantPrincipal) ([]*model.Grant, error) {
	if len(subjects) == 0 {
		return []*model.Grant{}, nil
	}

	var clauses []string
	var args []any
	idx := 1
	for _, subject := range subjects {
		clause, extra, next, err := subjectClause(r.store.Dialect, subject, idx)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, "("+clause+")")
		args = append(args, extra...)
		idx = next
	}

	query := fmt.Sprintf(`SELECT %s FROM grants WHERE %s ORDER BY created_at`, grantSelectColumns, strings.Join(clauses, " OR "))
	rows, err := r.store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list grants for subjects: %w", err)
	}
	defer rows.Close()
	return collectGrants(r.store.Dialect, rows)
}

func (r *GrantRepository) GetByPrincipalAndResource(ctx context.Context, principal model.GrantPrincipal, resource model.GrantResource) (*model.Grant, error) {
	clause, args, next, err := subjectClause(r.store.Dialect, principal, 1)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(
		`SELECT %s FROM grants WHERE (%s) AND resource_kind = %s`,
		grantSelectColumns,
		clause,
		placeholder(r.store.Dialect, next),
	)
	args = append(args, string(resource.Kind))
	next++
	if resource.Kind == model.GrantResourcePlatform {
		query += fmt.Sprintf(` AND capability = %s AND resource_uuid IS NULL`, placeholder(r.store.Dialect, next))
		args = append(args, resource.Capability)
	} else if resource.UUID != nil {
		query += fmt.Sprintf(` AND resource_uuid = %s`, placeholder(r.store.Dialect, next))
		args = append(args, uuidString(r.store.Dialect, *resource.UUID))
	} else {
		return nil, service.ErrInvalidGrant
	}

	grant, err := scanGrant(r.store.Dialect, r.store.DB.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGrantNotFound
	}
	if err != nil {
		return nil, err
	}
	return grant, nil
}

func (r *GrantRepository) CountOwners(ctx context.Context, kind model.GrantResourceKind, resourceUUID uuid.UUID) (int, error) {
	query := fmt.Sprintf(
		`SELECT COUNT(*) FROM grants WHERE resource_kind = %s AND resource_uuid = %s AND owner = %s`,
		placeholder(r.store.Dialect, 1),
		placeholder(r.store.Dialect, 2),
		placeholder(r.store.Dialect, 3),
	)
	var n int
	if err := r.store.DB.QueryRowContext(ctx, query, string(kind), uuidString(r.store.Dialect, resourceUUID), boolToStore(r.store.Dialect, true)).Scan(&n); err != nil {
		return 0, fmt.Errorf("count owners: %w", err)
	}
	return n, nil
}

func (r *GrantRepository) DeleteBySubnet(ctx context.Context, subnetUUID uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		return deleteGrantsForSubnet(ctx, q, r.store.Dialect, subnetUUID)
	})
}

func (r *GrantRepository) ApplyTransfer(ctx context.Context, source *model.Grant, dest *model.Grant) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		existing, err := getGrantByPrincipalAndResourceTx(ctx, q, r.store.Dialect, dest.Principal, dest.Resource)
		if err != nil && !errors.Is(err, service.ErrGrantNotFound) {
			return err
		}
		if existing != nil {
			existing.Owner = true
			existing.Permissions = existing.Permissions.WithOwner(true)
			if err := updateGrantRow(ctx, q, r.store.Dialect, existing); err != nil {
				return err
			}
			dest.UUID = existing.UUID
			dest.Permissions = existing.Permissions
			dest.CreatedAt = existing.CreatedAt
			dest.UpdatedAt = existing.UpdatedAt
		} else {
			if dest.UUID == uuid.Nil {
				dest.UUID = uuid.New()
			}
			dest.Owner = true
			dest.Permissions = dest.Permissions.WithOwner(true)
			if err := insertGrant(ctx, q, r.store.Dialect, dest); err != nil {
				return err
			}
		}

		source.Owner = false
		if !source.Permissions.Any() {
			query := fmt.Sprintf(`DELETE FROM grants WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
			if _, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, source.UUID)); err != nil {
				return fmt.Errorf("delete source grant: %w", err)
			}
			return nil
		}
		return updateGrantRow(ctx, q, r.store.Dialect, source)
	})
}

func deleteGrantsForSubnet(ctx context.Context, q db.Querier, dialect db.Dialect, subnetUUID uuid.UUID) error {
	query := fmt.Sprintf(
		`DELETE FROM grants WHERE resource_kind = %s AND resource_uuid = %s`,
		placeholder(dialect, 1),
		placeholder(dialect, 2),
	)
	if _, err := q.ExecContext(ctx, query, string(model.GrantResourceSubnet), uuidString(dialect, subnetUUID)); err != nil {
		return fmt.Errorf("delete subnet grants: %w", err)
	}
	return nil
}

func insertGrant(ctx context.Context, q db.Querier, dialect db.Dialect, grant *model.Grant) error {
	now := time.Now().UTC()
	grant.CreatedAt = now
	grant.UpdatedAt = now
	if grant.Owner {
		grant.Permissions = grant.Permissions.WithOwner(true)
	}

	query := fmt.Sprintf(`
		INSERT INTO grants (
			uuid, principal_kind, local_user_uuid, identity_pool_uuid, oidc_subject, oidc_group,
			machine_uuid, machine_pool_uuid, resource_kind, resource_uuid, network_uuid, capability,
			can_create, can_read, can_update, can_delete, owner, created_at, updated_at
		) VALUES (%s)
	`, placeholders(dialect, 19))

	if _, err := q.ExecContext(ctx, query, grantArgs(dialect, grant)...); err != nil {
		if isUniqueViolation(err) {
			return service.ErrGrantConflict
		}
		return fmt.Errorf("insert grant: %w", err)
	}
	return nil
}

func updateGrantRow(ctx context.Context, q db.Querier, dialect db.Dialect, grant *model.Grant) error {
	now := time.Now().UTC()
	grant.UpdatedAt = now
	if grant.Owner {
		grant.Permissions = grant.Permissions.WithOwner(true)
	}

	query := fmt.Sprintf(`
		UPDATE grants SET
			can_create = %s,
			can_read = %s,
			can_update = %s,
			can_delete = %s,
			owner = %s,
			updated_at = %s
		WHERE uuid = %s
	`,
		placeholder(dialect, 1),
		placeholder(dialect, 2),
		placeholder(dialect, 3),
		placeholder(dialect, 4),
		placeholder(dialect, 5),
		placeholder(dialect, 6),
		placeholder(dialect, 7),
	)

	res, err := q.ExecContext(ctx, query,
		boolToStore(dialect, grant.Permissions.Create),
		boolToStore(dialect, grant.Permissions.Read),
		boolToStore(dialect, grant.Permissions.Update),
		boolToStore(dialect, grant.Permissions.Delete),
		boolToStore(dialect, grant.Owner),
		formatTime(dialect, now),
		uuidString(dialect, grant.UUID),
	)
	if err != nil {
		return fmt.Errorf("update grant: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update grant rows: %w", err)
	}
	if n == 0 {
		return service.ErrGrantNotFound
	}
	return nil
}

func grantArgs(dialect db.Dialect, grant *model.Grant) []any {
	var resourceUUID any
	if grant.Resource.UUID != nil {
		resourceUUID = uuidString(dialect, *grant.Resource.UUID)
	}
	capability := grant.Resource.Capability
	var cap any
	if capability != "" {
		cap = capability
	}
	return []any{
		uuidString(dialect, grant.UUID),
		string(grant.Principal.Kind),
		nullableUUID(dialect, grant.Principal.LocalUserUUID),
		nullableUUID(dialect, grant.Principal.IdentityPoolUUID),
		nullString(grant.Principal.Subject),
		nullString(grant.Principal.Group),
		nullableUUID(dialect, grant.Principal.MachineUUID),
		nullableUUID(dialect, grant.Principal.MachinePoolUUID),
		string(grant.Resource.Kind),
		resourceUUID,
		nullableUUID(dialect, grant.NetworkUUID),
		cap,
		boolToStore(dialect, grant.Permissions.Create),
		boolToStore(dialect, grant.Permissions.Read),
		boolToStore(dialect, grant.Permissions.Update),
		boolToStore(dialect, grant.Permissions.Delete),
		boolToStore(dialect, grant.Owner),
		formatTime(dialect, grant.CreatedAt),
		formatTime(dialect, grant.UpdatedAt),
	}
}

func subjectClause(dialect db.Dialect, subject model.GrantPrincipal, start int) (string, []any, int, error) {
	switch subject.Kind {
	case model.GrantPrincipalLocalUser:
		if subject.LocalUserUUID == nil {
			return "", nil, start, fmt.Errorf("local user uuid required")
		}
		return fmt.Sprintf(`principal_kind = %s AND local_user_uuid = %s`, placeholder(dialect, start), placeholder(dialect, start+1)),
			[]any{string(subject.Kind), uuidString(dialect, *subject.LocalUserUUID)}, start + 2, nil
	case model.GrantPrincipalOIDC:
		if subject.IdentityPoolUUID == nil {
			return "", nil, start, fmt.Errorf("oidc pool uuid required")
		}
		return fmt.Sprintf(`principal_kind = %s AND identity_pool_uuid = %s AND oidc_subject = %s`,
				placeholder(dialect, start), placeholder(dialect, start+1), placeholder(dialect, start+2)),
			[]any{string(subject.Kind), uuidString(dialect, *subject.IdentityPoolUUID), subject.Subject}, start + 3, nil
	case model.GrantPrincipalOIDCGroup:
		if subject.IdentityPoolUUID == nil {
			return "", nil, start, fmt.Errorf("oidc pool uuid required")
		}
		return fmt.Sprintf(`principal_kind = %s AND identity_pool_uuid = %s AND oidc_group = %s`,
				placeholder(dialect, start), placeholder(dialect, start+1), placeholder(dialect, start+2)),
			[]any{string(subject.Kind), uuidString(dialect, *subject.IdentityPoolUUID), subject.Group}, start + 3, nil
	case model.GrantPrincipalMachine:
		if subject.MachineUUID == nil {
			return "", nil, start, fmt.Errorf("machine uuid required")
		}
		return fmt.Sprintf(`principal_kind = %s AND machine_uuid = %s`, placeholder(dialect, start), placeholder(dialect, start+1)),
			[]any{string(subject.Kind), uuidString(dialect, *subject.MachineUUID)}, start + 2, nil
	case model.GrantPrincipalMachinePool:
		if subject.MachinePoolUUID == nil {
			return "", nil, start, fmt.Errorf("machine pool uuid required")
		}
		return fmt.Sprintf(`principal_kind = %s AND machine_pool_uuid = %s`, placeholder(dialect, start), placeholder(dialect, start+1)),
			[]any{string(subject.Kind), uuidString(dialect, *subject.MachinePoolUUID)}, start + 2, nil
	default:
		return "", nil, start, fmt.Errorf("unknown principal kind %q", subject.Kind)
	}
}

func getGrantByPrincipalAndResourceTx(ctx context.Context, q db.Querier, dialect db.Dialect, principal model.GrantPrincipal, resource model.GrantResource) (*model.Grant, error) {
	clause, args, next, err := subjectClause(dialect, principal, 1)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT %s FROM grants WHERE (%s) AND resource_kind = %s`, grantSelectColumns, clause, placeholder(dialect, next))
	args = append(args, string(resource.Kind))
	next++
	if resource.Kind == model.GrantResourcePlatform {
		query += fmt.Sprintf(` AND capability = %s`, placeholder(dialect, next))
		args = append(args, resource.Capability)
	} else if resource.UUID != nil {
		query += fmt.Sprintf(` AND resource_uuid = %s`, placeholder(dialect, next))
		args = append(args, uuidString(dialect, *resource.UUID))
	}
	grant, err := scanGrant(dialect, q.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrGrantNotFound
	}
	return grant, err
}

func collectGrants(dialect db.Dialect, rows *sql.Rows) ([]*model.Grant, error) {
	var grants []*model.Grant
	for rows.Next() {
		grant, err := scanGrant(dialect, rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate grants: %w", err)
	}
	if grants == nil {
		grants = []*model.Grant{}
	}
	return grants, nil
}

func scanGrant(dialect db.Dialect, s scanner) (*model.Grant, error) {
	var (
		rawUUID      any
		kind         string
		rawLocal     any
		rawPool      any
		oidcSubject  sql.NullString
		oidcGroup    sql.NullString
		rawMachine   any
		rawMachPool  any
		resourceKind string
		rawResource  any
		rawNetwork   any
		capability   sql.NullString
		createRaw    any
		readRaw      any
		updateRaw    any
		deleteRaw    any
		ownerRaw     any
		createdRaw   any
		updatedRaw   any
	)
	if err := s.Scan(
		&rawUUID, &kind, &rawLocal, &rawPool, &oidcSubject, &oidcGroup,
		&rawMachine, &rawMachPool, &resourceKind, &rawResource, &rawNetwork, &capability,
		&createRaw, &readRaw, &updateRaw, &deleteRaw, &ownerRaw, &createdRaw, &updatedRaw,
	); err != nil {
		return nil, err
	}

	id, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}
	localUser, err := scanOptionalUUID(dialect, rawLocal)
	if err != nil {
		return nil, err
	}
	pool, err := scanOptionalUUID(dialect, rawPool)
	if err != nil {
		return nil, err
	}
	machine, err := scanOptionalUUID(dialect, rawMachine)
	if err != nil {
		return nil, err
	}
	machinePool, err := scanOptionalUUID(dialect, rawMachPool)
	if err != nil {
		return nil, err
	}
	resourceUUID, err := scanOptionalUUID(dialect, rawResource)
	if err != nil {
		return nil, err
	}
	networkUUID, err := scanOptionalUUID(dialect, rawNetwork)
	if err != nil {
		return nil, err
	}
	canCreate, err := scanBool(createRaw)
	if err != nil {
		return nil, err
	}
	canRead, err := scanBool(readRaw)
	if err != nil {
		return nil, err
	}
	canUpdate, err := scanBool(updateRaw)
	if err != nil {
		return nil, err
	}
	canDelete, err := scanBool(deleteRaw)
	if err != nil {
		return nil, err
	}
	owner, err := scanBool(ownerRaw)
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

	return &model.Grant{
		UUID: id,
		Principal: model.GrantPrincipal{
			Kind:             model.GrantPrincipalKind(kind),
			LocalUserUUID:    localUser,
			IdentityPoolUUID: pool,
			Subject:          oidcSubject.String,
			Group:            oidcGroup.String,
			MachineUUID:      machine,
			MachinePoolUUID:  machinePool,
		},
		Resource: model.GrantResource{
			Kind:       model.GrantResourceKind(resourceKind),
			UUID:       resourceUUID,
			Capability: capability.String,
		},
		Permissions: model.Permissions{
			Create: canCreate,
			Read:   canRead,
			Update: canUpdate,
			Delete: canDelete,
		}.WithOwner(owner),
		Owner:       owner,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		NetworkUUID: networkUUID,
	}, nil
}
