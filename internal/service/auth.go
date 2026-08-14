package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"golang.org/x/oauth2"
)

const (
	defaultSessionTTL = 24 * time.Hour
	oidcLoginStateTTL = 10 * time.Minute
	sessionCookieName = "kilhog_session"
)

// AuthConfig holds deployment settings for authentication.
type AuthConfig struct {
	APIKey          string
	BootstrapToken  string
	PublicURL       string
	SessionTTL      time.Duration
	HTTPClient      *http.Client
}

// Principal is the authenticated caller attached to a request.
type Principal struct {
	Kind             model.PrincipalKind `json:"kind"`
	LocalUser        *model.LocalUser    `json:"local_user,omitempty"`
	IdentityPoolUUID *uuid.UUID          `json:"identity_pool_uuid,omitempty"`
	OIDCSubject      string              `json:"oidc_subject,omitempty"`
	OIDCEmail        string              `json:"oidc_email,omitempty"`
	OIDCName         string              `json:"oidc_name,omitempty"`
	SessionUUID      *uuid.UUID          `json:"session_uuid,omitempty"`
}

func (p *Principal) IsAdmin() bool {
	return p != nil && p.Kind == model.PrincipalKindLocalUser && p.LocalUser != nil &&
		p.LocalUser.Role == model.UserRoleAdmin && p.LocalUser.Enabled
}

// SessionToken is a newly issued session credential.
type SessionToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// AuthStatus describes whether authentication methods are available.
type AuthStatus struct {
	Configured         bool `json:"configured"`
	APIKeyConfigured   bool `json:"api_key_configured"`
	LocalUsers         int  `json:"local_users"`
	EnabledOIDCPools   int  `json:"enabled_oidc_pools"`
	BootstrapAvailable bool `json:"bootstrap_available"`
}

type AuthService struct {
	users       UserRepository
	pools       IdentityPoolRepository
	sessions    SessionRepository
	loginStates OIDCLoginStateRepository
	cfg         AuthConfig

	providersMu sync.Mutex
	providers   map[string]*oidc.Provider
}

func NewAuthService(
	users UserRepository,
	pools IdentityPoolRepository,
	sessions SessionRepository,
	loginStates OIDCLoginStateRepository,
	cfg AuthConfig,
) *AuthService {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	return &AuthService{
		users:       users,
		pools:       pools,
		sessions:    sessions,
		loginStates: loginStates,
		cfg:         cfg,
		providers:   map[string]*oidc.Provider{},
	}
}

func SessionCookieName() string { return sessionCookieName }

func (s *AuthService) Status(ctx context.Context) (*AuthStatus, error) {
	users, err := s.users.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	pools, err := s.pools.CountEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("count oidc pools: %w", err)
	}
	apiKey := strings.TrimSpace(s.cfg.APIKey) != ""
	return &AuthStatus{
		Configured:         apiKey || users > 0 || pools > 0,
		APIKeyConfigured:   apiKey,
		LocalUsers:         users,
		EnabledOIDCPools:   pools,
		BootstrapAvailable: users == 0,
	}, nil
}

func (s *AuthService) IsConfigured(ctx context.Context) (bool, error) {
	status, err := s.Status(ctx)
	if err != nil {
		return false, err
	}
	return status.Configured, nil
}

type BootstrapInput struct {
	Username        string
	Password        string
	DisplayName     string
	Email           string
	BootstrapToken  string
}

