package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

type GrantService struct {
	grants       GrantRepository
	users        UserRepository
	pools        IdentityPoolRepository
	machines     MachineRepository
	machinePools MachinePoolRepository
	networks     NetworkRepository
	subnets      SubnetRepository
}

func NewGrantService(
	grants GrantRepository,
	users UserRepository,
	pools IdentityPoolRepository,
	machines MachineRepository,
	machinePools MachinePoolRepository,
	networks NetworkRepository,
	subnets SubnetRepository,
) *GrantService {
	return &GrantService{
		grants:       grants,
		users:        users,
		pools:        pools,
		machines:     machines,
		machinePools: machinePools,
		networks:     networks,
		subnets:      subnets,
	}
}

type CreateGrantInput struct {
	Principal   model.GrantPrincipal
	Resource    model.GrantResource
	Permissions model.Permissions
	Owner       bool
}

type UpdateGrantInput struct {
	Permissions model.Permissions
	Owner       bool
}

type TransferInput struct {
	From *model.GrantPrincipal
	To   model.GrantPrincipal
}

func (s *GrantService) Create(ctx context.Context, caller *Principal, input CreateGrantInput) (*model.Grant, error) {
	grant, err := s.buildGrant(ctx, input)
	if err != nil {
		return nil, err
	}
	if err := s.grants.Create(ctx, grant); err != nil {
		if errors.Is(err, ErrGrantConflict) {
			return nil, userError(ErrGrantConflict, "a grant already exists for this principal and resource")
		}
		return nil, fmt.Errorf("create grant: %w", err)
	}
	return grant, nil
}

func (s *GrantService) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Grant, error) {
	grant, err := s.grants.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			return nil, ErrGrantNotFound
		}
		return nil, fmt.Errorf("get grant: %w", err)
	}
	return grant, nil
}

func (s *GrantService) Update(ctx context.Context, caller *Principal, id uuid.UUID, input UpdateGrantInput) (*model.Grant, error) {
	grant, err := s.grants.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			return nil, ErrGrantNotFound
		}
		return nil, fmt.Errorf("get grant: %w", err)
	}

	if grant.Resource.Kind == model.GrantResourcePlatform {
		if input.Owner {
			return nil, userError(ErrInvalidGrant, "platform grants cannot have owner")
		}
		grant.Permissions = model.Permissions{Create: true}
		grant.Owner = false
	} else {
		perms := input.Permissions
		if input.Owner {
			perms = perms.WithOwner(true)
		}
		if !perms.Any() && !input.Owner {
			return nil, userError(ErrInvalidGrant, "at least one permission or owner is required")
		}
		grant.Permissions = perms
		grant.Owner = input.Owner
	}

	if err := s.enforceLastOwnerOnUpdate(ctx, caller, grant); err != nil {
		return nil, err
	}
	if err := s.grants.Update(ctx, grant); err != nil {
		return nil, fmt.Errorf("update grant: %w", err)
	}
	return grant, nil
}

func (s *GrantService) Delete(ctx context.Context, caller *Principal, id uuid.UUID) error {
	grant, err := s.grants.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			return ErrGrantNotFound
		}
		return fmt.Errorf("get grant: %w", err)
	}
	if err := s.enforceLastOwnerOnDelete(ctx, caller, grant); err != nil {
		return err
	}
	if err := s.grants.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete grant: %w", err)
	}
	return nil
}

func (s *GrantService) ListByResource(ctx context.Context, kind model.GrantResourceKind, resourceUUID uuid.UUID) ([]*model.Grant, error) {
	grants, err := s.grants.ListByResource(ctx, kind, resourceUUID)
	if err != nil {
		return nil, fmt.Errorf("list grants: %w", err)
	}
	if grants == nil {
		grants = []*model.Grant{}
	}
	return grants, nil
}

func (s *GrantService) ListPlatform(ctx context.Context) ([]*model.Grant, error) {
	grants, err := s.grants.ListPlatform(ctx)
	if err != nil {
		return nil, fmt.Errorf("list platform grants: %w", err)
	}
	if grants == nil {
		grants = []*model.Grant{}
	}
	return grants, nil
}

func (s *GrantService) ListEffectiveForPrincipal(ctx context.Context, principal *Principal) ([]*model.Grant, error) {
	if principal == nil || principal.Kind == model.PrincipalKindAPIKey {
		return []*model.Grant{}, nil
	}
	subjects := GrantSubjects(principal)
	if len(subjects) == 0 {
		return []*model.Grant{}, nil
	}
	grants, err := s.grants.ListForSubjects(ctx, subjects)
	if err != nil {
		return nil, fmt.Errorf("list grants for principal: %w", err)
	}
	if grants == nil {
		grants = []*model.Grant{}
	}
	return grants, nil
}

