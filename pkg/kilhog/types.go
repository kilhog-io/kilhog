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

// GrantPrincipalKind identifies a grant subject.
type GrantPrincipalKind string

const (
	GrantPrincipalLocalUser   GrantPrincipalKind = "local_user"
	GrantPrincipalOIDC        GrantPrincipalKind = "oidc"
	GrantPrincipalOIDCGroup   GrantPrincipalKind = "oidc_group"
	GrantPrincipalMachine     GrantPrincipalKind = "machine"
	GrantPrincipalMachinePool GrantPrincipalKind = "machine_pool"
)

// GrantResourceKind identifies a grant object.
type GrantResourceKind string

const (
	GrantResourceNetwork  GrantResourceKind = "network"
	GrantResourceSubnet   GrantResourceKind = "subnet"
	GrantResourcePlatform GrantResourceKind = "platform"
)

// Permissions are independent CRUD flags on a grant.
type Permissions struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

// GrantPrincipal is the subject of a grant.
type GrantPrincipal struct {
	Kind             GrantPrincipalKind `json:"kind"`
	LocalUserUUID    *uuid.UUID         `json:"local_user_uuid,omitempty"`
	IdentityPoolUUID *uuid.UUID         `json:"identity_pool_uuid,omitempty"`
	Subject          string             `json:"subject,omitempty"`
	Group            string             `json:"group,omitempty"`
	MachineUUID      *uuid.UUID         `json:"machine_uuid,omitempty"`
	MachinePoolUUID  *uuid.UUID         `json:"machine_pool_uuid,omitempty"`
}

// GrantResource is the object of a grant.
type GrantResource struct {
	Kind       GrantResourceKind `json:"kind"`
	UUID       *uuid.UUID        `json:"uuid,omitempty"`
	Capability string            `json:"capability,omitempty"`
}

// Grant assigns permissions to one principal on one resource.
type Grant struct {
	UUID        uuid.UUID      `json:"uuid"`
	Principal   GrantPrincipal `json:"principal"`
	Resource    GrantResource  `json:"resource"`
	Permissions Permissions    `json:"permissions"`
	Owner       bool           `json:"owner"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

// CreateResourceGrantInput is the request body for POST .../grants.
type CreateResourceGrantInput struct {
	Principal   GrantPrincipal `json:"principal"`
	Permissions Permissions    `json:"permissions"`
	Owner       bool           `json:"owner,omitempty"`
}

// CreatePlatformGrantInput is the request body for POST /auth/platform-grants.
type CreatePlatformGrantInput struct {
	Principal  GrantPrincipal `json:"principal"`
	Capability string         `json:"capability,omitempty"`
}

// UpdateGrantInput is the request body for PUT .../grants/{grant_uuid}.
type UpdateGrantInput struct {
	Permissions Permissions `json:"permissions"`
	Owner       bool        `json:"owner"`
}

// TransferOwnershipInput is the request body for POST .../ownership/transfer.
type TransferOwnershipInput struct {
	From *GrantPrincipal `json:"from,omitempty"`
	To   GrantPrincipal  `json:"to"`
}

// HealthStatus is returned by GET /healthz.
type HealthStatus struct {
	Status string `json:"status"`
}