func (s *AuthService) Bootstrap(ctx context.Context, input BootstrapInput) (*model.LocalUser, *SessionToken, error) {
	count, err := s.users.Count(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil, nil, userError(ErrBootstrapUnavailable, "bootstrap is unavailable because a local user already exists")
	}
	if err := s.checkBootstrapToken(input.BootstrapToken); err != nil {
		return nil, nil, err
	}

	userSvc := NewUserService(s.users)
	user, err := userSvc.Create(ctx, CreateUserInput{
		Username:    input.Username,
		Password:    input.Password,
		DisplayName: input.DisplayName,
		Email:       input.Email,
		Role:        model.UserRoleAdmin,
	})
	if err != nil {
		return nil, nil, err
	}

	token, err := s.createLocalSession(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, token, nil
}

type LocalLoginInput struct {
	Username string
	Password string
}

func (s *AuthService) LoginLocal(ctx context.Context, input LocalLoginInput) (*model.LocalUser, *SessionToken, error) {
	username := strings.TrimSpace(input.Username)
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, nil, userError(ErrInvalidCredentials, "invalid username or password")
		}
		return nil, nil, fmt.Errorf("get user: %w", err)
	}
	if !user.Enabled || !checkPassword(user.PasswordHash, input.Password) {
		return nil, nil, userError(ErrInvalidCredentials, "invalid username or password")
	}
	token, err := s.createLocalSession(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	return user, token, nil
}

func (s *AuthService) Logout(ctx context.Context, rawToken string) error {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil
	}
	if err := s.sessions.DeleteByTokenHash(ctx, hashToken(rawToken)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *AuthService) AuthenticateRequest(ctx context.Context, apiKeyHeader, bearerToken, sessionCookie string) (*Principal, error) {
	configured, err := s.IsConfigured(ctx)
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, ErrAuthNotConfigured
	}

	expectedKey := strings.TrimSpace(s.cfg.APIKey)
	if expectedKey != "" {
		candidates := []string{strings.TrimSpace(apiKeyHeader)}
		if bearerToken != "" {
			candidates = append(candidates, bearerToken)
		}
		for _, candidate := range candidates {
			if candidate != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(expectedKey)) == 1 {
				return &Principal{Kind: model.PrincipalKindAPIKey}, nil
			}
		}
	}

	for _, raw := range []string{bearerToken, sessionCookie} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if principal, err := s.authenticateSession(ctx, raw); err == nil {
			return principal, nil
		} else if !errors.Is(err, ErrSessionNotFound) && !errors.Is(err, ErrSessionExpired) {
			return nil, err
		}
	}

	if bearerToken != "" && looksLikeJWT(bearerToken) {
		if principal, err := s.authenticateOIDCBearer(ctx, bearerToken); err == nil {
			return principal, nil
		} else if !errors.Is(err, ErrUnauthenticated) {
			return nil, err
		}
	}

	return nil, ErrUnauthenticated
}

func (s *AuthService) Me(ctx context.Context, principal *Principal) (*Principal, error) {
	if principal == nil {
		return nil, ErrUnauthenticated
	}
	return principal, nil
}

type OIDCLoginStart struct {
	AuthURL string `json:"auth_url"`
}

func (s *AuthService) StartOIDCLogin(ctx context.Context, slug string) (*OIDCLoginStart, error) {
	pool, err := s.pools.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, ErrIdentityPoolNotFound) {
			return nil, ErrIdentityPoolNotFound
		}
		return nil, fmt.Errorf("get identity pool: %w", err)
	}
	if !pool.Enabled {
		return nil, userError(ErrOIDCNotConfigured, "identity pool is disabled")
	}
	if s.cfg.PublicURL == "" {
		return nil, userError(ErrInvalidIdentityPool, "KILHOG_PUBLIC_URL must be set for OIDC login")
	}

	provider, err := s.providerFor(ctx, pool.Issuer)
	if err != nil {
		return nil, err
	}

	state, err := randomURLString(32)
	if err != nil {
		return nil, err
	}
	verifier := oauth2.GenerateVerifier()
	nonce, err := randomURLString(32)
	if err != nil {
		return nil, err
	}

	redirectURI := s.cfg.PublicURL + "/auth/oidc/callback"
	if err := s.loginStates.Create(ctx, &model.OIDCLoginState{
		State:        state,
		PoolUUID:     pool.UUID,
		CodeVerifier: verifier,
		Nonce:        nonce,
		RedirectURI:  redirectURI,
		ExpiresAt:    time.Now().UTC().Add(oidcLoginStateTTL),
	}); err != nil {
		return nil, fmt.Errorf("store oidc login state: %w", err)
	}

	oauthCfg := s.oauthConfig(pool, provider, redirectURI)
	authURL := oauthCfg.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.S256ChallengeOption(verifier),
		oidc.Nonce(nonce),
	)
	return &OIDCLoginStart{AuthURL: authURL}, nil
}

