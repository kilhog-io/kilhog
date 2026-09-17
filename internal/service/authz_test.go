package service_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

type rbacEnv struct {
	repos  *repository.Repositories
	authz  *service.AuthorizationService
	grants *service.GrantService
	users  *service.UserService
	nets   *service.NetworkService
	subs   *service.SubnetService
	admin  *service.Principal
	user   *service.Principal
	other  *service.Principal
	apiKey *service.Principal
}

func openRBAC(t *testing.T) *rbacEnv {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")
	repos, err := repository.Open(t.Context(), db.Config{
		Driver:      db.DialectSQLite,
		DSN:         "file:" + dbPath + "?_pragma=foreign_keys(ON)",
		AutoMigrate: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repos.Close() })

	users := service.NewUserService(repos.Users)
	adminUser, err := users.Create(t.Context(), service.CreateUserInput{
		Username: "admin",
		Password: "password123",
		Role:     model.UserRoleAdmin,
	})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	op, err := users.Create(t.Context(), service.CreateUserInput{
		Username: "operator",
		Password: "password123",
		Role:     model.UserRoleUser,
	})
	if err != nil {
		t.Fatalf("create operator: %v", err)
	}
	other, err := users.Create(t.Context(), service.CreateUserInput{
		Username: "other",
		Password: "password123",
		Role:     model.UserRoleUser,
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	return &rbacEnv{
		repos:  repos,
		authz:  service.NewAuthorizationService(repos.Grants, repos.Subnets),
		grants: service.NewGrantService(repos.Grants, repos.Users, repos.IdentityPools, repos.Machines, repos.MachinePools, repos.Networks, repos.Subnets),
		users:  users,
		nets:   service.NewNetworkService(repos.Networks, repos.Subnets),
		subs:   service.NewSubnetService(repos.Subnets, repos.Networks),
		admin:  &service.Principal{Kind: model.PrincipalKindLocalUser, LocalUser: adminUser},
		user:   &service.Principal{Kind: model.PrincipalKindLocalUser, LocalUser: op},
		other:  &service.Principal{Kind: model.PrincipalKindLocalUser, LocalUser: other},
		apiKey: &service.Principal{Kind: model.PrincipalKindAPIKey},
	}
}

func (e *rbacEnv) createNetwork(t *testing.T, name string) *model.Network {
	t.Helper()
	network, err := e.nets.Create(t.Context(), service.CreateNetworkInput{Name: name})
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	return network
}

func (e *rbacEnv) createSubnet(t *testing.T, network uuid.UUID, name, address string, prefix int, parent model.Parent) *model.Subnet {
	t.Helper()
	subnet, err := e.subs.CreateInNetwork(t.Context(), network, parent, service.CreateSubnetInput{
		Name:    name,
		Prefix:  prefix,
		Address: address,
		Type:    model.AddressTypeIPv4,
	})
	if err != nil {
		t.Fatalf("create subnet %s: %v", name, err)
	}
	return subnet
}

func (e *rbacEnv) grant(t *testing.T, principal *service.Principal, input service.CreateGrantInput) *model.Grant {
	t.Helper()
	grant, err := e.grants.Create(t.Context(), principal, input)
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	return grant
}

func TestAuthorization_PrivilegedBypass(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "lab")

	tests := []struct {
		name      string
		principal *service.Principal
		wantIPAM  bool
		wantOwner bool
	}{
		{name: "admin", principal: e.admin, wantIPAM: true, wantOwner: true},
		{name: "api key", principal: e.apiKey, wantIPAM: true, wantOwner: false},
		{name: "user deny", principal: e.user, wantIPAM: false, wantOwner: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := e.authz.Can(ctx, tt.principal, service.ActionRead, model.GrantResourceNetwork, network.UUID)
			if err != nil {
				t.Fatalf("Can: %v", err)
			}
			if ok != tt.wantIPAM {
				t.Fatalf("Can(read) = %v, want %v", ok, tt.wantIPAM)
			}
			owner, err := e.authz.IsOwner(ctx, tt.principal, model.GrantResourceNetwork, network.UUID)
			if err != nil {
				t.Fatalf("IsOwner: %v", err)
			}
			if owner != tt.wantOwner {
				t.Fatalf("IsOwner = %v, want %v", owner, tt.wantOwner)
			}
			create, err := e.authz.CanCreateNetworks(ctx, tt.principal)
			if err != nil {
				t.Fatalf("CanCreateNetworks: %v", err)
			}
			if create != tt.wantIPAM {
				t.Fatalf("CanCreateNetworks = %v, want %v", create, tt.wantIPAM)
			}
		})
	}
}

