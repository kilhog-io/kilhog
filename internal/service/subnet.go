package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/iputil"
	"github.com/kilhog-io/kilhog/internal/model"
)

var (
	ErrSubnetNotFound       = errors.New("subnet not found")
	ErrSubnetNameTaken      = errors.New("subnet name already exists in network")
	ErrInvalidSubnetName    = errors.New("subnet name is required")
	ErrInvalidSubnetPrefix  = errors.New("invalid subnet prefix")
	ErrInvalidSubnetAddress = errors.New("invalid subnet address")
	ErrInvalidSubnetParent  = errors.New("subnet parent is required")
	ErrParentNotFound       = errors.New("subnet parent not found")
	ErrSubnetHasChildren    = errors.New("subnet has child subnets")
	ErrSubnetOverlap        = iputil.ErrOverlap
	ErrAddressOutsideParent = iputil.ErrOutsideParent
	ErrPrefixTooBroad       = iputil.ErrPrefixTooBroad
	ErrNoFreeAddress        = iputil.ErrNoFreeBlock
	ErrIPv6NotSupported     = errors.New("only ipv4 is supported")
	ErrAddressRequired      = errors.New("address is required when parent is a network")
	ErrSubnetNotInNetwork   = errors.New("subnet does not belong to network")
)

type SubnetService struct {
	subnets  SubnetRepository
	networks NetworkRepository
	metrics  ResourceMetrics
}

