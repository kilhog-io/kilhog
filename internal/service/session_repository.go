package service

import (
	"context"
	"time"

	"github.com/kilhog-io/kilhog/internal/model"
)

type SessionRepository interface {
	Create(ctx context.Context, session *model.Session) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*model.Session, error)
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
	DeleteExpired(ctx context.Context, now time.Time) error
}

type OIDCLoginStateRepository interface {
	Create(ctx context.Context, state *model.OIDCLoginState) error
	Take(ctx context.Context, state string) (*model.OIDCLoginState, error)
	DeleteExpired(ctx context.Context, now time.Time) error
}
