package kilhog

import "github.com/google/uuid"

// Tag is a key-value metadata pair attached to a network or subnet.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AddressType is the address family of a subnet.
type AddressType string

const (
	AddressTypeIPv4 AddressType = "ipv4"
	AddressTypeIPv6 AddressType = "ipv6"
)

// ParentKind identifies whether a subnet parent is a network or another subnet.
type ParentKind string

const (
	ParentKindNetwork ParentKind = "network"
	ParentKindSubnet  ParentKind = "subnet"
)

// Parent is a reference to the parent of a subnet in the hierarchy.
type Parent struct {
	Kind ParentKind `json:"kind"`
	UUID uuid.UUID  `json:"uuid"`
}

// Network is the root tenancy container for subnets.
type Network struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Tags        []Tag     `json:"tags,omitempty"`
}

// Subnet represents an IP address space (CIDR block or host address).
type Subnet struct {
	UUID        uuid.UUID   `json:"uuid"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Prefix      int         `json:"prefix"`
	Address     string      `json:"address"`
	Type        AddressType `json:"type"`
	Parent      Parent      `json:"parent"`
	Tags        []Tag       `json:"tags,omitempty"`
}

// CreateNetworkInput is the request body for POST /networks.
type CreateNetworkInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Tags        []Tag  `json:"tags,omitempty"`
}

// UpdateNetworkInput is the request body for PUT /networks/{uuid}.
type UpdateNetworkInput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Tags        []Tag  `json:"tags,omitempty"`
}

// CreateSubnetInput is the request body for POST .../subnets.
type CreateSubnetInput struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Prefix      int         `json:"prefix"`
	Address     string      `json:"address,omitempty"`
	Type        AddressType `json:"type,omitempty"`
	Tags        []Tag       `json:"tags,omitempty"`
}

// UpdateSubnetInput is the request body for PUT .../subnets/{subnet_uuid}.
type UpdateSubnetInput struct {
	Description string `json:"description,omitempty"`
}

// HealthStatus is returned by GET /healthz.
type HealthStatus struct {
	Status string `json:"status"`
}
