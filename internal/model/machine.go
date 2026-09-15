package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	PrincipalKindMachine PrincipalKind = "machine"

	AuthMethodAPIKey = "api_key"
	AuthMethodJWT    = "jwt"

	JWKSModeDiscovery JWKSMode = "discovery"
	JWKSModeURI       JWKSMode = "uri"
	JWKSModeStatic    JWKSMode = "static"

	ClaimOpEq     ClaimOp = "eq"
	ClaimOpIn     ClaimOp = "in"
	ClaimOpPrefix ClaimOp = "prefix"

	MachineAPIKeyPrefix = "khog_mkey_"
)

type JWKSMode string

type ClaimOp string

// ClaimCondition is a top-level JWT claim constraint on a machine.
type ClaimCondition struct {
	Claim string  `json:"claim"`
	Op    ClaimOp `json:"op"`
	Value any     `json:"value"`
}

// MachinePool is a named group of machine identities.
type MachinePool struct {
	UUID        uuid.UUID `json:"uuid"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MachineIdentityProvider is a JWT trust configuration owned by a machine pool.
type MachineIdentityProvider struct {
	UUID            uuid.UUID       `json:"uuid"`
	MachinePoolUUID uuid.UUID       `json:"machine_pool_uuid"`
	Name            string          `json:"name"`
	Issuer          string          `json:"issuer"`
	Audiences       []string        `json:"audiences"`
	JWKSMode        JWKSMode        `json:"jwks_mode"`
	JWKSURI         string          `json:"jwks_uri,omitempty"`
	JWKS            json.RawMessage `json:"jwks,omitempty"`
	Enabled         bool            `json:"enabled"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Machine is a named workload identity inside a machine pool.
type Machine struct {
	UUID            uuid.UUID        `json:"uuid"`
	MachinePoolUUID uuid.UUID        `json:"machine_pool_uuid"`
	Name            string           `json:"name"`
	Description     string           `json:"description,omitempty"`
	Enabled         bool             `json:"enabled"`
	ProviderUUID    *uuid.UUID       `json:"provider_uuid,omitempty"`
	Subject         string           `json:"subject,omitempty"`
	SubjectPrefix   string           `json:"subject_prefix,omitempty"`
	Claims          []ClaimCondition `json:"claims,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// MachineAPIKey is a secret bound to one machine. Secret is returned only at creation.
type MachineAPIKey struct {
	UUID        uuid.UUID  `json:"uuid"`
	MachineUUID uuid.UUID  `json:"machine_uuid"`
	Name        string     `json:"name,omitempty"`
	Prefix      string     `json:"prefix"`
	Secret      string     `json:"secret,omitempty"`
	TokenHash   string     `json:"-"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}
