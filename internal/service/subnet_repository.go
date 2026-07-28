package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

// SubnetRepository defines persistence operations for subnets.
type SubnetRepository interface {
	Create(ctx context.Context, subnet *model.Subnet) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.Subnet, error)
	GetByName(ctx context.Context, networkID uuid.UUID, name string) (*model.Subnet, error)
	Update(ctx context.Context, subnet *model.Subnet) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByNetwork(ctx context.Context, networkID uuid.UUID) ([]*model.Subnet, error)
	ListByParent(ctx context.Context, parent model.Parent) ([]*model.Subnet, error)
}
