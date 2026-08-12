package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

type IdentityPoolRepository interface {
	Create(ctx context.Context, pool *model.IdentityPool) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.IdentityPool, error)
	GetBySlug(ctx context.Context, slug string) (*model.IdentityPool, error)
	GetByIssuer(ctx context.Context, issuer string) (*model.IdentityPool, error)
	Update(ctx context.Context, pool *model.IdentityPool) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*model.IdentityPool, error)
	ListEnabled(ctx context.Context) ([]*model.IdentityPool, error)
	CountEnabled(ctx context.Context) (int, error)
}
