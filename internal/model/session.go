package model

import (
	"time"

	"github.com/google/uuid"
)

// PrincipalKind identifies how a session was established.
type PrincipalKind string

const (
	PrincipalKindLocalUser PrincipalKind = "local_user"
	PrincipalKindOIDC      PrincipalKind = "oidc"
	PrincipalKindAPIKey    PrincipalKind = "api_key"
)

// Session is a server-side authenticated session.
type Session struct {
	UUID             uuid.UUID
	TokenHash        string
	PrincipalKind    PrincipalKind
	LocalUserUUID    *uuid.UUID
	IdentityPoolUUID *uuid.UUID
	OIDCSubject      string
	OIDCEmail        string
	OIDCName         string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

// OIDCLoginState holds PKCE/state data for an in-flight Authorization Code flow.
type OIDCLoginState struct {
	State        string
	PoolUUID     uuid.UUID
	CodeVerifier string
	Nonce        string
	RedirectURI  string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}
