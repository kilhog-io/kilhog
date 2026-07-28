package service_test

import (
	"context"
	"testing"

	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestSubnetService_CreateWithAddress(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	net, err := service.NewNetworkService(repos.Networks, repos.Subnets).Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	subnet, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Type:    model.AddressTypeIPv4,
		Parent: model.Parent{
			Kind: model.ParentKindNetwork,
			UUID: net.UUID,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if subnet.CIDR() != "10.0.0.0/24" {
		t.Fatalf("CIDR() = %q, want 10.0.0.0/24", subnet.CIDR())
	}

	got, err := svc.GetByUUID(ctx, subnet.UUID)
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Name != "dmz" {
		t.Fatalf("name = %q, want dmz", got.Name)
	}
}

func TestSubnetService_CreateAutoAddress(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	net, err := service.NewNetworkService(repos.Networks, repos.Subnets).Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "parent",
		Prefix:  24,
		Address: "10.0.0.0",
		Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
	})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	first, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:   "first",
		Prefix: 25,
		Parent: model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID},
	})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if first.Address != "10.0.0.0" {
		t.Fatalf("first address = %q, want 10.0.0.0", first.Address)
	}

	second, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:   "second",
		Prefix: 25,
		Parent: model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID},
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if second.Address != "10.0.0.128" {
		t.Fatalf("second address = %q, want 10.0.0.128", second.Address)
	}
}

func TestSubnetService_CreateNestedSubnet(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	net, err := service.NewNetworkService(repos.Networks, repos.Subnets).Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "parent",
		Prefix:  16,
		Address: "192.168.0.0",
		Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
	})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	child, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "child",
		Prefix:  24,
		Address: "192.168.1.0",
		Parent:  model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID},
	})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}
	if child.CIDR() != "192.168.1.0/24" {
		t.Fatalf("child CIDR = %q, want 192.168.1.0/24", child.CIDR())
	}
}

func TestSubnetService_CreateValidation(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	net, err := service.NewNetworkService(repos.Networks, repos.Subnets).Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	parent, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "parent",
		Prefix:  24,
		Address: "192.168.0.0",
		Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
	})
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}

	tests := []struct {
		name    string
		input   service.CreateSubnetInput
		wantErr error
	}{
		{
			name: "empty name",
			input: service.CreateSubnetInput{
				Name:   "  ",
				Prefix: 24,
				Parent: model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
			},
			wantErr: service.ErrInvalidSubnetName,
		},
		{
			name: "missing parent",
			input: service.CreateSubnetInput{
				Name:   "orphan",
				Prefix: 24,
			},
			wantErr: service.ErrInvalidSubnetParent,
		},
		{
			name: "missing address with network parent",
			input: service.CreateSubnetInput{
				Name:   "no-address",
				Prefix: 24,
				Parent: model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
			},
			wantErr: service.ErrAddressRequired,
		},
		{
			name: "overlap sibling",
			input: service.CreateSubnetInput{
				Name:    "overlap",
				Prefix:  24,
				Address: "192.168.0.0",
				Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
			},
			wantErr: service.ErrSubnetOverlap,
		},
		{
			name: "outside parent",
			input: service.CreateSubnetInput{
				Name:    "outside",
				Prefix:  24,
				Address: "10.0.0.0",
				Parent:  model.Parent{Kind: model.ParentKindSubnet, UUID: parent.UUID},
			},
			wantErr: service.ErrAddressOutsideParent,
		},
		{
			name: "ipv6 not supported",
			input: service.CreateSubnetInput{
				Name:   "v6",
				Prefix: 64,
				Type:   model.AddressTypeIPv6,
				Parent: model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
			},
			wantErr: service.ErrIPv6NotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.input)
			if err != tt.wantErr {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubnetService_NetworkScoping(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	netA, err := networkSvc.Create(ctx, service.CreateNetworkInput{Name: "a"})
	if err != nil {
		t.Fatalf("Create network a: %v", err)
	}
	netB, err := networkSvc.Create(ctx, service.CreateNetworkInput{Name: "b"})
	if err != nil {
		t.Fatalf("Create network b: %v", err)
	}

	subnet, err := svc.CreateInNetwork(ctx, netA.UUID, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: netA.UUID,
	}, service.CreateSubnetInput{
		Name: "dmz", Prefix: 24, Address: "10.0.0.0",
	})
	if err != nil {
		t.Fatalf("CreateInNetwork() error = %v", err)
	}

	list, err := svc.ListByNetwork(ctx, netA.UUID)
	if err != nil {
		t.Fatalf("ListByNetwork() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	if _, err := svc.GetInNetwork(ctx, netB.UUID, subnet.UUID); err != service.ErrSubnetNotInNetwork {
		t.Fatalf("GetInNetwork() error = %v, want ErrSubnetNotInNetwork", err)
	}
}

func TestSubnetService_UpdateAndDelete(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	net, err := service.NewNetworkService(repos.Networks, repos.Subnets).Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	subnet, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
	})
	if err != nil {
		t.Fatalf("Create subnet: %v", err)
	}

	updated, err := svc.Update(ctx, subnet.UUID, service.UpdateSubnetInput{Description: "updated"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Description != "updated" {
		t.Fatalf("description = %q, want updated", updated.Description)
	}
	if updated.Name != "dmz" {
		t.Fatalf("name changed to %q, want immutable dmz", updated.Name)
	}

	child, err := svc.Create(ctx, service.CreateSubnetInput{
		Name:    "child",
		Prefix:  25,
		Address: "10.0.0.0",
		Parent:  model.Parent{Kind: model.ParentKindSubnet, UUID: subnet.UUID},
	})
	if err != nil {
		t.Fatalf("Create child: %v", err)
	}
	_ = child

	if err := svc.Delete(ctx, subnet.UUID); err != service.ErrSubnetHasChildren {
		t.Fatalf("Delete() error = %v, want ErrSubnetHasChildren", err)
	}

	if err := svc.Delete(ctx, child.UUID); err != nil {
		t.Fatalf("Delete child: %v", err)
	}
	if err := svc.Delete(ctx, subnet.UUID); err != nil {
		t.Fatalf("Delete parent: %v", err)
	}
}

func openSubnetServiceWithRepos(t *testing.T) (*service.SubnetService, *repository.Repositories) {
	t.Helper()

	dbPath := t.TempDir() + "/kilhog.db"
	cfg := db.Config{
		Driver:      db.DialectSQLite,
		DSN:         "file:" + dbPath + "?_pragma=foreign_keys(ON)",
		AutoMigrate: true,
	}

	repos, err := repository.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = repos.Close() })

	return service.NewSubnetService(repos.Subnets, repos.Networks), repos
}
