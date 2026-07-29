package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestNetworkService_CreateAndGet(t *testing.T) {
	svc := openNetworkService(t)

	ctx := context.Background()
	network, err := svc.Create(ctx, service.CreateNetworkInput{
		Name:        "lab",
		Description: "test network",
		Tags:        []model.Tag{{Key: "env", Value: "dev"}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if network.UUID == uuid.Nil {
		t.Fatal("expected generated uuid")
	}

	got, err := svc.GetByUUID(ctx, network.UUID)
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Name != "lab" {
		t.Fatalf("name = %q, want lab", got.Name)
	}
}

func TestNetworkService_CreateValidation(t *testing.T) {
	svc := openNetworkService(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		input   service.CreateNetworkInput
		wantErr error
	}{
		{
			name:    "empty name",
			input:   service.CreateNetworkInput{Name: "   "},
			wantErr: service.ErrInvalidNetworkName,
		},
		{
			name: "duplicate tag key",
			input: service.CreateNetworkInput{
				Name: "lab",
				Tags: []model.Tag{
					{Key: "env", Value: "dev"},
					{Key: "env", Value: "prod"},
				},
			},
			wantErr: service.ErrDuplicateTagKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNetworkService_UpdateAndList(t *testing.T) {
	svc := openNetworkService(t)
	ctx := context.Background()

	network, err := svc.Create(ctx, service.CreateNetworkInput{Name: "corp"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := svc.Update(ctx, network.UUID, service.UpdateNetworkInput{
		Name:        "corp-updated",
		Description: "updated description",
		Tags:        []model.Tag{{Key: "team", Value: "netops"}},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "corp-updated" {
		t.Fatalf("name = %q, want corp-updated", updated.Name)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
}

func TestNetworkService_Delete(t *testing.T) {
	svc, repos := openNetworkServiceWithRepos(t)
	ctx := context.Background()

	network, err := svc.Create(ctx, service.CreateNetworkInput{Name: "deletable"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Delete(ctx, network.UUID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := svc.GetByUUID(ctx, network.UUID); err != service.ErrNetworkNotFound {
		t.Fatalf("GetByUUID() after delete error = %v, want ErrNetworkNotFound", err)
	}

	networkWithChild, err := svc.Create(ctx, service.CreateNetworkInput{Name: "with-child"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	subnetID := uuid.New()
	if err := repos.Subnets.Create(ctx, &model.Subnet{
		UUID:    subnetID,
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Type:    model.AddressTypeIPv4,
		Parent: model.Parent{
			Kind: model.ParentKindNetwork,
			UUID: networkWithChild.UUID,
		},
	}); err != nil {
		t.Fatalf("create subnet: %v", err)
	}

	if err := svc.Delete(ctx, networkWithChild.UUID); !errors.Is(err, service.ErrNetworkHasChildren) {
		t.Fatalf("Delete() error = %v, want ErrNetworkHasChildren", err)
	}
}

func openNetworkService(t *testing.T) *service.NetworkService {
	t.Helper()
	svc, _ := openNetworkServiceWithRepos(t)
	return svc
}

func openNetworkServiceWithRepos(t *testing.T) (*service.NetworkService, *repository.Repositories) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "kilhog.db")
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

	return service.NewNetworkService(repos.Networks, repos.Subnets), repos
}
