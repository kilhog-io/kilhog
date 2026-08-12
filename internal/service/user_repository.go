package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.LocalUser) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.LocalUser, error)
	GetByUsername(ctx context.Context, username string) (*model.LocalUser, error)
	Update(ctx context.Context, user *model.LocalUser) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*model.LocalUser, error)
	Count(ctx context.Context) (int, error)
	CountEnabledAdmins(ctx context.Context) (int, error)
}