func TestAuthorization_CreateNetworksAndAutoOwner(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)

	ok, err := e.authz.CanCreateNetworks(ctx, e.user)
	if err != nil {
		t.Fatalf("CanCreateNetworks: %v", err)
	}
	if ok {
		t.Fatal("expected default deny")
	}

	e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:  model.GrantResource{Kind: model.GrantResourcePlatform, Capability: model.CapabilityCreateNetworks},
	})
	ok, err = e.authz.CanCreateNetworks(ctx, e.user)
	if err != nil {
		t.Fatalf("CanCreateNetworks: %v", err)
	}
	if !ok {
		t.Fatal("expected create_networks after platform grant")
	}

	network := e.createNetwork(t, "owned")
	if err := e.grants.EnsureOwner(ctx, e.user, network.UUID); err != nil {
		t.Fatalf("EnsureOwner: %v", err)
	}
	owner, err := e.authz.IsOwner(ctx, e.user, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("IsOwner: %v", err)
	}
	if !owner {
		t.Fatal("creator should become owner")
	}
	if err := e.authz.Require(ctx, e.user, service.ActionDelete, model.GrantResourceNetwork, network.UUID); err != nil {
		t.Fatalf("owner should have delete: %v", err)
	}
}

func TestAuthorization_ShareAndTransfer(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "tenancy")
	e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:  model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Owner:     true,
	})

	shared := e.grant(t, e.user, service.CreateGrantInput{
		Principal: model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.other.LocalUser.UUID},
		Resource:  model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Owner:     true,
	})
	if !shared.Owner {
		t.Fatal("share should set owner")
	}
	owner, err := e.authz.IsOwner(ctx, e.user, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("IsOwner user: %v", err)
	}
	if !owner {
		t.Fatal("original owner should remain")
	}

	from := model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID}
	to := model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.other.LocalUser.UUID}
	if _, err := e.grants.Transfer(ctx, e.user, model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID}, service.TransferInput{
		From: &from,
		To:   to,
	}); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	owner, err = e.authz.IsOwner(ctx, e.user, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("IsOwner after transfer: %v", err)
	}
	if owner {
		t.Fatal("source should lose ownership")
	}
	owner, err = e.authz.IsOwner(ctx, e.other, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("IsOwner dest: %v", err)
	}
	if !owner {
		t.Fatal("destination should own the network")
	}
}

func TestAuthorization_LastOwnerProtection(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "solo")
	grant := e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:  model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Owner:     true,
	})

	if err := e.grants.Delete(ctx, e.user, grant.UUID); !errors.Is(err, service.ErrLastOwner) {
		t.Fatalf("Delete last owner = %v, want ErrLastOwner", err)
	}
	if _, err := e.grants.Update(ctx, e.user, grant.UUID, service.UpdateGrantInput{Permissions: model.Permissions{Read: true}, Owner: false}); !errors.Is(err, service.ErrLastOwner) {
		t.Fatalf("Update drop owner = %v, want ErrLastOwner", err)
	}
	if err := e.grants.Delete(ctx, e.admin, grant.UUID); err != nil {
		t.Fatalf("admin can delete last owner: %v", err)
	}
}

func TestAuthorization_InheritanceAndStructuralRead(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "tree")
	parent := e.createSubnet(t, network.UUID, "root", "10.0.0.0", 16, model.Parent{Kind: model.ParentKindNetwork, UUID: network.UUID})
	child := e.createSubnet(t, network.UUID, "child", "10.0.1.0", 24, model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID})
	sibling := e.createSubnet(t, network.UUID, "sib", "10.0.2.0", 24, model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID})

	e.grant(t, e.admin, service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceSubnet, UUID: &child.UUID},
		Permissions: model.Permissions{Read: true, Update: true},
	})

	if err := e.authz.Require(ctx, e.user, service.ActionRead, model.GrantResourceSubnet, child.UUID); err != nil {
		t.Fatalf("read child: %v", err)
	}
	if err := e.authz.Require(ctx, e.user, service.ActionRead, model.GrantResourceSubnet, parent.UUID); err != nil {
		t.Fatalf("structural read parent: %v", err)
	}
	if err := e.authz.Require(ctx, e.user, service.ActionRead, model.GrantResourceNetwork, network.UUID); err != nil {
		t.Fatalf("structural read network: %v", err)
	}
	if err := e.authz.Require(ctx, e.user, service.ActionUpdate, model.GrantResourceSubnet, parent.UUID); !errors.Is(err, service.ErrPermissionDenied) {
		t.Fatalf("update parent = %v, want permission denied", err)
	}
	if err := e.authz.Require(ctx, e.user, service.ActionRead, model.GrantResourceSubnet, sibling.UUID); !errors.Is(err, service.ErrResourceNotVisible) {
		t.Fatalf("sibling read = %v, want not visible", err)
	}

	owner, err := e.authz.IsOwner(ctx, e.user, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("IsOwner network: %v", err)
	}
	if owner {
		t.Fatal("subnet grant must not own the parent network")
	}

	e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.other.LocalUser.UUID},
		Resource:  model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Owner:     true,
	})
	if err := e.authz.Require(ctx, e.other, service.ActionCreate, model.GrantResourceSubnet, sibling.UUID); err != nil {
		t.Fatalf("network owner inherits subnet create: %v", err)
	}
	if err := e.authz.RequireOwner(ctx, e.other, model.GrantResourceSubnet, child.UUID); err != nil {
		t.Fatalf("network owner manages subnet grants: %v", err)
	}
}

