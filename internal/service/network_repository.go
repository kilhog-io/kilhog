package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

// NetworkRepository defines persistence operations for networks.
type NetworkRepository interface {
	Create(ctx context.Context, network *model.Network) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.Network, error)
	GetByName(ctx context.Context, name string) (*model.Network, error)
	Update(ctx context.Context, network *model.Network) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*model.Network, error)
	Count(ctx context.Context) (int64, error)
}