func (s *GrantService) EnsureOwner(ctx context.Context, principal *Principal, networkUUID uuid.UUID) error {
	if principal == nil || IsPrivilegedIPAM(principal) {
		return nil
	}
	subject, err := principalAsGrantSubject(principal)
	if err != nil {
		return err
	}
	resource := model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &networkUUID}
	existing, err := s.grants.GetByPrincipalAndResource(ctx, subject, resource)
	if err != nil && !errors.Is(err, ErrGrantNotFound) {
		return fmt.Errorf("lookup owner grant: %w", err)
	}
	if existing != nil {
		if existing.Owner {
			return nil
		}
		existing.Owner = true
		existing.Permissions = existing.Permissions.WithOwner(true)
		if err := s.grants.Update(ctx, existing); err != nil {
			return fmt.Errorf("update owner grant: %w", err)
		}
		return nil
	}

	grant := &model.Grant{
		UUID:        uuid.New(),
		Principal:   subject,
		Resource:    resource,
		Permissions: model.Permissions{}.WithOwner(true),
		Owner:       true,
		NetworkUUID: &networkUUID,
	}
	if err := s.grants.Create(ctx, grant); err != nil && !errors.Is(err, ErrGrantConflict) {
		return fmt.Errorf("create owner grant: %w", err)
	}
	return nil
}

func (s *GrantService) Transfer(ctx context.Context, caller *Principal, resource model.GrantResource, input TransferInput) (*model.Grant, error) {
	if resource.Kind != model.GrantResourceNetwork && resource.Kind != model.GrantResourceSubnet {
		return nil, userError(ErrInvalidGrant, "ownership transfer only applies to networks and subnets")
	}
	if resource.UUID == nil {
		return nil, userError(ErrInvalidGrant, "resource uuid is required")
	}

	to, err := s.validateSubject(ctx, input.To, true)
	if err != nil {
		return nil, err
	}

	from, err := s.resolveTransferFrom(ctx, caller, resource, input.From)
	if err != nil {
		return nil, err
	}
	if principalsEqual(from.Principal, to) {
		return nil, userError(ErrInvalidGrant, "source and destination principals must differ")
	}

	networkUUID, err := s.networkUUIDForResource(ctx, resource)
	if err != nil {
		return nil, err
	}

	dest := &model.Grant{
		UUID:        uuid.New(),
		Principal:   to,
		Resource:    resource,
		Owner:       true,
		Permissions: model.Permissions{}.WithOwner(true),
		NetworkUUID: networkUUID,
	}

	// Owner-implied CRUD is not kept on the source after transfer.
	from.Owner = false
	from.Permissions = model.Permissions{}

	if err := s.grants.ApplyTransfer(ctx, from, dest); err != nil {
		if errors.Is(err, ErrGrantConflict) {
			return nil, userError(ErrGrantConflict, "a grant already exists for this principal and resource")
		}
		return nil, fmt.Errorf("transfer ownership: %w", err)
	}

	updated, err := s.grants.GetByPrincipalAndResource(ctx, to, resource)
	if err != nil {
		return nil, fmt.Errorf("reload transferred grant: %w", err)
	}
	return updated, nil
}

func (s *GrantService) buildGrant(ctx context.Context, input CreateGrantInput) (*model.Grant, error) {
	principal, err := s.validateSubject(ctx, input.Principal, true)
	if err != nil {
		return nil, err
	}
	resource, networkUUID, err := s.validateResource(ctx, input.Resource)
	if err != nil {
		return nil, err
	}

	perms := input.Permissions
	owner := input.Owner
	if resource.Kind == model.GrantResourcePlatform {
		if owner {
			return nil, userError(ErrInvalidGrant, "platform grants cannot have owner")
		}
		if resource.Capability != model.CapabilityCreateNetworks {
			return nil, userError(ErrInvalidGrant, "unsupported platform capability")
		}
		perms = model.Permissions{Create: true}
		owner = false
	} else {
		if owner {
			perms = perms.WithOwner(true)
		}
		if !perms.Any() && !owner {
			return nil, userError(ErrInvalidGrant, "at least one permission or owner is required")
		}
	}

	return &model.Grant{
		UUID:        uuid.New(),
		Principal:   principal,
		Resource:    resource,
		Permissions: perms,
		Owner:       owner,
		NetworkUUID: networkUUID,
	}, nil
}

