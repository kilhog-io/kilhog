package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

// SubnetCreateTx exposes subnet persistence operations that must run inside
// CreateAtomically so sibling reads and inserts share the same write transaction.
type SubnetCreateTx interface {
	ListSiblings(ctx context.Context) ([]*model.Subnet, error)
	GetByName(ctx context.Context, networkID uuid.UUID, name string) (*model.Subnet, error)
	Insert(ctx context.Context, subnet *model.Subnet) error
}

// SubnetRepository defines persistence operations for subnets.
type SubnetRepository interface {
	Create(ctx context.Context, subnet *model.Subnet) error
	CreateAtomically(ctx context.Context, parent model.Parent, fn func(SubnetCreateTx) (*model.Subnet, error)) (*model.Subnet, error)
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.Subnet, error)
	GetByName(ctx context.Context, networkID uuid.UUID, name string) (*model.Subnet, error)
	Update(ctx context.Context, subnet *model.Subnet) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByNetwork(ctx context.Context, networkID uuid.UUID) ([]*model.Subnet, error)
	ListByParent(ctx context.Context, parent model.Parent) ([]*model.Subnet, error)
}
