package model

import (
	"time"

	"github.com/google/uuid"
)

// IdentityPool is a named OIDC provider configuration.
type IdentityPool struct {
	UUID         uuid.UUID `json:"uuid"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Issuer       string    `json:"issuer"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"-"`
	Scopes       []string  `json:"scopes"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// HasClientSecret is true when a secret is stored (secret value is never exposed).
	HasClientSecret bool `json:"has_client_secret"`
}

// PublicIdentityPool is the subset exposed for login discovery.
type PublicIdentityPool struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