func TestAuthorization_OIDCGroupAndMachinePool(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "federated")

	pool := &model.IdentityPool{
		UUID:        uuid.New(),
		Name:        "corp",
		Slug:        "corp",
		Issuer:      "https://idp.example.com",
		ClientID:    "kilhog",
		Scopes:      []string{"openid"},
		GroupsClaim: "groups",
		Enabled:     true,
	}
	if err := e.repos.IdentityPools.Create(ctx, pool); err != nil {
		t.Fatalf("create pool: %v", err)
	}
	e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{
			Kind:             model.GrantPrincipalOIDCGroup,
			IdentityPoolUUID: &pool.UUID,
			Group:            "netops",
		},
		Resource: model.GrantResource{Kind: model.GrantResourcePlatform, Capability: model.CapabilityCreateNetworks},
	})
	e.grant(t, e.admin, service.CreateGrantInput{
		Principal: model.GrantPrincipal{
			Kind:             model.GrantPrincipalOIDCGroup,
			IdentityPoolUUID: &pool.UUID,
			Group:            "netops",
		},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true},
	})

	oidc := &service.Principal{
		Kind:             model.PrincipalKindOIDC,
		IdentityPoolUUID: &pool.UUID,
		OIDCSubject:      "user-1",
		OIDCGroups:       []string{"netops"},
	}
	ok, err := e.authz.CanCreateNetworks(ctx, oidc)
	if err != nil || !ok {
		t.Fatalf("group create_networks = %v, %v", ok, err)
	}
	if err := e.authz.Require(ctx, oidc, service.ActionRead, model.GrantResourceNetwork, network.UUID); err != nil {
		t.Fatalf("group read: %v", err)
	}
	oidc.OIDCGroups = []string{"other"}
	ok, err = e.authz.CanCreateNetworks(ctx, oidc)
	if err != nil {
		t.Fatalf("CanCreateNetworks: %v", err)
	}
	if ok {
		t.Fatal("leaving the group should drop the grant")
	}

	machines := service.NewMachineIdentityService(e.repos.MachinePools, e.repos.MachineProviders, e.repos.Machines, e.repos.MachineAPIKeys, nil)
	mp, err := machines.CreatePool(ctx, service.CreateMachinePoolInput{Name: "ci", Slug: "ci"})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	machine, err := machines.CreateMachine(ctx, mp.UUID, service.CreateMachineInput{Name: "deploy"})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	e.grant(t, e.admin, service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalMachinePool, MachinePoolUUID: &mp.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true, Create: true},
	})
	machinePrincipal := &service.Principal{
		Kind:            model.PrincipalKindMachine,
		MachineUUID:     &machine.UUID,
		MachinePoolUUID: &mp.UUID,
	}
	if err := e.authz.Require(ctx, machinePrincipal, service.ActionCreate, model.GrantResourceNetwork, network.UUID); err != nil {
		t.Fatalf("machine pool grant: %v", err)
	}
}

func TestAuthorization_DisabledSubjectInert(t *testing.T) {
	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "inert")
	e.grant(t, e.admin, service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true},
	})

	disabled := *e.user.LocalUser
	disabled.Enabled = false
	inert := &service.Principal{Kind: model.PrincipalKindLocalUser, LocalUser: &disabled}
	ok, err := e.authz.Can(ctx, inert, service.ActionRead, model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("Can: %v", err)
	}
	if ok {
		t.Fatal("disabled user grants must be inert")
	}
}