func NewSubnetService(subnets SubnetRepository, networks NetworkRepository, opts ...SubnetServiceOption) *SubnetService {
	s := &SubnetService{
		subnets:  subnets,
		networks: networks,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SubnetServiceOption configures optional SubnetService dependencies.
type SubnetServiceOption func(*SubnetService)

// WithSubnetMetrics attaches functional metrics to the subnet service.
func WithSubnetMetrics(m ResourceMetrics) SubnetServiceOption {
	return func(s *SubnetService) {
		s.metrics = m
	}
}

type CreateSubnetInput struct {
	Name        string
	Description string
	Prefix      int
	Address     string
	Type        model.AddressType
	Parent      model.Parent
}

type UpdateSubnetInput struct {
	Description string
}

type parentContext struct {
	networkUUID  uuid.UUID
	parentPrefix *netip.Prefix
}

func (s *SubnetService) Create(ctx context.Context, input CreateSubnetInput) (*model.Subnet, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrInvalidSubnetName
	}

	if input.Parent.Kind == "" || input.Parent.UUID == uuid.Nil {
		return nil, ErrInvalidSubnetParent
	}

	addrType := input.Type
	if addrType == "" {
		addrType = model.AddressTypeIPv4
	}
	if addrType != model.AddressTypeIPv4 {
		return nil, ErrIPv6NotSupported
	}

	parentCtx, err := s.resolveParentContext(ctx, input.Parent)
	if err != nil {
		return nil, err
	}

	address := strings.TrimSpace(input.Address)
	description := strings.TrimSpace(input.Description)

	subnet, err := s.subnets.CreateAtomically(ctx, input.Parent, func(tx SubnetCreateTx) (*model.Subnet, error) {
		if existing, err := tx.GetByName(ctx, parentCtx.networkUUID, name); err == nil && existing != nil {
			return nil, userError(ErrSubnetNameTaken, `subnet name %q is already used in this network`, name)
		} else if err != nil && !errors.Is(err, ErrSubnetNotFound) {
			return nil, fmt.Errorf("check subnet name: %w", err)
		}

		siblings, err := tx.ListSiblings(ctx)
		if err != nil {
			return nil, fmt.Errorf("list sibling subnets: %w", err)
		}

		siblingPrefixes, err := subnetsToPrefixes(siblings)
		if err != nil {
			return nil, err
		}

		var candidate netip.Prefix
		switch {
		case address != "":
			candidate, err = iputil.ParseIPv4Prefix(address, input.Prefix)
			if err != nil {
				switch {
				case errors.Is(err, iputil.ErrInvalidIPv4Address):
					return nil, userError(ErrInvalidSubnetAddress, `invalid IPv4 address %q`, address)
				case errors.Is(err, iputil.ErrInvalidPrefix):
					return nil, userError(ErrInvalidSubnetPrefix, `invalid IPv4 prefix length %d (must be between 1 and 32)`, input.Prefix)
				default:
					return nil, fmt.Errorf("parse subnet address: %w", err)
				}
			}
		case input.Parent.Kind == model.ParentKindNetwork:
			return nil, userError(ErrAddressRequired, "address is required when the parent is a network")
		case parentCtx.parentPrefix == nil:
			return nil, fmt.Errorf("missing parent cidr for auto address allocation")
		default:
			candidate, err = iputil.FindFreeIPv4Block(*parentCtx.parentPrefix, input.Prefix, siblingPrefixes)
			if err != nil {
				switch {
				case errors.Is(err, iputil.ErrInvalidPrefix):
					return nil, userError(ErrInvalidSubnetPrefix, `invalid IPv4 prefix length %d (must be between 1 and 32)`, input.Prefix)
				case errors.Is(err, iputil.ErrPrefixTooBroad):
					return nil, userError(ErrPrefixTooBroad, `prefix /%d is less specific than parent prefix /%d`, input.Prefix, parentCtx.parentPrefix.Bits())
				case errors.Is(err, iputil.ErrNoFreeBlock):
					return nil, userError(ErrNoFreeAddress, `no free /%d block found in parent CIDR %s`, input.Prefix, parentCtx.parentPrefix.String())
				default:
					return nil, fmt.Errorf("find free address: %w", err)
				}
			}
		}

		if err := iputil.ValidateIPv4Subnet(candidate, parentCtx.parentPrefix, siblingPrefixes); err != nil {
			cidr := iputil.PrefixString(candidate)
			switch {
			case errors.Is(err, iputil.ErrOverlap):
				return nil, userError(ErrSubnetOverlap, `subnet %s overlaps with an existing sibling under the same parent`, cidr)
			case errors.Is(err, iputil.ErrOutsideParent):
				return nil, userError(ErrAddressOutsideParent, `subnet %s is outside parent CIDR %s`, cidr, parentCtx.parentPrefix.String())
			case errors.Is(err, iputil.ErrPrefixTooBroad):
				if parentCtx.parentPrefix != nil {
					return nil, userError(ErrPrefixTooBroad, `prefix /%d is less specific than parent prefix /%d`, candidate.Bits(), parentCtx.parentPrefix.Bits())
				}
				return nil, ErrPrefixTooBroad
			case errors.Is(err, iputil.ErrInvalidPrefix):
				return nil, userError(ErrInvalidSubnetPrefix, `invalid IPv4 prefix length %d (must be between 1 and 32)`, candidate.Bits())
			default:
				return nil, fmt.Errorf("validate subnet: %w", err)
			}
		}

		subnet := &model.Subnet{
			UUID:        uuid.New(),
			Name:        name,
			Description: description,
			Prefix:      candidate.Bits(),
			Address:     iputil.PrefixAddress(candidate),
			Type:        model.AddressTypeIPv4,
			Parent:      input.Parent,
		}

		if err := tx.Insert(ctx, subnet); err != nil {
			return nil, fmt.Errorf("create subnet: %w", err)
		}

		return subnet, nil
	})
	if err != nil {
		return nil, err
	}

	if s.metrics != nil {
		s.metrics.SubnetCreated(ctx)
	}

	return subnet, nil
}

func (s *SubnetService) ListByNetwork(ctx context.Context, networkUUID uuid.UUID) ([]*model.Subnet, error) {
	if _, err := s.networks.GetByUUID(ctx, networkUUID); err != nil {
		if errors.Is(err, ErrNetworkNotFound) {
			return nil, ErrNetworkNotFound
		}
		return nil, fmt.Errorf("get network: %w", err)
	}

	subnets, err := s.subnets.ListByNetwork(ctx, networkUUID)
	if err != nil {
		return nil, fmt.Errorf("list subnets: %w", err)
	}
	return subnets, nil
}

func (s *SubnetService) ListChildren(ctx context.Context, networkUUID, parentSubnetUUID uuid.UUID) ([]*model.Subnet, error) {
	if _, err := s.ensureInNetwork(ctx, networkUUID, parentSubnetUUID); err != nil {
		return nil, err
	}

	children, err := s.subnets.ListByParent(ctx, model.Parent{
		Kind: model.ParentKindSubnet,
		UUID: parentSubnetUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("list child subnets: %w", err)
	}
	return children, nil
}

func (s *SubnetService) CreateInNetwork(ctx context.Context, networkUUID uuid.UUID, parent model.Parent, input CreateSubnetInput) (*model.Subnet, error) {
	switch parent.Kind {
	case model.ParentKindNetwork:
		if parent.UUID != networkUUID {
			return nil, ErrSubnetNotInNetwork
		}
	case model.ParentKindSubnet:
		if _, err := s.ensureInNetwork(ctx, networkUUID, parent.UUID); err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidSubnetParent
	}

	input.Parent = parent
	return s.Create(ctx, input)
}

func (s *SubnetService) GetInNetwork(ctx context.Context, networkUUID, subnetUUID uuid.UUID) (*model.Subnet, error) {
	return s.ensureInNetwork(ctx, networkUUID, subnetUUID)
}

func (s *SubnetService) UpdateInNetwork(ctx context.Context, networkUUID, subnetUUID uuid.UUID, input UpdateSubnetInput) (*model.Subnet, error) {
	if _, err := s.ensureInNetwork(ctx, networkUUID, subnetUUID); err != nil {
		return nil, err
	}
	return s.Update(ctx, subnetUUID, input)
}

func (s *SubnetService) DeleteInNetwork(ctx context.Context, networkUUID, subnetUUID uuid.UUID) error {
	if _, err := s.ensureInNetwork(ctx, networkUUID, subnetUUID); err != nil {
		return err
	}
	return s.Delete(ctx, subnetUUID)
}

func (s *SubnetService) ensureInNetwork(ctx context.Context, networkUUID, subnetUUID uuid.UUID) (*model.Subnet, error) {
	subnet, err := s.subnets.GetByUUID(ctx, subnetUUID)
	if err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return nil, ErrSubnetNotFound
		}
		return nil, fmt.Errorf("get subnet: %w", err)
	}

	actualNetworkUUID, err := s.resolveNetworkUUID(ctx, subnet)
	if err != nil {
		return nil, err
	}
	if actualNetworkUUID != networkUUID {
		return nil, ErrSubnetNotInNetwork
	}

	return subnet, nil
}

