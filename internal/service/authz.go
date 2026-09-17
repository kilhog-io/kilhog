package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

type AuthorizationService struct {
	grants  GrantRepository
	subnets SubnetRepository
}

func NewAuthorizationService(grants GrantRepository, subnets SubnetRepository) *AuthorizationService {
	return &AuthorizationService{grants: grants, subnets: subnets}
}

func IsPrivilegedIPAM(principal *Principal) bool {
	if principal == nil {
		return false
	}
	return principal.IsAdmin() || principal.Kind == model.PrincipalKindAPIKey
}

func GrantSubjects(principal *Principal) []model.GrantPrincipal {
	if principal == nil {
		return nil
	}
	switch principal.Kind {
	case model.PrincipalKindLocalUser:
		if principal.LocalUser == nil || !principal.LocalUser.Enabled {
			return nil
		}
		id := principal.LocalUser.UUID
		return []model.GrantPrincipal{{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &id}}
	case model.PrincipalKindOIDC:
		if principal.IdentityPoolUUID == nil || principal.OIDCSubject == "" {
			return nil
		}
		subjects := []model.GrantPrincipal{{
			Kind:             model.GrantPrincipalOIDC,
			IdentityPoolUUID: principal.IdentityPoolUUID,
			Subject:          principal.OIDCSubject,
		}}
		for _, group := range principal.OIDCGroups {
			if group == "" {
				continue
			}
			subjects = append(subjects, model.GrantPrincipal{
				Kind:             model.GrantPrincipalOIDCGroup,
				IdentityPoolUUID: principal.IdentityPoolUUID,
				Group:            group,
			})
		}
		return subjects
	case model.PrincipalKindMachine:
		if principal.MachineUUID == nil {
			return nil
		}
		subjects := []model.GrantPrincipal{{
			Kind:        model.GrantPrincipalMachine,
			MachineUUID: principal.MachineUUID,
		}}
		if principal.MachinePoolUUID != nil {
			subjects = append(subjects, model.GrantPrincipal{
				Kind:            model.GrantPrincipalMachinePool,
				MachinePoolUUID: principal.MachinePoolUUID,
			})
		}
		return subjects
	default:
		return nil
	}
}

func (a *AuthorizationService) IsPrivileged(principal *Principal) bool {
	return IsPrivilegedIPAM(principal)
}

func (a *AuthorizationService) CanCreateNetworks(ctx context.Context, principal *Principal) (bool, error) {
	if IsPrivilegedIPAM(principal) {
		return true, nil
	}
	grants, err := a.grantsFor(ctx, principal)
	if err != nil {
		return false, err
	}
	for _, grant := range grants {
		if grant.Resource.Kind == model.GrantResourcePlatform &&
			grant.Resource.Capability == model.CapabilityCreateNetworks &&
			grant.Permissions.Create {
			return true, nil
		}
	}
	return false, nil
}

func (a *AuthorizationService) Can(ctx context.Context, principal *Principal, action Action, kind model.GrantResourceKind, resourceUUID uuid.UUID) (bool, error) {
	if action == ActionManageGrants {
		return a.IsOwner(ctx, principal, kind, resourceUUID)
	}
	if IsPrivilegedIPAM(principal) {
		return true, nil
	}
	effective, err := a.EffectivePermissions(ctx, principal, kind, resourceUUID)
	if err != nil {
		return false, err
	}
	if permissionAllows(effective.Permissions, effective.Owner, action) {
		return true, nil
	}
	if action == ActionRead {
		return a.hasStructuralRead(ctx, principal, kind, resourceUUID)
	}
	return false, nil
}

func (a *AuthorizationService) IsOwner(ctx context.Context, principal *Principal, kind model.GrantResourceKind, resourceUUID uuid.UUID) (bool, error) {
	if principal != nil && principal.IsAdmin() {
		return true, nil
	}
	effective, err := a.EffectivePermissions(ctx, principal, kind, resourceUUID)
	if err != nil {
		return false, err
	}
	return effective.Owner, nil
}

func (a *AuthorizationService) Visible(ctx context.Context, principal *Principal, kind model.GrantResourceKind, resourceUUID uuid.UUID) (bool, error) {
	if IsPrivilegedIPAM(principal) || (principal != nil && principal.IsAdmin()) {
		return true, nil
	}
	effective, err := a.EffectivePermissions(ctx, principal, kind, resourceUUID)
	if err != nil {
		return false, err
	}
	if effective.Owner || effective.Permissions.Any() {
		return true, nil
	}
	return a.hasStructuralRead(ctx, principal, kind, resourceUUID)
}

