package repository_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
)

func TestNetworkRepository_CRUD(t *testing.T) {
	repos := openTestRepositories(t)
	defer repos.Close()

	ctx := context.Background()
	networkID := uuid.New()

	network := &model.Network{
		UUID:        networkID,
		Name:        "lab",
		Description: "test network",
		Tags: []model.Tag{
			{Key: "env", Value: "dev"},
		},
	}

	if err := repos.Networks.Create(ctx, network); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repos.Networks.GetByUUID(ctx, networkID)
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.Name != network.Name {
		t.Fatalf("name = %q, want %q", got.Name, network.Name)
	}
	if len(got.Tags) != 1 || got.Tags[0].Key != "env" {
		t.Fatalf("tags = %#v, want env=dev", got.Tags)
	}

	byName, err := repos.Networks.GetByName(ctx, "lab")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if byName.UUID != networkID {
		t.Fatalf("uuid = %v, want %v", byName.UUID, networkID)
	}

	network.Description = "updated"
	network.Tags = []model.Tag{{Key: "team", Value: "netops"}}
	if err := repos.Networks.Update(ctx, network); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	list, err := repos.Networks.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}

	if err := repos.Networks.Delete(ctx, networkID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestSubnetRepository_CreateAndList(t *testing.T) {
	repos := openTestRepositories(t)
	defer repos.Close()

	ctx := context.Background()
	networkID := uuid.New()
	subnetID := uuid.New()

	if err := repos.Networks.Create(ctx, &model.Network{
		UUID: networkID,
		Name: "corp",
	}); err != nil {
		t.Fatalf("create network: %v", err)
	}

	subnet := &model.Subnet{
		UUID:    subnetID,
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Type:    model.AddressTypeIPv4,
		Parent: model.Parent{
			Kind: model.ParentKindNetwork,
			UUID: networkID,
		},
		Tags: []model.Tag{{Key: "zone", Value: "public"}},
	}

	if err := repos.Subnets.Create(ctx, subnet); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repos.Subnets.GetByUUID(ctx, subnetID)
	if err != nil {
		t.Fatalf("GetByUUID() error = %v", err)
	}
	if got.CIDR() != "10.0.0.0/24" {
		t.Fatalf("CIDR() = %q, want 10.0.0.0/24", got.CIDR())
	}

	byName, err := repos.Subnets.GetByName(ctx, networkID, "dmz")
	if err != nil {
		t.Fatalf("GetByName() error = %v", err)
	}
	if byName.UUID != subnetID {
		t.Fatalf("uuid = %v, want %v", byName.UUID, subnetID)
	}

	byNetwork, err := repos.Subnets.ListByNetwork(ctx, networkID)
	if err != nil {
		t.Fatalf("ListByNetwork() error = %v", err)
	}
	if len(byNetwork) != 1 {
		t.Fatalf("len(byNetwork) = %d, want 1", len(byNetwork))
	}

	byParent, err := repos.Subnets.ListByParent(ctx, model.Parent{Kind: model.ParentKindNetwork, UUID: networkID})
	if err != nil {
		t.Fatalf("ListByParent() error = %v", err)
	}
	if len(byParent) != 1 {
		t.Fatalf("len(byParent) = %d, want 1", len(byParent))
	}
}

func openTestRepositories(t *testing.T) *repository.Repositories {
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

	return repos
}