func (s *GrantService) validateResource(ctx context.Context, resource model.GrantResource) (model.GrantResource, *uuid.UUID, error) {
	switch resource.Kind {
	case model.GrantResourcePlatform:
		if strings.TrimSpace(resource.Capability) == "" {
			resource.Capability = model.CapabilityCreateNetworks
		}
		if resource.Capability != model.CapabilityCreateNetworks {
			return model.GrantResource{}, nil, userError(ErrInvalidGrant, "unsupported platform capability")
		}
		resource.UUID = nil
		return resource, nil, nil
	case model.GrantResourceNetwork:
		if resource.UUID == nil {
			return model.GrantResource{}, nil, userError(ErrInvalidGrant, "network uuid is required")
		}
		if _, err := s.networks.GetByUUID(ctx, *resource.UUID); err != nil {
			if errors.Is(err, ErrNetworkNotFound) {
				return model.GrantResource{}, nil, ErrNetworkNotFound
			}
			return model.GrantResource{}, nil, fmt.Errorf("get network: %w", err)
		}
		resource.Capability = ""
		return resource, resource.UUID, nil
	case model.GrantResourceSubnet:
		if resource.UUID == nil {
			return model.GrantResource{}, nil, userError(ErrInvalidGrant, "subnet uuid is required")
		}
		subnet, err := s.subnets.GetByUUID(ctx, *resource.UUID)
		if err != nil {
			if errors.Is(err, ErrSubnetNotFound) {
				return model.GrantResource{}, nil, ErrSubnetNotFound
			}
			return model.GrantResource{}, nil, fmt.Errorf("get subnet: %w", err)
		}
		networkUUID, err := s.resolveSubnetNetwork(ctx, subnet)
		if err != nil {
			return model.GrantResource{}, nil, err
		}
		resource.Capability = ""
		return resource, &networkUUID, nil
	default:
		return model.GrantResource{}, nil, userError(ErrInvalidGrant, "unknown resource kind %q", resource.Kind)
	}
}

