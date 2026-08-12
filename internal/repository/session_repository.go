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

type SessionRepository struct {
	store *db.Store
}

func NewSessionRepository(store *db.Store) *SessionRepository {
	return &SessionRepository{store: store}
}

var _ service.SessionRepository = (*SessionRepository)(nil)

func (r *SessionRepository) Create(ctx context.Context, session *model.Session) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO sessions (
				uuid, token_hash, principal_kind, local_user_uuid, identity_pool_uuid,
				oidc_subject, oidc_email, oidc_name, expires_at, created_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 10))

		now := time.Now().UTC()
		session.CreatedAt = now

		if _, err := q.ExecContext(ctx, query,
			uuidString(r.store.Dialect, session.UUID),
			session.TokenHash,
			string(session.PrincipalKind),
			nullableUUID(r.store.Dialect, session.LocalUserUUID),
			nullableUUID(r.store.Dialect, session.IdentityPoolUUID),
			nullString(session.OIDCSubject),
			nullString(session.OIDCEmail),
			nullString(session.OIDCName),
			formatTime(r.store.Dialect, session.ExpiresAt),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert session: %w", err)
		}
		return nil
	})
}

func (r *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error) {
	query := fmt.Sprintf(`
		SELECT uuid, token_hash, principal_kind, local_user_uuid, identity_pool_uuid,
		       oidc_subject, oidc_email, oidc_name, expires_at, created_at
		FROM sessions
		WHERE token_hash = %s
	`, placeholder(r.store.Dialect, 1))

	row := r.store.DB.QueryRowContext(ctx, query, tokenHash)
	session, err := scanSession(r.store.Dialect, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (r *SessionRepository) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM sessions WHERE token_hash = %s`, placeholder(r.store.Dialect, 1))
		if _, err := q.ExecContext(ctx, query, tokenHash); err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
		return nil
	})
}

func (r *SessionRepository) DeleteExpired(ctx context.Context, now time.Time) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM sessions WHERE expires_at <= %s`, placeholder(r.store.Dialect, 1))
		if _, err := q.ExecContext(ctx, query, formatTime(r.store.Dialect, now)); err != nil {
			return fmt.Errorf("delete expired sessions: %w", err)
		}
		return nil
	})
}

type OIDCLoginStateRepository struct {
	store *db.Store
}

func NewOIDCLoginStateRepository(store *db.Store) *OIDCLoginStateRepository {
	return &OIDCLoginStateRepository{store: store}
}

var _ service.OIDCLoginStateRepository = (*OIDCLoginStateRepository)(nil)

