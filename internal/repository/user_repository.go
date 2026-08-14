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

type UserRepository struct {
	store *db.Store
}

func NewUserRepository(store *db.Store) *UserRepository {
	return &UserRepository{store: store}
}

var _ service.UserRepository = (*UserRepository)(nil)

func (r *UserRepository) Create(ctx context.Context, user *model.LocalUser) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO local_users (
				uuid, username, password_hash, display_name, email, role, enabled, created_at, updated_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 9))

		now := time.Now().UTC()
		user.CreatedAt = now
		user.UpdatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, user.UUID),
			user.Username,
			user.PasswordHash,
			nullString(user.DisplayName),
			nullString(user.Email),
			string(user.Role),
			boolToStore(r.store.Dialect, user.Enabled),
			formatTime(r.store.Dialect, now),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert local user: %w", err)
		}
		return nil
	})
}

func (r *UserRepository) GetByUUID(ctx context.Context, id uuid.UUID) (*model.LocalUser, error) {
	query := fmt.Sprintf(`
		SELECT uuid, username, password_hash, display_name, email, role, enabled, created_at, updated_at
		FROM local_users
		WHERE uuid = %s
	`, placeholder(r.store.Dialect, 1))

	return r.scanOne(ctx, r.store.DB.QueryRowContext(ctx, query, uuidString(r.store.Dialect, id)))
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.LocalUser, error) {
	query := fmt.Sprintf(`
		SELECT uuid, username, password_hash, display_name, email, role, enabled, created_at, updated_at
		FROM local_users
		WHERE lower(username) = lower(%s)
	`, placeholder(r.store.Dialect, 1))

	return r.scanOne(ctx, r.store.DB.QueryRowContext(ctx, query, username))
}

func (r *UserRepository) Update(ctx context.Context, user *model.LocalUser) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			UPDATE local_users
			SET username = %s,
			    password_hash = %s,
			    display_name = %s,
			    email = %s,
			    role = %s,
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
		)

		now := time.Now().UTC()
		user.UpdatedAt = now

		res, err := q.ExecContext(ctx, query,
			user.Username,
			user.PasswordHash,
			nullString(user.DisplayName),
			nullString(user.Email),
			string(user.Role),
			boolToStore(r.store.Dialect, user.Enabled),
			formatTime(r.store.Dialect, now),
			uuidString(r.store.Dialect, user.UUID),
		)
		if err != nil {
			return fmt.Errorf("update local user: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("update local user rows: %w", err)
		}
		if n == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM local_users WHERE uuid = %s`, placeholder(r.store.Dialect, 1))
		res, err := q.ExecContext(ctx, query, uuidString(r.store.Dialect, id))
		if err != nil {
			return fmt.Errorf("delete local user: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete local user rows: %w", err)
		}
		if n == 0 {
			return service.ErrUserNotFound
		}
		return nil
	})
}

func (r *UserRepository) List(ctx context.Context) ([]*model.LocalUser, error) {
	query := `
		SELECT uuid, username, password_hash, display_name, email, role, enabled, created_at, updated_at
		FROM local_users
		ORDER BY username
	`
	rows, err := r.store.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list local users: %w", err)
	}
	defer rows.Close()

	var users []*model.LocalUser
	for rows.Next() {
		user, err := scanLocalUser(r.store.Dialect, rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate local users: %w", err)
	}
	return users, nil
}

func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count local users: %w", err)
	}
	return n, nil
}

func (r *UserRepository) CountEnabledAdmins(ctx context.Context) (int, error) {
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM local_users
		WHERE role = %s AND enabled = %s
	`, placeholder(r.store.Dialect, 1), placeholder(r.store.Dialect, 2))

	var n int
	if err := r.store.DB.QueryRowContext(ctx, query, string(model.UserRoleAdmin), boolToStore(r.store.Dialect, true)).Scan(&n); err != nil {
		return 0, fmt.Errorf("count enabled admins: %w", err)
	}
	return n, nil
}

func (r *UserRepository) scanOne(ctx context.Context, row *sql.Row) (*model.LocalUser, error) {
	_ = ctx
	user, err := scanLocalUser(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLocalUser(dialect db.Dialect, s scanner) (*model.LocalUser, error) {
	var (
		rawUUID      any
		username     string
		passwordHash string
		displayName  sql.NullString
		email        sql.NullString
		role         string
		enabledRaw   any
		createdRaw   any
		updatedRaw   any
	)
	if err := s.Scan(&rawUUID, &username, &passwordHash, &displayName, &email, &role, &enabledRaw, &createdRaw, &updatedRaw); err != nil {
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

	return &model.LocalUser{
		UUID:         id,
		Username:     username,
		PasswordHash: passwordHash,
		DisplayName:  displayName.String,
		Email:        email.String,
		Role:         model.UserRole(role),
		Enabled:      enabled,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func boolToStore(dialect db.Dialect, value bool) any {
	if dialect == db.DialectPostgres {
		return value
	}
	if value {
		return 1
	}
	return 0
}

func scanBool(raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	case int32:
		return v != 0, nil
	case int:
		return v != 0, nil
	case []byte:
		s := strings.TrimSpace(string(v))
		return s == "1" || strings.EqualFold(s, "true") || s == "t", nil
	case string:
		s := strings.TrimSpace(v)
		return s == "1" || strings.EqualFold(s, "true") || s == "t", nil
	default:
		return false, fmt.Errorf("unsupported bool type %T", raw)
	}
}

func formatTime(dialect db.Dialect, t time.Time) any {
	t = t.UTC()
	if dialect == db.DialectPostgres {
		return t
	}
	return t.Format(time.RFC3339Nano)
}

func scanTime(raw any) (time.Time, error) {
	switch v := raw.(type) {
	case time.Time:
		return v.UTC(), nil
	case string:
		return parseTimeString(v)
	case []byte:
		return parseTimeString(string(v))
	default:
		return time.Time{}, fmt.Errorf("unsupported time type %T", raw)
	}
}

func parseTimeString(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q", raw)
}