func (s *GrantService) validateSubject(ctx context.Context, principal model.GrantPrincipal, requireEnabled bool) (model.GrantPrincipal, error) {
	principal.Kind = model.GrantPrincipalKind(strings.TrimSpace(string(principal.Kind)))
	switch principal.Kind {
	case model.GrantPrincipalLocalUser:
		if principal.LocalUserUUID == nil {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "local_user_uuid is required")
		}
		user, err := s.users.GetByUUID(ctx, *principal.LocalUserUUID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				return model.GrantPrincipal{}, userError(ErrGrantSubjectNotFound, "local user not found")
			}
			return model.GrantPrincipal{}, fmt.Errorf("get local user: %w", err)
		}
		if user.Role == model.UserRoleAdmin {
			return model.GrantPrincipal{}, userError(ErrGrantTargetAdmin, "cannot grant to a local admin")
		}
		if requireEnabled && !user.Enabled {
			return model.GrantPrincipal{}, userError(ErrGrantDisabledSubject, "grant subject is disabled")
		}
		return model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &user.UUID}, nil
	case model.GrantPrincipalOIDC:
		if principal.IdentityPoolUUID == nil || strings.TrimSpace(principal.Subject) == "" {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "identity_pool_uuid and subject are required")
		}
		pool, err := s.pools.GetByUUID(ctx, *principal.IdentityPoolUUID)
		if err != nil {
			if errors.Is(err, ErrIdentityPoolNotFound) {
				return model.GrantPrincipal{}, userError(ErrGrantSubjectNotFound, "identity pool not found")
			}
			return model.GrantPrincipal{}, fmt.Errorf("get identity pool: %w", err)
		}
		if requireEnabled && !pool.Enabled {
			return model.GrantPrincipal{}, userError(ErrGrantDisabledSubject, "grant subject is disabled")
		}
		subject := strings.TrimSpace(principal.Subject)
		return model.GrantPrincipal{
			Kind:             model.GrantPrincipalOIDC,
			IdentityPoolUUID: &pool.UUID,
			Subject:          subject,
		}, nil
	case model.GrantPrincipalOIDCGroup:
		if principal.IdentityPoolUUID == nil || strings.TrimSpace(principal.Group) == "" {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "identity_pool_uuid and group are required")
		}
		pool, err := s.pools.GetByUUID(ctx, *principal.IdentityPoolUUID)
		if err != nil {
			if errors.Is(err, ErrIdentityPoolNotFound) {
				return model.GrantPrincipal{}, userError(ErrGrantSubjectNotFound, "identity pool not found")
			}
			return model.GrantPrincipal{}, fmt.Errorf("get identity pool: %w", err)
		}
		if requireEnabled && !pool.Enabled {
			return model.GrantPrincipal{}, userError(ErrGrantDisabledSubject, "grant subject is disabled")
		}
		return model.GrantPrincipal{
			Kind:             model.GrantPrincipalOIDCGroup,
			IdentityPoolUUID: &pool.UUID,
			Group:            strings.TrimSpace(principal.Group),
		}, nil
	case model.GrantPrincipalMachine:
		if principal.MachineUUID == nil {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "machine_uuid is required")
		}
		machine, err := s.machines.GetByUUID(ctx, *principal.MachineUUID)
		if err != nil {
			if errors.Is(err, ErrMachineNotFound) {
				return model.GrantPrincipal{}, userError(ErrGrantSubjectNotFound, "machine not found")
			}
			return model.GrantPrincipal{}, fmt.Errorf("get machine: %w", err)
		}
		pool, err := s.machinePools.GetByUUID(ctx, machine.MachinePoolUUID)
		if err != nil {
			return model.GrantPrincipal{}, fmt.Errorf("get machine pool: %w", err)
		}
		if requireEnabled && (!machine.Enabled || !pool.Enabled) {
			return model.GrantPrincipal{}, userError(ErrGrantDisabledSubject, "grant subject is disabled")
		}
		return model.GrantPrincipal{Kind: model.GrantPrincipalMachine, MachineUUID: &machine.UUID}, nil
	case model.GrantPrincipalMachinePool:
		if principal.MachinePoolUUID == nil {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "machine_pool_uuid is required")
		}
		pool, err := s.machinePools.GetByUUID(ctx, *principal.MachinePoolUUID)
		if err != nil {
			if errors.Is(err, ErrMachinePoolNotFound) {
				return model.GrantPrincipal{}, userError(ErrGrantSubjectNotFound, "machine pool not found")
			}
			return model.GrantPrincipal{}, fmt.Errorf("get machine pool: %w", err)
		}
		if requireEnabled && !pool.Enabled {
			return model.GrantPrincipal{}, userError(ErrGrantDisabledSubject, "grant subject is disabled")
		}
		return model.GrantPrincipal{Kind: model.GrantPrincipalMachinePool, MachinePoolUUID: &pool.UUID}, nil
	default:
		return model.GrantPrincipal{}, userError(ErrInvalidGrant, "unknown principal kind %q", principal.Kind)
	}
}

func (s *GrantService) resolveTransferFrom(ctx context.Context, caller *Principal, resource model.GrantResource, from *model.GrantPrincipal) (*model.Grant, error) {
	var subject model.GrantPrincipal
	if from == nil {
		own, err := principalAsGrantSubject(caller)
		if err != nil {
			return nil, userError(ErrInvalidGrant, "from is required when the caller has no personal owner grant")
		}
		subject = own
	} else {
		normalized, err := s.validateSubject(ctx, *from, false)
		if err != nil {
			return nil, err
		}
		subject = normalized
	}

	grant, err := s.grants.GetByPrincipalAndResource(ctx, subject, resource)
	if err != nil {
		if errors.Is(err, ErrGrantNotFound) {
			return nil, userError(ErrGrantNotFound, "source owner grant not found")
		}
		return nil, fmt.Errorf("get source grant: %w", err)
	}
	if !grant.Owner {
		return nil, userError(ErrGrantNotFound, "source owner grant not found")
	}
	return grant, nil
}

func (s *GrantService) enforceLastOwnerOnUpdate(ctx context.Context, caller *Principal, grant *model.Grant) error {
	if caller != nil && caller.IsAdmin() {
		return nil
	}
	if grant.Owner || grant.Resource.Kind != model.GrantResourceNetwork || grant.Resource.UUID == nil {
		return nil
	}
	return s.refuseIfLastNetworkOwner(ctx, grant)
}

func (s *GrantService) enforceLastOwnerOnDelete(ctx context.Context, caller *Principal, grant *model.Grant) error {
	if caller != nil && caller.IsAdmin() {
		return nil
	}
	if !grant.Owner || grant.Resource.Kind != model.GrantResourceNetwork || grant.Resource.UUID == nil {
		return nil
	}
	return s.refuseIfLastNetworkOwner(ctx, grant)
}