func TestGrantService_RejectAdminSubject(t *testing.T) {
	e := openRBAC(t)
	network := e.createNetwork(t, "admin-target")
	_, err := e.grants.Create(t.Context(), e.admin, service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.admin.LocalUser.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true},
	})
	if !errors.Is(err, service.ErrGrantTargetAdmin) {
		t.Fatalf("Create = %v, want ErrGrantTargetAdmin", err)
	}
}

func TestGrantRepository_UniquePrincipalResource(t *testing.T) {
	e := openRBAC(t)
	network := e.createNetwork(t, "unique")
	input := service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true},
	}
	e.grant(t, e.admin, input)
	_, err := e.grants.Create(t.Context(), e.admin, input)
	if !errors.Is(err, service.ErrGrantConflict) {
		t.Fatalf("duplicate grant = %v, want ErrGrantConflict", err)
	}
}

func TestGrantService_EnsureOwnerIdempotentForPrivileged(t *testing.T) {
	e := openRBAC(t)
	network := e.createNetwork(t, "priv")
	if err := e.grants.EnsureOwner(t.Context(), e.admin, network.UUID); err != nil {
		t.Fatalf("EnsureOwner admin: %v", err)
	}
	if err := e.grants.EnsureOwner(t.Context(), e.apiKey, network.UUID); err != nil {
		t.Fatalf("EnsureOwner api key: %v", err)
	}
	list, err := e.grants.ListByResource(t.Context(), model.GrantResourceNetwork, network.UUID)
	if err != nil {
		t.Fatalf("ListByResource: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("privileged EnsureOwner should not insert rows, got %d", len(list))
	}
}

func TestAuthorization_LogsDeniedAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(previous) })

	ctx := context.Background()
	e := openRBAC(t)
	network := e.createNetwork(t, "logged")

	if err := e.authz.Require(ctx, e.user, service.ActionRead, model.GrantResourceNetwork, network.UUID); !errors.Is(err, service.ErrResourceNotVisible) {
		t.Fatalf("Require read = %v, want not visible", err)
	}
	hiddenLog := buf.String()
	for _, want := range []string{
		"rbac denied",
		"no applicable grant or structural visibility",
		"action=read",
		"resource_kind=network",
		network.UUID.String(),
		"principal_kind=local_user",
		"username=operator",
		e.user.LocalUser.UUID.String(),
	} {
		if !strings.Contains(hiddenLog, want) {
			t.Fatalf("not-visible log missing %q\n%s", want, hiddenLog)
		}
	}

	e.grant(t, e.admin, service.CreateGrantInput{
		Principal:   model.GrantPrincipal{Kind: model.GrantPrincipalLocalUser, LocalUserUUID: &e.user.LocalUser.UUID},
		Resource:    model.GrantResource{Kind: model.GrantResourceNetwork, UUID: &network.UUID},
		Permissions: model.Permissions{Read: true},
	})
	buf.Reset()
	if err := e.authz.Require(ctx, e.user, service.ActionDelete, model.GrantResourceNetwork, network.UUID); !errors.Is(err, service.ErrPermissionDenied) {
		t.Fatalf("Require delete = %v, want permission denied", err)
	}
	deniedLog := buf.String()
	for _, want := range []string{
		"rbac denied",
		"missing required permission",
		"action=delete",
		"effective_read=true",
		"effective_delete=false",
		"grant_subjects=local_user:" + e.user.LocalUser.UUID.String(),
	} {
		if !strings.Contains(deniedLog, want) {
			t.Fatalf("missing-permission log missing %q\n%s", want, deniedLog)
		}
	}

	buf.Reset()
	if err := e.authz.RequireOwner(ctx, e.user, model.GrantResourceNetwork, network.UUID); !errors.Is(err, service.ErrPermissionDenied) {
		t.Fatalf("RequireOwner = %v, want permission denied", err)
	}
	ownerLog := buf.String()
	if !strings.Contains(ownerLog, "not owner of resource") || !strings.Contains(ownerLog, "action=manage_grants") {
		t.Fatalf("owner log = %s", ownerLog)
	}

	buf.Reset()
	if err := e.authz.RequireCreateNetworks(ctx, e.user); !errors.Is(err, service.ErrPermissionDenied) {
		t.Fatalf("RequireCreateNetworks = %v, want permission denied", err)
	}
	createLog := buf.String()
	for _, want := range []string{
		"missing create_networks grant",
		"resource_kind=platform",
		"capability=create_networks",
		"username=operator",
	} {
		if !strings.Contains(createLog, want) {
			t.Fatalf("create_networks log missing %q\n%s", want, createLog)
		}
	}
}