func (a *AuthorizationService) Require(ctx context.Context, principal *Principal, action Action, kind model.GrantResourceKind, resourceUUID uuid.UUID) error {
	ok, err := a.Can(ctx, principal, action, kind, resourceUUID)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	visible, err := a.Visible(ctx, principal, kind, resourceUUID)
	if err != nil {
		return err
	}
	if !visible {
		logRBACDenied(principal, action, kind, resourceUUID, "", "no applicable grant or structural visibility")
		return ErrResourceNotVisible
	}
	effective, err := a.EffectivePermissions(ctx, principal, kind, resourceUUID)
	if err != nil {
		return err
	}
	logRBACDenied(principal, action, kind, resourceUUID, "", "missing required permission", effectivePermissionAttrs(effective)...)
	return userError(ErrPermissionDenied, "missing required permission")
}

func (a *AuthorizationService) RequireOwner(ctx context.Context, principal *Principal, kind model.GrantResourceKind, resourceUUID uuid.UUID) error {
	owner, err := a.IsOwner(ctx, principal, kind, resourceUUID)
	if err != nil {
		return err
	}
	if owner {
		return nil
	}
	visible, err := a.Visible(ctx, principal, kind, resourceUUID)
	if err != nil {
		return err
	}
	if !visible {
		logRBACDenied(principal, ActionManageGrants, kind, resourceUUID, "", "no applicable grant or structural visibility")
		return ErrResourceNotVisible
	}
	logRBACDenied(principal, ActionManageGrants, kind, resourceUUID, "", "not owner of resource")
	return userError(ErrPermissionDenied, "owner access required")
}

func (a *AuthorizationService) RequireCreateNetworks(ctx context.Context, principal *Principal) error {
	ok, err := a.CanCreateNetworks(ctx, principal)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	logRBACDenied(principal, ActionCreate, model.GrantResourcePlatform, uuid.Nil, model.CapabilityCreateNetworks, "missing create_networks grant")
	return userError(ErrPermissionDenied, "missing create_networks permission")
}

func (a *AuthorizationService) VisibleNetworkUUIDs(ctx context.Context, principal *Principal, networks []*model.Network) ([]uuid.UUID, error) {
	if IsPrivilegedIPAM(principal) {
		ids := make([]uuid.UUID, 0, len(networks))
		for _, network := range networks {
			ids = append(ids, network.UUID)
		}
		return ids, nil
	}
	grants, err := a.grantsFor(ctx, principal)
	if err != nil {
		return nil, err
	}
	visible := map[uuid.UUID]struct{}{}
	for _, grant := range grants {
		switch grant.Resource.Kind {
		case model.GrantResourceNetwork:
			if grant.Resource.UUID != nil && (grant.Owner || grant.Permissions.Any()) {
				visible[*grant.Resource.UUID] = struct{}{}
			}
		case model.GrantResourceSubnet:
			if grant.NetworkUUID != nil && (grant.Owner || grant.Permissions.Any()) {
				visible[*grant.NetworkUUID] = struct{}{}
			}
		}
	}
	ids := make([]uuid.UUID, 0, len(visible))
	for _, network := range networks {
		if _, ok := visible[network.UUID]; ok {
			ids = append(ids, network.UUID)
		}
	}
	return ids, nil
}

func (a *AuthorizationService) FilterReadableSubnets(ctx context.Context, principal *Principal, networkUUID uuid.UUID, subnets []*model.Subnet) ([]*model.Subnet, error) {
	if IsPrivilegedIPAM(principal) {
		if subnets == nil {
			return []*model.Subnet{}, nil
		}
		return subnets, nil
	}
	out := make([]*model.Subnet, 0, len(subnets))
	for _, subnet := range subnets {
		ok, err := a.Can(ctx, principal, ActionRead, model.GrantResourceSubnet, subnet.UUID)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, subnet)
		}
	}
	return out, nil
}

type EffectiveAccess struct {
	Permissions model.Permissions
	Owner       bool
}