func (s *SubnetService) GetByUUID(ctx context.Context, id uuid.UUID) (*model.Subnet, error) {
	subnet, err := s.subnets.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return nil, ErrSubnetNotFound
		}
		return nil, fmt.Errorf("get subnet: %w", err)
	}
	return subnet, nil
}

func (s *SubnetService) Update(ctx context.Context, id uuid.UUID, input UpdateSubnetInput) (*model.Subnet, error) {
	subnet, err := s.subnets.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return nil, ErrSubnetNotFound
		}
		return nil, fmt.Errorf("get subnet: %w", err)
	}

	subnet.Description = strings.TrimSpace(input.Description)

	if err := s.subnets.Update(ctx, subnet); err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return nil, ErrSubnetNotFound
		}
		return nil, fmt.Errorf("update subnet: %w", err)
	}

	if s.metrics != nil {
		s.metrics.SubnetUpdated(ctx)
	}

	return subnet, nil
}

func (s *SubnetService) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.subnets.GetByUUID(ctx, id); err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return ErrSubnetNotFound
		}
		return fmt.Errorf("get subnet: %w", err)
	}

	children, err := s.subnets.ListByParent(ctx, model.Parent{
		Kind: model.ParentKindSubnet,
		UUID: id,
	})
	if err != nil {
		return fmt.Errorf("check subnet children: %w", err)
	}
	if len(children) > 0 {
		return userError(ErrSubnetHasChildren, "subnet has %d child subnet(s) and cannot be deleted", len(children))
	}

	if err := s.subnets.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return ErrSubnetNotFound
		}
		return fmt.Errorf("delete subnet: %w", err)
	}

	if s.metrics != nil {
		s.metrics.SubnetDeleted(ctx)
	}

	return nil
}

func (s *SubnetService) resolveParentContext(ctx context.Context, parent model.Parent) (*parentContext, error) {
	switch parent.Kind {
	case model.ParentKindNetwork:
		if _, err := s.networks.GetByUUID(ctx, parent.UUID); err != nil {
			if errors.Is(err, ErrNetworkNotFound) {
				return nil, ErrParentNotFound
			}
			return nil, fmt.Errorf("get parent network: %w", err)
		}
		return &parentContext{
			networkUUID:  parent.UUID,
			parentPrefix: nil,
		}, nil
	case model.ParentKindSubnet:
		parentSubnet, err := s.subnets.GetByUUID(ctx, parent.UUID)
		if err != nil {
			if errors.Is(err, ErrSubnetNotFound) {
				return nil, ErrParentNotFound
			}
			return nil, fmt.Errorf("get parent subnet: %w", err)
		}
		if parentSubnet.Type != model.AddressTypeIPv4 {
			return nil, ErrIPv6NotSupported
		}

		parentPrefix, err := iputil.ParseIPv4Prefix(parentSubnet.Address, parentSubnet.Prefix)
		if err != nil {
			return nil, fmt.Errorf("parse parent cidr: %w", err)
		}

		networkUUID, err := s.resolveNetworkUUID(ctx, parentSubnet)
		if err != nil {
			return nil, err
		}

		return &parentContext{
			networkUUID:  networkUUID,
			parentPrefix: &parentPrefix,
		}, nil
	default:
		return nil, ErrInvalidSubnetParent
	}
}

func (s *SubnetService) resolveNetworkUUID(ctx context.Context, subnet *model.Subnet) (uuid.UUID, error) {
	if subnet.Parent.Kind == model.ParentKindNetwork {
		return subnet.Parent.UUID, nil
	}

	parentSubnet, err := s.subnets.GetByUUID(ctx, subnet.Parent.UUID)
	if err != nil {
		if errors.Is(err, ErrSubnetNotFound) {
			return uuid.Nil, ErrParentNotFound
		}
		return uuid.Nil, fmt.Errorf("resolve network from parent subnet: %w", err)
	}

	return s.resolveNetworkUUID(ctx, parentSubnet)
}

func subnetsToPrefixes(subnets []*model.Subnet) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(subnets))
	for _, subnet := range subnets {
		if subnet.Type != model.AddressTypeIPv4 {
			continue
		}
		prefix, err := iputil.ParseIPv4Prefix(subnet.Address, subnet.Prefix)
		if err != nil {
			return nil, fmt.Errorf("parse sibling cidr: %w", err)
		}
		prefixes = append(prefixes, prefix)
	}

	return prefixes, nil
}
