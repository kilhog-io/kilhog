package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	GrantPrincipalLocalUser    GrantPrincipalKind = "local_user"
	GrantPrincipalOIDC         GrantPrincipalKind = "oidc"
	GrantPrincipalOIDCGroup    GrantPrincipalKind = "oidc_group"
	GrantPrincipalMachine      GrantPrincipalKind = "machine"
	GrantPrincipalMachinePool  GrantPrincipalKind = "machine_pool"

	GrantResourceNetwork  GrantResourceKind = "network"
	GrantResourceSubnet   GrantResourceKind = "subnet"
	GrantResourcePlatform GrantResourceKind = "platform"

	CapabilityCreateNetworks = "create_networks"

	DefaultGroupsClaim = "groups"
)

type GrantPrincipalKind string
type GrantResourceKind string

// Permissions are independent CRUD flags on a grant.
type Permissions struct {
	Create bool `json:"create"`
	Read   bool `json:"read"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

func (p Permissions) Any() bool {
	return p.Create || p.Read || p.Update || p.Delete
}

func (p Permissions) Union(other Permissions) Permissions {
	return Permissions{
		Create: p.Create || other.Create,
		Read:   p.Read || other.Read,
		Update: p.Update || other.Update,
		Delete: p.Delete || other.Delete,
	}
}

func (p Permissions) WithOwner(owner bool) Permissions {
	if owner {
		return Permissions{Create: true, Read: true, Update: true, Delete: true}
	}
	return p
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
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	// NetworkUUID is the tenancy network for network/subnet grants (cascade).
	NetworkUUID *uuid.UUID `json:"-"`
}
