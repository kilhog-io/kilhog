package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

type MachinePoolRepository interface {
	Create(ctx context.Context, pool *model.MachinePool) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachinePool, error)
	GetBySlug(ctx context.Context, slug string) (*model.MachinePool, error)
	Update(ctx context.Context, pool *model.MachinePool) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context) ([]*model.MachinePool, error)
	CountEnabled(ctx context.Context) (int, error)
}

type MachineProviderRepository interface {
	Create(ctx context.Context, provider *model.MachineIdentityProvider) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachineIdentityProvider, error)
	GetByName(ctx context.Context, poolUUID uuid.UUID, name string) (*model.MachineIdentityProvider, error)
	GetByIssuer(ctx context.Context, poolUUID uuid.UUID, issuer string) (*model.MachineIdentityProvider, error)
	Update(ctx context.Context, provider *model.MachineIdentityProvider) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByPool(ctx context.Context, poolUUID uuid.UUID) ([]*model.MachineIdentityProvider, error)
	ListEnabledByIssuer(ctx context.Context, issuer string) ([]*model.MachineIdentityProvider, error)
}

type MachineRepository interface {
	Create(ctx context.Context, machine *model.Machine) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.Machine, error)
	GetByName(ctx context.Context, poolUUID uuid.UUID, name string) (*model.Machine, error)
	Update(ctx context.Context, machine *model.Machine) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByPool(ctx context.Context, poolUUID uuid.UUID) ([]*model.Machine, error)
	ListEnabledByProvider(ctx context.Context, providerUUID uuid.UUID) ([]*model.Machine, error)
}

type MachineAPIKeyRepository interface {
	Create(ctx context.Context, key *model.MachineAPIKey) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.MachineAPIKey, error)
	GetByPrefix(ctx context.Context, prefix string) (*model.MachineAPIKey, error)
	ListByMachine(ctx context.Context, machineUUID uuid.UUID) ([]*model.MachineAPIKey, error)
	Revoke(ctx context.Context, id uuid.UUID, at time.Time) error
	TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}
