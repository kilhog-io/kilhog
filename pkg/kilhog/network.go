package kilhog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListNetworks returns all networks via GET /networks.
func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	var networks []Network
	if err := c.do(ctx, httpMethodGet, "/networks", nil, &networks); err != nil {
		return nil, err
	}
	if networks == nil {
		return []Network{}, nil
	}
	return networks, nil
}

// GetNetwork returns a network by UUID via GET /networks/{uuid}.
func (c *Client) GetNetwork(ctx context.Context, id uuid.UUID) (*Network, error) {
	var network Network
	path := fmt.Sprintf("/networks/%s", id)
	if err := c.do(ctx, httpMethodGet, path, nil, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

// CreateNetwork creates a network via POST /networks.
func (c *Client) CreateNetwork(ctx context.Context, input CreateNetworkInput) (*Network, error) {
	var network Network
	if err := c.do(ctx, httpMethodPost, "/networks", input, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

// UpdateNetwork updates a network via PUT /networks/{uuid}.
func (c *Client) UpdateNetwork(ctx context.Context, id uuid.UUID, input UpdateNetworkInput) (*Network, error) {
	var network Network
	path := fmt.Sprintf("/networks/%s", id)
	if err := c.do(ctx, httpMethodPut, path, input, &network); err != nil {
		return nil, err
	}
	return &network, nil
}

// DeleteNetwork deletes a network via DELETE /networks/{uuid}.
func (c *Client) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	path := fmt.Sprintf("/networks/%s", id)
	return c.do(ctx, httpMethodDelete, path, nil, nil)
}
