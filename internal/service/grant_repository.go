package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

var (
	ErrGrantNotFound      = errors.New("grant not found")
	ErrGrantConflict      = errors.New("grant already exists")
	ErrInvalidGrant         = errors.New("invalid grant")
	ErrLastOwner            = errors.New("cannot remove last owner")
	ErrGrantTargetAdmin     = errors.New("cannot grant to admin")
	ErrGrantDisabledSubject = errors.New("grant subject is disabled")
	ErrGrantSubjectNotFound = errors.New("grant subject not found")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrResourceNotVisible   = errors.New("resource not visible")
)

type Action string

const (
	ActionCreate       Action = "create"
	ActionRead         Action = "read"
	ActionUpdate       Action = "update"
	ActionDelete       Action = "delete"
	ActionManageGrants Action = "manage_grants"
)

type GrantRepository interface {
	Create(ctx context.Context, grant *model.Grant) error
	GetByUUID(ctx context.Context, id uuid.UUID) (*model.Grant, error)
	Update(ctx context.Context, grant *model.Grant) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByResource(ctx context.Context, kind model.GrantResourceKind, resourceUUID uuid.UUID) ([]*model.Grant, error)
	ListPlatform(ctx context.Context) ([]*model.Grant, error)
	ListForSubjects(ctx context.Context, subjects []model.GrantPrincipal) ([]*model.Grant, error)
	GetByPrincipalAndResource(ctx context.Context, principal model.GrantPrincipal, resource model.GrantResource) (*model.Grant, error)
	CountOwners(ctx context.Context, kind model.GrantResourceKind, resourceUUID uuid.UUID) (int, error)
	DeleteBySubnet(ctx context.Context, subnetUUID uuid.UUID) error
	ApplyTransfer(ctx context.Context, source *model.Grant, dest *model.Grant) error
}
