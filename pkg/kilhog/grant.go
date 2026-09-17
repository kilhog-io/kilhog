package kilhog

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// ListMyGrants returns grants that apply to the current principal via GET /auth/me/grants.
func (c *Client) ListMyGrants(ctx context.Context) ([]Grant, error) {
	return c.listGrants(ctx, "/auth/me/grants")
}

// ListPlatformGrants returns platform grants via GET /auth/platform-grants.
func (c *Client) ListPlatformGrants(ctx context.Context) ([]Grant, error) {
	return c.listGrants(ctx, "/auth/platform-grants")
}

// CreatePlatformGrant creates a platform grant via POST /auth/platform-grants.
func (c *Client) CreatePlatformGrant(ctx context.Context, input CreatePlatformGrantInput) (*Grant, error) {
	var grant Grant
	if err := c.do(ctx, httpMethodPost, "/auth/platform-grants", input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// UpdatePlatformGrant updates a platform grant via PUT /auth/platform-grants/{grant_uuid}.
func (c *Client) UpdatePlatformGrant(ctx context.Context, grantUUID uuid.UUID, input UpdateGrantInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/auth/platform-grants/%s", grantUUID)
	if err := c.do(ctx, httpMethodPut, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// DeletePlatformGrant deletes a platform grant via DELETE /auth/platform-grants/{grant_uuid}.
func (c *Client) DeletePlatformGrant(ctx context.Context, grantUUID uuid.UUID) error {
	path := fmt.Sprintf("/auth/platform-grants/%s", grantUUID)
	return c.do(ctx, httpMethodDelete, path, nil, nil)
}

// ListNetworkGrants returns grants on a network via GET /networks/{uuid}/grants.
func (c *Client) ListNetworkGrants(ctx context.Context, networkUUID uuid.UUID) ([]Grant, error) {
	return c.listGrants(ctx, fmt.Sprintf("/networks/%s/grants", networkUUID))
}

// CreateNetworkGrant creates a grant on a network via POST /networks/{uuid}/grants.
func (c *Client) CreateNetworkGrant(ctx context.Context, networkUUID uuid.UUID, input CreateResourceGrantInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/grants", networkUUID)
	if err := c.do(ctx, httpMethodPost, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// UpdateNetworkGrant updates a network grant via PUT /networks/{uuid}/grants/{grant_uuid}.
func (c *Client) UpdateNetworkGrant(ctx context.Context, networkUUID, grantUUID uuid.UUID, input UpdateGrantInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/grants/%s", networkUUID, grantUUID)
	if err := c.do(ctx, httpMethodPut, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// DeleteNetworkGrant deletes a network grant via DELETE /networks/{uuid}/grants/{grant_uuid}.
func (c *Client) DeleteNetworkGrant(ctx context.Context, networkUUID, grantUUID uuid.UUID) error {
	path := fmt.Sprintf("/networks/%s/grants/%s", networkUUID, grantUUID)
	return c.do(ctx, httpMethodDelete, path, nil, nil)
}

// TransferNetworkOwnership transfers network ownership via POST /networks/{uuid}/ownership/transfer.
func (c *Client) TransferNetworkOwnership(ctx context.Context, networkUUID uuid.UUID, input TransferOwnershipInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/ownership/transfer", networkUUID)
	if err := c.do(ctx, httpMethodPost, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// ListSubnetGrants returns grants on a subnet via GET /networks/{uuid}/subnets/{subnet_uuid}/grants.
func (c *Client) ListSubnetGrants(ctx context.Context, networkUUID, subnetUUID uuid.UUID) ([]Grant, error) {
	return c.listGrants(ctx, fmt.Sprintf("/networks/%s/subnets/%s/grants", networkUUID, subnetUUID))
}

// CreateSubnetGrant creates a grant on a subnet via POST /networks/{uuid}/subnets/{subnet_uuid}/grants.
func (c *Client) CreateSubnetGrant(ctx context.Context, networkUUID, subnetUUID uuid.UUID, input CreateResourceGrantInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/subnets/%s/grants", networkUUID, subnetUUID)
	if err := c.do(ctx, httpMethodPost, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// UpdateSubnetGrant updates a subnet grant via PUT /networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}.
func (c *Client) UpdateSubnetGrant(ctx context.Context, networkUUID, subnetUUID, grantUUID uuid.UUID, input UpdateGrantInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/subnets/%s/grants/%s", networkUUID, subnetUUID, grantUUID)
	if err := c.do(ctx, httpMethodPut, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

// DeleteSubnetGrant deletes a subnet grant via DELETE /networks/{uuid}/subnets/{subnet_uuid}/grants/{grant_uuid}.
func (c *Client) DeleteSubnetGrant(ctx context.Context, networkUUID, subnetUUID, grantUUID uuid.UUID) error {
	path := fmt.Sprintf("/networks/%s/subnets/%s/grants/%s", networkUUID, subnetUUID, grantUUID)
	return c.do(ctx, httpMethodDelete, path, nil, nil)
}

// TransferSubnetOwnership transfers subnet ownership via POST .../ownership/transfer.
func (c *Client) TransferSubnetOwnership(ctx context.Context, networkUUID, subnetUUID uuid.UUID, input TransferOwnershipInput) (*Grant, error) {
	var grant Grant
	path := fmt.Sprintf("/networks/%s/subnets/%s/ownership/transfer", networkUUID, subnetUUID)
	if err := c.do(ctx, httpMethodPost, path, input, &grant); err != nil {
		return nil, err
	}
	return &grant, nil
}

func (c *Client) listGrants(ctx context.Context, path string) ([]Grant, error) {
	var grants []Grant
	if err := c.do(ctx, httpMethodGet, path, nil, &grants); err != nil {
		return nil, err
	}
	if grants == nil {
		return []Grant{}, nil
	}
	return grants, nil
}
