package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

var (
	ErrNetworkNotFound    = errors.New("network not found")
	ErrNetworkHasChildren = errors.New("network has child subnets")
	ErrNetworkNameTaken   = errors.New("network name already exists")
	ErrInvalidNetworkName = errors.New("network name is required")
	ErrDuplicateTagKey    = errors.New("duplicate tag key")
)

type NetworkService struct {
	networks NetworkRepository
	subnets  SubnetRepository
}

func NewNetworkService(networks NetworkRepository, subnets SubnetRepository) *NetworkService {
	return &NetworkService{
		networks: networks,
		subnets:  subnets,
	}
}

type CreateNetworkInput struct {
	Name        string
	Description string
	Tags        []model.Tag
}

type UpdateNetworkInput struct {
	Name        string
	Description string
	Tags        []model.Tag
}

func (s *NetworkService) Create(ctx context.Context, input CreateNetworkInput) (*model.Network, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidNetworkName
	}
	if err := validateTags(input.Tags); err != nil {
		return nil, err
	}

	if existing, err := s.networks.GetByName(ctx, name); err == nil && existing != nil {
		return nil, ErrNetworkNameTaken
	} else if err != nil && !errors.Is(err, ErrNetworkNotFound) {
		return nil, fmt.Errorf("check network name: %w", err)
	}

	network := &model.Network{
		UUID:        uuid.New(),
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Tags:        input.Tags,
	}

	if err := s.networks.Create(ctx, network); err != nil {
		return nil, fmt.Errorf("create network: %w", err)
	}

	return network, nil
}

func (s *NetworkService) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Network, error) {
	network, err := s.networks.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return nil, ErrNetworkNotFound
		}
		return nil, fmt.Errorf("get network: %w", err)
	}
	return network, nil
}

func (s *NetworkService) List(ctx context.Context) ([]*model.Network, error) {
	networks, err := s.networks.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return networks, nil
}

func (s *NetworkService) Update(ctx context.Context, id uuid.UUID, input UpdateNetworkInput) (*model.Network, error) {
	network, err := s.networks.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return nil, ErrNetworkNotFound
		}
		return nil, fmt.Errorf("get network: %w", err)
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidNetworkName
	}
	if err := validateTags(input.Tags); err != nil {
		return nil, err
	}

	if name != network.Name {
		if existing, err := s.networks.GetByName(ctx, name); err == nil && existing != nil && existing.UUID != id {
			return nil, ErrNetworkNameTaken
		} else if err != nil && !errors.Is(err, ErrNetworkNotFound) {
			return nil, fmt.Errorf("check network name: %w", err)
		}
	}

	network.Name = name
	network.Description = strings.TrimSpace(input.Description)
	network.Tags = input.Tags

	if err := s.networks.Update(ctx, network); err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return nil, ErrNetworkNotFound
		}
		return nil, fmt.Errorf("update network: %w", err)
	}

	return network, nil
}

func (s *NetworkService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.networks.GetByUUID(ctx, id); err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return ErrNetworkNotFound
		}
		return fmt.Errorf("get network: %w", err)
	}

	children, err := s.subnets.ListByParent(ctx, model.Parent{
		Kind: model.ParentKindNetwork,
		UUID: id,
	})
	if err != nil {
		return fmt.Errorf("check network children: %w", err)
	}
	if len(children) > 0 {
		return ErrNetworkHasChildren
	}

	if err := s.networks.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return ErrNetworkNotFound
		}
		return fmt.Errorf("delete network: %w", err)
	}

	return nil
}

func validateTags(tags []model.Tag) error {
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		key := strings.TrimSpace(tag.Key)
		if key == "" {
			return fmt.Errorf("tag key is required")
		}
		if _, ok := seen[key]; ok {
			return ErrDuplicateTagKey
		}
		seen[key] = struct{}{}
	}
	return nil
}