func (r *OIDCLoginStateRepository) Create(ctx context.Context, state *model.OIDCLoginState) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			INSERT INTO oidc_login_states (
				state, pool_uuid, code_verifier, nonce, redirect_uri, expires_at, created_at
			) VALUES (%s)
		`, placeholders(r.store.Dialect, 7))

		now := time.Now().UTC()
		state.CreatedAt = now

		if _, err := q.ExecContext(ctx, query,
			state.State,
			uuidString(r.store.Dialect, state.PoolUUID),
			state.CodeVerifier,
			state.Nonce,
			state.RedirectURI,
			formatTime(r.store.Dialect, state.ExpiresAt),
			formatTime(r.store.Dialect, now),
		); err != nil {
			return fmt.Errorf("insert oidc login state: %w", err)
		}
		return nil
	})
}

func (r *OIDCLoginStateRepository) Take(ctx context.Context, state string) (*model.OIDCLoginState, error) {
	var result *model.OIDCLoginState
	err := r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`
			SELECT state, pool_uuid, code_verifier, nonce, redirect_uri, expires_at, created_at
			FROM oidc_login_states
			WHERE state = %s
		`, placeholder(r.store.Dialect, 1))

		row := q.QueryRowContext(ctx, query, state)
		item, err := scanOIDCLoginState(r.store.Dialect, row)
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrOIDCLoginStateNotFound
		}
		if err != nil {
			return err
		}

		del := fmt.Sprintf(`DELETE FROM oidc_login_states WHERE state = %s`, placeholder(r.store.Dialect, 1))
		if _, err := q.ExecContext(ctx, del, state); err != nil {
			return fmt.Errorf("delete oidc login state: %w", err)
		}
		result = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *OIDCLoginStateRepository) DeleteExpired(ctx context.Context, now time.Time) error {
	return r.store.WithWriteTx(ctx, func(q db.Querier) error {
		query := fmt.Sprintf(`DELETE FROM oidc_login_states WHERE expires_at <= %s`, placeholder(r.store.Dialect, 1))
		if _, err := q.ExecContext(ctx, query, formatTime(r.store.Dialect, now)); err != nil {
			return fmt.Errorf("delete expired oidc login states: %w", err)
		}
		return nil
	})
}

func nullableUUID(dialect db.Dialect, id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return uuidString(dialect, *id)
}

func scanSession(dialect db.Dialect, s scanner) (*model.Session, error) {
	var (
		rawUUID      any
		tokenHash    string
		kind         string
		rawLocalUser any
		rawPool      any
		oidcSubject  sql.NullString
		oidcEmail    sql.NullString
		oidcName     sql.NullString
		expiresRaw   any
		createdRaw   any
	)
	if err := s.Scan(&rawUUID, &tokenHash, &kind, &rawLocalUser, &rawPool, &oidcSubject, &oidcEmail, &oidcName, &expiresRaw, &createdRaw); err != nil {
		return nil, err
	}

	id, err := scanUUID(dialect, rawUUID)
	if err != nil {
		return nil, err
	}
	localUser, err := scanOptionalUUID(dialect, rawLocalUser)
	if err != nil {
		return nil, err
	}
	pool, err := scanOptionalUUID(dialect, rawPool)
	if err != nil {
		return nil, err
	}
	expiresAt, err := scanTime(expiresRaw)
	if err != nil {
		return nil, fmt.Errorf("scan expires_at: %w", err)
	}
	createdAt, err := scanTime(createdRaw)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}

	return &model.Session{
		UUID:             id,
		TokenHash:        tokenHash,
		PrincipalKind:    model.PrincipalKind(kind),
		LocalUserUUID:    localUser,
		IdentityPoolUUID: pool,
		OIDCSubject:      oidcSubject.String,
		OIDCEmail:        oidcEmail.String,
		OIDCName:         oidcName.String,
		ExpiresAt:        expiresAt,
		CreatedAt:        createdAt,
	}, nil
}

func scanOIDCLoginState(dialect db.Dialect, s scanner) (*model.OIDCLoginState, error) {
	var (
		state        string
		rawPool      any
		codeVerifier string
		nonce        string
		redirectURI  string
		expiresRaw   any
		createdRaw   any
	)
	if err := s.Scan(&state, &rawPool, &codeVerifier, &nonce, &redirectURI, &expiresRaw, &createdRaw); err != nil {
		return nil, err
	}
	poolUUID, err := scanUUID(dialect, rawPool)
	if err != nil {
		return nil, err
	}
	expiresAt, err := scanTime(expiresRaw)
	if err != nil {
		return nil, fmt.Errorf("scan expires_at: %w", err)
	}
	createdAt, err := scanTime(createdRaw)
	if err != nil {
		return nil, fmt.Errorf("scan created_at: %w", err)
	}
	return &model.OIDCLoginState{
		State:        state,
		PoolUUID:     poolUUID,
		CodeVerifier: codeVerifier,
		Nonce:        nonce,
		RedirectURI:  redirectURI,
		ExpiresAt:    expiresAt,
		CreatedAt:    createdAt,
	}, nil
}

func scanOptionalUUID(dialect db.Dialect, raw any) (*uuid.UUID, error) {
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
	id, err := scanUUID(dialect, raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