func (s *AuthService) CompleteOIDCLogin(ctx context.Context, state, code string) (*SessionToken, *Principal, error) {
	state = strings.TrimSpace(state)
	code = strings.TrimSpace(code)
	if state == "" || code == "" {
		return nil, nil, userError(ErrUnauthenticated, "missing state or code")
	}

	loginState, err := s.loginStates.Take(ctx, state)
	if err != nil {
		if errors.Is(err, ErrOIDCLoginStateNotFound) {
			return nil, nil, userError(ErrUnauthenticated, "invalid or expired OIDC login state")
		}
		return nil, nil, fmt.Errorf("take oidc login state: %w", err)
	}
	if time.Now().UTC().After(loginState.ExpiresAt) {
		return nil, nil, userError(ErrOIDCLoginStateExpired, "OIDC login state expired")
	}

	pool, err := s.pools.GetByUUID(ctx, loginState.PoolUUID)
	if err != nil {
		return nil, nil, fmt.Errorf("get identity pool: %w", err)
	}
	if !pool.Enabled {
		return nil, nil, userError(ErrOIDCNotConfigured, "identity pool is disabled")
	}

	provider, err := s.providerFor(ctx, pool.Issuer)
	if err != nil {
		return nil, nil, err
	}
	oauthCfg := s.oauthConfig(pool, provider, loginState.RedirectURI)

	token, err := oauthCfg.Exchange(ctx, code, oauth2.VerifierOption(loginState.CodeVerifier))
	if err != nil {
		return nil, nil, userError(ErrUnauthenticated, "failed to exchange authorization code")
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, nil, userError(ErrUnauthenticated, "missing id_token in token response")
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: pool.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, nil, userError(ErrUnauthenticated, "invalid id_token")
	}
	if idToken.Nonce != loginState.Nonce {
		return nil, nil, userError(ErrUnauthenticated, "invalid id_token nonce")
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	_ = idToken.Claims(&claims)

	sessionToken, err := s.createOIDCSession(ctx, pool, idToken.Subject, claims.Email, claims.Name)
	if err != nil {
		return nil, nil, err
	}
	poolUUID := pool.UUID
	principal := &Principal{
		Kind:             model.PrincipalKindOIDC,
		IdentityPoolUUID: &poolUUID,
		OIDCSubject:      idToken.Subject,
		OIDCEmail:        claims.Email,
		OIDCName:         claims.Name,
	}
	return sessionToken, principal, nil
}

func (s *AuthService) checkBootstrapToken(provided string) error {
	expected := strings.TrimSpace(s.cfg.BootstrapToken)
	if expected == "" {
		return nil
	}
	provided = strings.TrimSpace(provided)
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return userError(ErrBootstrapForbidden, "invalid bootstrap token")
	}
	return nil
}

