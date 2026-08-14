package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var defaultOIDCScopes = []string{"openid", "profile", "email"}

type IdentityPoolService struct {
	pools IdentityPoolRepository
}

func NewIdentityPoolService(pools IdentityPoolRepository) *IdentityPoolService {
	return &IdentityPoolService{pools: pools}
}

type CreateIdentityPoolInput struct {
	Name         string
	Slug         string
	Issuer       string
	ClientID     string
	ClientSecret string
	Scopes       []string
	Enabled      *bool
}

type UpdateIdentityPoolInput struct {
	Name         *string
	Slug         *string
	Issuer       *string
	ClientID     *string
	ClientSecret *string
	ClearSecret  bool
	Scopes       *[]string
	Enabled      *bool
}

func (s *IdentityPoolService) List(ctx context.Context) ([]*model.IdentityPool, error) {
	pools, err := s.pools.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list identity pools: %w", err)
	}
	if pools == nil {
		pools = []*model.IdentityPool{}
	}
	return pools, nil
}

func (s *IdentityPoolService) ListEnabledPublic(ctx context.Context) ([]model.PublicIdentityPool, error) {
	pools, err := s.pools.ListEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled identity pools: %w", err)
	}
	out := make([]model.PublicIdentityPool, 0, len(pools))
	for _, pool := range pools {
		out = append(out, model.PublicIdentityPool{Name: pool.Name, Slug: pool.Slug})
	}
	return out, nil
}

func (s *IdentityPoolService) GetByUUID(ctx context.Context, id uuid.UUID) (*model.IdentityPool, error) {
	pool, err := s.pools.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrIdentityPoolNotFound) {
			return nil, ErrIdentityPoolNotFound
		}
		return nil, fmt.Errorf("get identity pool: %w", err)
	}
	return pool, nil
}

func (s *IdentityPoolService) GetBySlug(ctx context.Context, slug string) (*model.IdentityPool, error) {
	pool, err := s.pools.GetBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, ErrIdentityPoolNotFound) {
			return nil, ErrIdentityPoolNotFound
		}
		return nil, fmt.Errorf("get identity pool by slug: %w", err)
	}
	return pool, nil
}

func (s *IdentityPoolService) Create(ctx context.Context, input CreateIdentityPoolInput) (*model.IdentityPool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, userError(ErrInvalidIdentityPool, "name is required")
	}
	slug, err := normalizeSlug(input.Slug)
	if err != nil {
		return nil, err
	}
	issuer, err := normalizeIssuer(input.Issuer)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" {
		return nil, userError(ErrInvalidIdentityPool, "client_id is required")
	}
	scopes := normalizeScopes(input.Scopes)

	if err := s.ensureUnique(ctx, name, slug, issuer, uuid.Nil); err != nil {
		return nil, err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	pool := &model.IdentityPool{
		UUID:         uuid.New(),
		Name:         name,
		Slug:         slug,
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: strings.TrimSpace(input.ClientSecret),
		Scopes:       scopes,
		Enabled:      enabled,
	}
	if err := s.pools.Create(ctx, pool); err != nil {
		return nil, fmt.Errorf("create identity pool: %w", err)
	}
	return pool, nil
}

func (s *IdentityPoolService) Update(ctx context.Context, id uuid.UUID, input UpdateIdentityPoolInput) (*model.IdentityPool, error) {
	pool, err := s.pools.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrIdentityPoolNotFound) {
			return nil, ErrIdentityPoolNotFound
		}
		return nil, fmt.Errorf("get identity pool: %w", err)
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, userError(ErrInvalidIdentityPool, "name is required")
		}
		pool.Name = name
	}
	if input.Slug != nil {
		slug, err := normalizeSlug(*input.Slug)
		if err != nil {
			return nil, err
		}
		pool.Slug = slug
	}
	if input.Issuer != nil {
		issuer, err := normalizeIssuer(*input.Issuer)
		if err != nil {
			return nil, err
		}
		pool.Issuer = issuer
	}
	if input.ClientID != nil {
		clientID := strings.TrimSpace(*input.ClientID)
		if clientID == "" {
			return nil, userError(ErrInvalidIdentityPool, "client_id is required")
		}
		pool.ClientID = clientID
	}
	if input.ClearSecret {
		pool.ClientSecret = ""
	} else if input.ClientSecret != nil {
		pool.ClientSecret = strings.TrimSpace(*input.ClientSecret)
	}
	if input.Scopes != nil {
		pool.Scopes = normalizeScopes(*input.Scopes)
	}
	if input.Enabled != nil {
		pool.Enabled = *input.Enabled
	}

	if err := s.ensureUnique(ctx, pool.Name, pool.Slug, pool.Issuer, pool.UUID); err != nil {
		return nil, err
	}

	if err := s.pools.Update(ctx, pool); err != nil {
		return nil, fmt.Errorf("update identity pool: %w", err)
	}
	return pool, nil
}

func (s *IdentityPoolService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.pools.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrIdentityPoolNotFound) {
			return ErrIdentityPoolNotFound
		}
		return fmt.Errorf("delete identity pool: %w", err)
	}
	return nil
}

func (s *IdentityPoolService) ensureUnique(ctx context.Context, name, slug, issuer string, self uuid.UUID) error {
	if existing, err := s.pools.GetBySlug(ctx, slug); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrIdentityPoolSlugTaken, `identity pool slug %q is already used`, slug)
	} else if err != nil && !errors.Is(err, ErrIdentityPoolNotFound) {
		return fmt.Errorf("check slug: %w", err)
	}

	if existing, err := s.pools.GetByIssuer(ctx, issuer); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrIdentityPoolIssuerTaken, `identity pool issuer %q is already used`, issuer)
	} else if err != nil && !errors.Is(err, ErrIdentityPoolNotFound) {
		return fmt.Errorf("check issuer: %w", err)
	}

	pools, err := s.pools.List(ctx)
	if err != nil {
		return fmt.Errorf("list pools for name check: %w", err)
	}
	for _, pool := range pools {
		if pool.UUID != self && strings.EqualFold(pool.Name, name) {
			return userError(ErrIdentityPoolNameTaken, `identity pool name %q is already used`, name)
		}
	}
	return nil
}

func normalizeSlug(raw string) (string, error) {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if slug == "" {
		return "", userError(ErrInvalidIdentityPool, "slug is required")
	}
	if !slugPattern.MatchString(slug) {
		return "", userError(ErrInvalidIdentityPool, "slug must be lowercase alphanumeric with optional hyphens")
	}
	return slug, nil
}

func normalizeIssuer(raw string) (string, error) {
	issuer := strings.TrimSpace(raw)
	issuer = strings.TrimRight(issuer, "/")
	if issuer == "" {
		return "", userError(ErrInvalidIdentityPool, "issuer is required")
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", userError(ErrInvalidIdentityPool, "issuer must be an absolute URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", userError(ErrInvalidIdentityPool, "issuer must use http or https")
	}
	return issuer, nil
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		out := make([]string, len(defaultOIDCScopes))
		copy(out, defaultOIDCScopes)
		return out
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(scopes))
	hasOpenID := false
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		if scope == "openid" {
			hasOpenID = true
		}
		out = append(out, scope)
	}
	if !hasOpenID {
		out = append([]string{"openid"}, out...)
	}
	return out
}