func (s *GrantService) refuseIfLastNetworkOwner(ctx context.Context, grant *model.Grant) error {
	if !grant.Owner {
		// Reloading original is needed when updating owner to false.
		original, err := s.grants.GetByUUID(ctx, grant.UUID)
		if err != nil {
			return fmt.Errorf("get grant: %w", err)
		}
		if !original.Owner {
			return nil
		}
	}
	n, err := s.grants.CountOwners(ctx, model.GrantResourceNetwork, *grant.Resource.UUID)
	if err != nil {
		return fmt.Errorf("count owners: %w", err)
	}
	if n <= 1 {
		return userError(ErrLastOwner, "cannot remove the last owner of this network")
	}
	return nil
}

func (s *GrantService) networkUUIDForResource(ctx context.Context, resource model.GrantResource) (*uuid.UUID, error) {
	switch resource.Kind {
	case model.GrantResourceNetwork:
		return resource.UUID, nil
	case model.GrantResourceSubnet:
		if resource.UUID == nil {
			return nil, userError(ErrInvalidGrant, "subnet uuid is required")
		}
		subnet, err := s.subnets.GetByUUID(ctx, *resource.UUID)
		if err != nil {
			return nil, err
		}
		id, err := s.resolveSubnetNetwork(ctx, subnet)
		if err != nil {
			return nil, err
		}
		return &id, nil
	default:
		return nil, nil
	}
}

func (s *GrantService) resolveSubnetNetwork(ctx context.Context, subnet *model.Subnet) (uuid.UUID, error) {
	current := subnet
	for i := 0; i < 64; i++ {
		if current.Parent.Kind == model.ParentKindNetwork {
			return current.Parent.UUID, nil
		}
		parent, err := s.subnets.GetByUUID(ctx, current.Parent.UUID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("resolve subnet network: %w", err)
		}
		current = parent
	}
	return uuid.Nil, fmt.Errorf("resolve subnet network: hierarchy too deep")
}

func principalAsGrantSubject(principal *Principal) (model.GrantPrincipal, error) {
	if principal == nil {
		return model.GrantPrincipal{}, userError(ErrInvalidGrant, "principal is required")
	}
	switch principal.Kind {
	case model.PrincipalKindLocalUser:
		if principal.LocalUser == nil {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "local user is required")
		}
		id := principal.LocalUser.UUID
		return model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &id}, nil
	case model.PrincipalKindOIDC:
		if principal.IdentityPoolUUID == nil || principal.OIDCSubject == "" {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "oidc principal is incomplete")
		}
		return model.GrantPrincipal{
			Kind:             model.GrantPrincipalOIDC,
			IdentityPoolUUID: principal.IdentityPoolUUID,
			Subject:          principal.OIDCSubject,
		}, nil
	case model.PrincipalKindMachine:
		if principal.MachineUUID == nil {
			return model.GrantPrincipal{}, userError(ErrInvalidGrant, "machine principal is incomplete")
		}
		return model.GrantPrincipal{Kind: model.GrantPrincipalMachine, MachineUUID: principal.MachineUUID}, nil
	default:
		return model.GrantPrincipal{}, userError(ErrInvalidGrant, "this principal cannot own a resource")
	}
}

func principalsEqual(a, b model.GrantPrincipal) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case model.GrantPrincipalLocalUser:
		return a.LocalUserUUID != nil && b.LocalUserUUID != nil && *a.LocalUserUUID == *b.LocalUserUUID
	case model.GrantPrincipalOIDC:
		return a.IdentityPoolUUID != nil && b.IdentityPoolUUID != nil &&
			*a.IdentityPoolUUID == *b.IdentityPoolUUID && a.Subject == b.Subject
	case model.GrantPrincipalOIDCGroup:
		return a.IdentityPoolUUID != nil && b.IdentityPoolUUID != nil &&
			*a.IdentityPoolUUID == *b.IdentityPoolUUID && a.Group == b.Group
	case model.GrantPrincipalMachine:
		return a.MachineUUID != nil && b.MachineUUID != nil && *a.MachineUUID == *b.MachineUUID
	case model.GrantPrincipalMachinePool:
		return a.MachinePoolUUID != nil && b.MachinePoolUUID != nil && *a.MachinePoolUUID == *b.MachinePoolUUID
	default:
		return false
	}
}

func (s *GrantService) GrantMatchesResource(grant *model.Grant, kind model.GrantResourceKind, resourceUUID uuid.UUID) bool {
	if grant == nil || grant.Resource.Kind != kind || grant.Resource.UUID == nil {
		return false
	}
	return *grant.Resource.UUID == resourceUUID
}