func (s *AuthService) createLocalSession(ctx context.Context, user *model.LocalUser) (*SessionToken, error) {
	raw, hash, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(s.cfg.SessionTTL)
	userUUID := user.UUID
	session := &model.Session{
		UUID:          uuid.New(),
		TokenHash:     hash,
		PrincipalKind: model.PrincipalKindLocalUser,
		LocalUserUUID: &userUUID,
		ExpiresAt:     expires,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &SessionToken{Token: raw, ExpiresAt: expires}, nil
}

func (s *AuthService) createOIDCSession(ctx context.Context, pool *model.IdentityPool, subject, email, name string) (*SessionToken, error) {
	raw, hash, err := newSessionToken()
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(s.cfg.SessionTTL)
	poolUUID := pool.UUID
	session := &model.Session{
		UUID:             uuid.New(),
		TokenHash:        hash,
		PrincipalKind:    model.PrincipalKindOIDC,
		IdentityPoolUUID: &poolUUID,
		OIDCSubject:      subject,
		OIDCEmail:        email,
		OIDCName:         name,
		ExpiresAt:        expires,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &SessionToken{Token: raw, ExpiresAt: expires}, nil
}

func (s *AuthService) authenticateSession(ctx context.Context, rawToken string) (*Principal, error) {
	session, err := s.sessions.GetByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.sessions.DeleteByTokenHash(ctx, session.TokenHash)
		return nil, ErrSessionExpired
	}

	sessionUUID := session.UUID
	switch session.PrincipalKind {
	case model.PrincipalKindLocalUser:
		if session.LocalUserUUID == nil {
			return nil, ErrSessionNotFound
		}
		user, err := s.users.GetByUUID(ctx, *session.LocalUserUUID)
		if err != nil {
			return nil, err
		}
		if !user.Enabled {
			return nil, ErrUnauthenticated
		}
		return &Principal{
			Kind:        model.PrincipalKindLocalUser,
			LocalUser:   user,
			SessionUUID: &sessionUUID,
		}, nil
	case model.PrincipalKindOIDC:
		return &Principal{
			Kind:             model.PrincipalKindOIDC,
			IdentityPoolUUID: session.IdentityPoolUUID,
			OIDCSubject:      session.OIDCSubject,
			OIDCEmail:        session.OIDCEmail,
			OIDCName:         session.OIDCName,
			SessionUUID:      &sessionUUID,
		}, nil
	default:
		return nil, ErrSessionNotFound
	}
}

func (s *AuthService) authenticateOIDCBearer(ctx context.Context, rawToken string) (*Principal, error) {
	pools, err := s.pools.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled pools: %w", err)
	}
	for _, pool := range pools {
		provider, err := s.providerFor(ctx, pool.Issuer)
		if err != nil {
			continue
		}
		verifier := provider.Verifier(&oidc.Config{ClientID: pool.ClientID})
		idToken, err := verifier.Verify(ctx, rawToken)
		if err != nil {
			// Access tokens are often not ID tokens; try as JWT access token with audience = client_id.
			accessVerifier := provider.Verifier(&oidc.Config{
				ClientID:             pool.ClientID,
				SkipClientIDCheck:    false,
				SupportedSigningAlgs: nil,
			})
			idToken, err = accessVerifier.Verify(ctx, rawToken)
			if err != nil {
				continue
			}
		}
		var claims struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		_ = idToken.Claims(&claims)
		poolUUID := pool.UUID
		return &Principal{
			Kind:             model.PrincipalKindOIDC,
			IdentityPoolUUID: &poolUUID,
			OIDCSubject:      idToken.Subject,
			OIDCEmail:        claims.Email,
			OIDCName:         claims.Name,
		}, nil
	}
	return nil, ErrUnauthenticated
}

func (s *AuthService) providerFor(ctx context.Context, issuer string) (*oidc.Provider, error) {
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	if p, ok := s.providers[issuer]; ok {
		return p, nil
	}
	ctx = oidc.ClientContext(ctx, s.cfg.HTTPClient)
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, userError(ErrOIDCNotConfigured, "failed to discover OIDC provider for issuer %s", issuer)
	}
	s.providers[issuer] = provider
	return provider, nil
}

func (s *AuthService) oauthConfig(pool *model.IdentityPool, provider *oidc.Provider, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     pool.ClientID,
		ClientSecret: pool.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       pool.Scopes,
	}
}

func newSessionToken() (raw, hash string, err error) {
	raw, err = randomURLString(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomURLString(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func looksLikeJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3
}

// RequireAdmin returns ErrForbidden when the principal is not a local admin.
func RequireAdmin(principal *Principal) error {
	if principal == nil || !principal.IsAdmin() {
		return ErrForbidden
	}
	return nil
}