func (a *AuthorizationService) EffectivePermissions(ctx context.Context, principal *Principal, kind model.GrantResourceKind, resourceUUID uuid.UUID) (EffectiveAccess, error) {
	if principal != nil && principal.IsAdmin() {
		return EffectiveAccess{
			Permissions: model.Permissions{}.WithOwner(true),
			Owner:       true,
		}, nil
	}
	if principal != nil && principal.Kind == model.PrincipalKindAPIKey {
		return EffectiveAccess{
			Permissions: model.Permissions{}.WithOwner(true),
			Owner:       false,
		}, nil
	}

	grants, err := a.grantsFor(ctx, principal)
	if err != nil {
		return EffectiveAccess{}, err
	}

	var access EffectiveAccess
	switch kind {
	case model.GrantResourceNetwork:
		for _, grant := range grants {
			if grant.Resource.Kind == model.GrantResourceNetwork && grant.Resource.UUID != nil && *grant.Resource.UUID == resourceUUID {
				access = unionAccess(access, grant)
			}
		}
	case model.GrantResourceSubnet:
		chain, networkUUID, err := a.subnetScope(ctx, resourceUUID)
		if err != nil {
			return EffectiveAccess{}, err
		}
		inChain := map[uuid.UUID]struct{}{}
		for _, subnet := range chain {
			inChain[subnet.UUID] = struct{}{}
		}
		for _, grant := range grants {
			switch grant.Resource.Kind {
			case model.GrantResourceNetwork:
				if grant.Resource.UUID != nil && *grant.Resource.UUID == networkUUID {
					access = unionAccess(access, grant)
				}
			case model.GrantResourceSubnet:
				if grant.Resource.UUID != nil {
					if _, ok := inChain[*grant.Resource.UUID]; ok {
						access = unionAccess(access, grant)
					}
				}
			}
		}
	default:
		return EffectiveAccess{}, fmt.Errorf("unsupported resource kind %q", kind)
	}
	if access.Owner {
		access.Permissions = access.Permissions.WithOwner(true)
	}
	return access, nil
}

func (a *AuthorizationService) hasStructuralRead(ctx context.Context, principal *Principal, kind model.GrantResourceKind, resourceUUID uuid.UUID) (bool, error) {
	grants, err := a.grantsFor(ctx, principal)
	if err != nil {
		return false, err
	}
	switch kind {
	case model.GrantResourceNetwork:
		for _, grant := range grants {
			if grant.Resource.Kind == model.GrantResourceSubnet && grant.NetworkUUID != nil &&
				*grant.NetworkUUID == resourceUUID && (grant.Owner || grant.Permissions.Any()) {
				return true, nil
			}
			if grant.Resource.Kind == model.GrantResourceNetwork && grant.Resource.UUID != nil &&
				*grant.Resource.UUID == resourceUUID && (grant.Owner || grant.Permissions.Any()) {
				return true, nil
			}
		}
		return false, nil
	case model.GrantResourceSubnet:
		for _, grant := range grants {
			if !grant.Owner && !grant.Permissions.Any() {
				continue
			}
			if grant.Resource.Kind == model.GrantResourceSubnet && grant.Resource.UUID != nil {
				ok, err := a.isAncestorOrSelf(ctx, resourceUUID, *grant.Resource.UUID)
				if err != nil {
					if errors.Is(err, ErrSubnetNotFound) {
						continue
					}
					return false, err
				}
				if ok {
					return true, nil
				}
			}
		}
		return false, nil
	default:
		return false, nil
	}
}

func (a *AuthorizationService) grantsFor(ctx context.Context, principal *Principal) ([]*model.Grant, error) {
	subjects := GrantSubjects(principal)
	if len(subjects) == 0 {
		return []*model.Grant{}, nil
	}
	grants, err := a.grants.ListForSubjects(ctx, subjects)
	if err != nil {
		return nil, fmt.Errorf("list grants for subjects: %w", err)
	}
	if grants == nil {
		grants = []*model.Grant{}
	}
	return grants, nil
}

func (a *AuthorizationService) subnetScope(ctx context.Context, subnetUUID uuid.UUID) ([]*model.Subnet, uuid.UUID, error) {
	var chain []*model.Subnet
	currentUUID := subnetUUID
	for i := 0; i < 64; i++ {
		subnet, err := a.subnets.GetByUUID(ctx, currentUUID)
		if err != nil {
			if errors.Is(err, ErrSubnetNotFound) {
				return nil, uuid.Nil, ErrSubnetNotFound
			}
			return nil, uuid.Nil, fmt.Errorf("get subnet: %w", err)
		}
		chain = append(chain, subnet)
		if subnet.Parent.Kind == model.ParentKindNetwork {
			return chain, subnet.Parent.UUID, nil
		}
		currentUUID = subnet.Parent.UUID
	}
	return nil, uuid.Nil, fmt.Errorf("subnet hierarchy too deep")
}

func (a *AuthorizationService) isAncestorOrSelf(ctx context.Context, ancestor, descendant uuid.UUID) (bool, error) {
	if ancestor == descendant {
		return true, nil
	}
	chain, _, err := a.subnetScope(ctx, descendant)
	if err != nil {
		return false, err
	}
	for _, subnet := range chain {
		if subnet.UUID == ancestor {
			return true, nil
		}
	}
	return false, nil
}

