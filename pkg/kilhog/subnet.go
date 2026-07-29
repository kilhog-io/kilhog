package kilhog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func networkSubnetsPath(networkID uuid.UUID) string {
	return fmt.Sprintf("/networks/%s/subnets", networkID)
}

func networkSubnetPath(networkID, subnetID uuid.UUID) string {
	return fmt.Sprintf("/networks/%s/subnets/%s", networkID, subnetID)
}

func childSubnetsPath(networkID, parentSubnetID uuid.UUID) string {
	return fmt.Sprintf("/networks/%s/subnets/%s/subnets", networkID, parentSubnetID)
}

// ListSubnets returns all subnets in a network via GET /networks/{uuid}/subnets.
func (c *Client) ListSubnets(ctx context.Context, networkID uuid.UUID) ([]Subnet, error) {
	var subnets []Subnet
	if err := c.do(ctx, httpMethodGet, networkSubnetsPath(networkID), nil, &subnets); err != nil {
		return nil, err
	}
	if subnets == nil {
		return []Subnet{}, nil
	}
	return subnets, nil
}

// GetSubnet returns a subnet by UUID via GET /networks/{uuid}/subnets/{subnet_uuid}.
func (c *Client) GetSubnet(ctx context.Context, networkID, subnetID uuid.UUID) (*Subnet, error) {
	var subnet Subnet
	if err := c.do(ctx, httpMethodGet, networkSubnetPath(networkID, subnetID), nil, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

// CreateSubnetInNetwork creates a direct child subnet via POST /networks/{uuid}/subnets.
func (c *Client) CreateSubnetInNetwork(ctx context.Context, networkID uuid.UUID, input CreateSubnetInput) (*Subnet, error) {
	var subnet Subnet
	if err := c.do(ctx, httpMethodPost, networkSubnetsPath(networkID), input, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

// CreateSubnetUnderParent creates a child subnet via POST /networks/{uuid}/subnets/{subnet_uuid}/subnets.
func (c *Client) CreateSubnetUnderParent(ctx context.Context, networkID, parentSubnetID uuid.UUID, input CreateSubnetInput) (*Subnet, error) {
	var subnet Subnet
	if err := c.do(ctx, httpMethodPost, childSubnetsPath(networkID, parentSubnetID), input, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

// ListChildSubnets returns direct children of a subnet via GET .../subnets/{subnet_uuid}/subnets.
func (c *Client) ListChildSubnets(ctx context.Context, networkID, parentSubnetID uuid.UUID) ([]Subnet, error) {
	var subnets []Subnet
	if err := c.do(ctx, httpMethodGet, childSubnetsPath(networkID, parentSubnetID), nil, &subnets); err != nil {
		return nil, err
	}
	if subnets == nil {
		return []Subnet{}, nil
	}
	return subnets, nil
}

// UpdateSubnet updates a subnet description via PUT /networks/{uuid}/subnets/{subnet_uuid}.
func (c *Client) UpdateSubnet(ctx context.Context, networkID, subnetID uuid.UUID, input UpdateSubnetInput) (*Subnet, error) {
	var subnet Subnet
	if err := c.do(ctx, httpMethodPut, networkSubnetPath(networkID, subnetID), input, &subnet); err != nil {
		return nil, err
	}
	return &subnet, nil
}

// DeleteSubnet deletes a subnet via DELETE /networks/{uuid}/subnets/{subnet_uuid}.
func (c *Client) DeleteSubnet(ctx context.Context, networkID, subnetID uuid.UUID) error {
	return c.do(ctx, httpMethodDelete, networkSubnetPath(networkID, subnetID), nil, nil)
}
