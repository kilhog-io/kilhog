package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestSubnetService_NameTakenUserError(t *testing.T) {
	svc, repos := openSubnetServiceWithRepos(t)
	ctx := context.Background()

	networkSvc := service.NewNetworkService(repos.Networks, repos.Subnets)
	net, err := networkSvc.Create(ctx, service.CreateNetworkInput{Name: "lab"})
	if err != nil {
		t.Fatalf("Create network: %v", err)
	}

	input := service.CreateSubnetInput{
		Name:    "dmz",
		Prefix:  24,
		Address: "10.0.0.0",
		Parent:  model.Parent{Kind: model.ParentKindNetwork, UUID: net.UUID},
	}
	if _, err := svc.CreateInNetwork(ctx, net.UUID, input.Parent, input); err != nil {
		t.Fatalf("Create first subnet: %v", err)
	}

	_, err = svc.CreateInNetwork(ctx, net.UUID, input.Parent, input)
	if !errors.Is(err, service.ErrSubnetNameTaken) {
		t.Fatalf("Create() error = %v, want ErrSubnetNameTaken", err)
	}

	var userErr *service.UserError
	if !errors.As(err, &userErr) {
		t.Fatal("expected UserError wrapper")
	}
	if userErr.Message != `subnet name "dmz" is already used in this network` {
		t.Fatalf("message = %q", userErr.Message)
	}
}