func unionAccess(access EffectiveAccess, grant *model.Grant) EffectiveAccess {
	access.Permissions = access.Permissions.Union(grant.Permissions.WithOwner(grant.Owner))
	access.Owner = access.Owner || grant.Owner
	return access
}

func permissionAllows(perms model.Permissions, owner bool, action Action) bool {
	if owner {
		return true
	}
	switch action {
	case ActionCreate:
		return perms.Create
	case ActionRead:
		return perms.Read
	case ActionUpdate:
		return perms.Update
	case ActionDelete:
		return perms.Delete
	case ActionManageGrants:
		return false
	default:
		return false
	}
}

func logRBACDenied(principal *Principal, action Action, kind model.GrantResourceKind, resourceUUID uuid.UUID, capability, reason string, extra ...any) {
	attrs := []any{
		"reason", reason,
		"action", string(action),
		"resource_kind", string(kind),
	}
	if resourceUUID != uuid.Nil {
		attrs = append(attrs, "resource_uuid", resourceUUID.String())
	}
	if capability != "" {
		attrs = append(attrs, "capability", capability)
	}
	attrs = append(attrs, principalLogAttrs(principal)...)
	attrs = append(attrs, extra...)
	slog.Debug("rbac denied", attrs...)
}

func principalLogAttrs(principal *Principal) []any {
	if principal == nil {
		return []any{"principal_kind", "none"}
	}
	attrs := []any{"principal_kind", string(principal.Kind)}
	switch principal.Kind {
	case model.PrincipalKindLocalUser:
		if principal.LocalUser != nil {
			attrs = append(attrs,
				"username", principal.LocalUser.Username,
				"local_user_uuid", principal.LocalUser.UUID.String(),
				"role", string(principal.LocalUser.Role),
				"enabled", principal.LocalUser.Enabled,
			)
		}
	case model.PrincipalKindOIDC:
		if principal.IdentityPoolUUID != nil {
			attrs = append(attrs, "identity_pool_uuid", principal.IdentityPoolUUID.String())
		}
		attrs = append(attrs, "oidc_subject", principal.OIDCSubject)
		if len(principal.OIDCGroups) > 0 {
			attrs = append(attrs, "oidc_groups", strings.Join(principal.OIDCGroups, ","))
		}
	case model.PrincipalKindMachine:
		if principal.MachineUUID != nil {
			attrs = append(attrs, "machine_uuid", principal.MachineUUID.String())
		}
		if principal.MachinePoolUUID != nil {
			attrs = append(attrs, "machine_pool_uuid", principal.MachinePoolUUID.String())
		}
		if principal.MachineName != "" {
			attrs = append(attrs, "machine_name", principal.MachineName)
		}
	}
	if subjects := GrantSubjects(principal); len(subjects) > 0 {
		attrs = append(attrs, "grant_subjects", formatGrantSubjects(subjects))
	}
	return attrs
}

func formatGrantSubjects(subjects []model.GrantPrincipal) string {
	parts := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		parts = append(parts, formatGrantSubject(subject))
	}
	return strings.Join(parts, ",")
}

func formatGrantSubject(subject model.GrantPrincipal) string {
	switch subject.Kind {
	case model.GrantPrincipalLocalUser:
		if subject.LocalUserUUID != nil {
			return "local_user:" + subject.LocalUserUUID.String()
		}
	case model.GrantPrincipalOIDC:
		pool := ""
		if subject.IdentityPoolUUID != nil {
			pool = subject.IdentityPoolUUID.String()
		}
		return "oidc:" + pool + "/" + subject.Subject
	case model.GrantPrincipalOIDCGroup:
		pool := ""
		if subject.IdentityPoolUUID != nil {
			pool = subject.IdentityPoolUUID.String()
		}
		return "oidc_group:" + pool + "/" + subject.Group
	case model.GrantPrincipalMachine:
		if subject.MachineUUID != nil {
			return "machine:" + subject.MachineUUID.String()
		}
	case model.GrantPrincipalMachinePool:
		if subject.MachinePoolUUID != nil {
			return "machine_pool:" + subject.MachinePoolUUID.String()
		}
	}
	return string(subject.Kind)
}

func effectivePermissionAttrs(access EffectiveAccess) []any {
	return []any{
		"effective_create", access.Permissions.Create,
		"effective_read", access.Permissions.Read,
		"effective_update", access.Permissions.Update,
		"effective_delete", access.Permissions.Delete,
		"effective_owner", access.Owner,
	}
}
