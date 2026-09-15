package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

const (
	jwksCacheTTL       = time.Hour
	machineHTTPTimeout = 10 * time.Second
)

type MachineIdentityService struct {
	pools     MachinePoolRepository
	providers MachineProviderRepository
	machines  MachineRepository
	keys      MachineAPIKeyRepository
	http      *http.Client

	jwksMu    sync.Mutex
	jwksCache map[string]jwksCacheEntry
}

type jwksCacheEntry struct {
	set       *jose.JSONWebKeySet
	fetchedAt time.Time
}

func NewMachineIdentityService(
	pools MachinePoolRepository,
	providers MachineProviderRepository,
	machines MachineRepository,
	keys MachineAPIKeyRepository,
	httpClient *http.Client,
) *MachineIdentityService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: machineHTTPTimeout}
	}
	return &MachineIdentityService{
		pools:     pools,
		providers: providers,
		machines:  machines,
		keys:      keys,
		http:      httpClient,
		jwksCache: map[string]jwksCacheEntry{},
	}
}

// SetHTTPClient replaces the client used to fetch discovery documents and JWKS.
func (s *MachineIdentityService) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	s.http = client
}

func (s *MachineIdentityService) CountEnabledPools(ctx context.Context) (int, error) {
	n, err := s.pools.CountEnabled(ctx)
	if err != nil {
		return 0, fmt.Errorf("count enabled machine pools: %w", err)
	}
	return n, nil
}

type CreateMachinePoolInput struct {
	Name        string
	Slug        string
	Description string
	Enabled     *bool
}

type UpdateMachinePoolInput struct {
	Name        *string
	Slug        *string
	Description *string
	Enabled     *bool
}

func (s *MachineIdentityService) ListPools(ctx context.Context) ([]*model.MachinePool, error) {
	pools, err := s.pools.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list machine pools: %w", err)
	}
	if pools == nil {
		pools = []*model.MachinePool{}
	}
	return pools, nil
}

func (s *MachineIdentityService) GetPool(ctx context.Context, id uuid.UUID) (*model.MachinePool, error) {
	pool, err := s.pools.GetByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrMachinePoolNotFound) {
			return nil, ErrMachinePoolNotFound
		}
		return nil, fmt.Errorf("get machine pool: %w", err)
	}
	return pool, nil
}

func (s *MachineIdentityService) CreatePool(ctx context.Context, input CreateMachinePoolInput) (*model.MachinePool, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, userError(ErrInvalidMachinePool, "name is required")
	}
	slug, err := normalizeSlug(input.Slug)
	if err != nil {
		return nil, remapMachinePoolSlugError(err)
	}
	if err := s.ensurePoolUnique(ctx, name, slug, uuid.Nil); err != nil {
		return nil, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	pool := &model.MachinePool{
		UUID:        uuid.New(),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(input.Description),
		Enabled:     enabled,
	}
	if err := s.pools.Create(ctx, pool); err != nil {
		return nil, fmt.Errorf("create machine pool: %w", err)
	}
	return pool, nil
}

func (s *MachineIdentityService) UpdatePool(ctx context.Context, id uuid.UUID, input UpdateMachinePoolInput) (*model.MachinePool, error) {
	pool, err := s.GetPool(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, userError(ErrInvalidMachinePool, "name is required")
		}
		pool.Name = name
	}
	if input.Slug != nil {
		slug, err := normalizeSlug(*input.Slug)
		if err != nil {
			return nil, remapMachinePoolSlugError(err)
		}
		pool.Slug = slug
	}
	if input.Description != nil {
		pool.Description = strings.TrimSpace(*input.Description)
	}
	if input.Enabled != nil {
		pool.Enabled = *input.Enabled
	}
	if err := s.ensurePoolUnique(ctx, pool.Name, pool.Slug, pool.UUID); err != nil {
		return nil, err
	}
	if err := s.pools.Update(ctx, pool); err != nil {
		return nil, fmt.Errorf("update machine pool: %w", err)
	}
	return pool, nil
}

func (s *MachineIdentityService) DeletePool(ctx context.Context, id uuid.UUID) error {
	if err := s.pools.Delete(ctx, id); err != nil {
		if errors.Is(err, ErrMachinePoolNotFound) {
			return ErrMachinePoolNotFound
		}
		return fmt.Errorf("delete machine pool: %w", err)
	}
	return nil
}

func (s *MachineIdentityService) ensurePoolUnique(ctx context.Context, name, slug string, self uuid.UUID) error {
	if existing, err := s.pools.GetBySlug(ctx, slug); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrMachinePoolSlugTaken, `machine pool slug %q is already used`, slug)
	} else if err != nil && !errors.Is(err, ErrMachinePoolNotFound) {
		return fmt.Errorf("check machine pool slug: %w", err)
	}
	pools, err := s.pools.List(ctx)
	if err != nil {
		return fmt.Errorf("list machine pools for name check: %w", err)
	}
	for _, pool := range pools {
		if pool.UUID != self && strings.EqualFold(pool.Name, name) {
			return userError(ErrMachinePoolNameTaken, `machine pool name %q is already used`, name)
		}
	}
	return nil
}

type CreateMachineProviderInput struct {
	Name      string
	Issuer    string
	Audiences []string
	JWKSMode  model.JWKSMode
	JWKSURI   string
	JWKS      json.RawMessage
	Enabled   *bool
}

type UpdateMachineProviderInput struct {
	Name      *string
	Issuer    *string
	Audiences *[]string
	JWKSMode  *model.JWKSMode
	JWKSURI   *string
	JWKS      *json.RawMessage
	Enabled   *bool
}

func (s *MachineIdentityService) ListProviders(ctx context.Context, poolUUID uuid.UUID) ([]*model.MachineIdentityProvider, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	list, err := s.providers.ListByPool(ctx, poolUUID)
	if err != nil {
		return nil, fmt.Errorf("list machine identity providers: %w", err)
	}
	if list == nil {
		list = []*model.MachineIdentityProvider{}
	}
	for _, provider := range list {
		sanitizeProviderJWKS(provider)
	}
	return list, nil
}

func (s *MachineIdentityService) GetProvider(ctx context.Context, poolUUID, providerUUID uuid.UUID) (*model.MachineIdentityProvider, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	provider, err := s.providers.GetByUUID(ctx, providerUUID)
	if err != nil {
		if errors.Is(err, ErrMachineProviderNotFound) {
			return nil, ErrMachineProviderNotFound
		}
		return nil, fmt.Errorf("get machine identity provider: %w", err)
	}
	if provider.MachinePoolUUID != poolUUID {
		return nil, ErrMachineProviderNotFound
	}
	sanitizeProviderJWKS(provider)
	return provider, nil
}

func (s *MachineIdentityService) CreateProvider(ctx context.Context, poolUUID uuid.UUID, input CreateMachineProviderInput) (*model.MachineIdentityProvider, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	provider := &model.MachineIdentityProvider{
		UUID:            uuid.New(),
		MachinePoolUUID: poolUUID,
		JWKSMode:        input.JWKSMode,
		JWKSURI:         input.JWKSURI,
		JWKS:            input.JWKS,
		Audiences:       input.Audiences,
	}
	name, issuer, audiences, mode, jwksURI, jwks, err := s.normalizeProvider(
		input.Name, input.Issuer, input.Audiences, input.JWKSMode, input.JWKSURI, input.JWKS,
	)
	if err != nil {
		return nil, err
	}
	provider.Name = name
	provider.Issuer = issuer
	provider.Audiences = audiences
	provider.JWKSMode = mode
	provider.JWKSURI = jwksURI
	provider.JWKS = jwks
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	provider.Enabled = enabled
	if err := s.ensureProviderUnique(ctx, poolUUID, provider.Name, provider.Issuer, uuid.Nil); err != nil {
		return nil, err
	}
	if err := s.providers.Create(ctx, provider); err != nil {
		return nil, fmt.Errorf("create machine identity provider: %w", err)
	}
	sanitizeProviderJWKS(provider)
	return provider, nil
}

func (s *MachineIdentityService) UpdateProvider(ctx context.Context, poolUUID, providerUUID uuid.UUID, input UpdateMachineProviderInput) (*model.MachineIdentityProvider, error) {
	provider, err := s.GetProvider(ctx, poolUUID, providerUUID)
	if err != nil {
		return nil, err
	}
	name := provider.Name
	issuer := provider.Issuer
	audiences := provider.Audiences
	mode := provider.JWKSMode
	jwksURI := provider.JWKSURI
	jwks := provider.JWKS
	if input.Name != nil {
		name = *input.Name
	}
	if input.Issuer != nil {
		issuer = *input.Issuer
	}
	if input.Audiences != nil {
		audiences = *input.Audiences
	}
	if input.JWKSMode != nil {
		mode = *input.JWKSMode
	}
	if input.JWKSURI != nil {
		jwksURI = *input.JWKSURI
	}
	if input.JWKS != nil {
		jwks = *input.JWKS
	}
	name, issuer, audiences, mode, jwksURI, jwks, err = s.normalizeProvider(name, issuer, audiences, mode, jwksURI, jwks)
	if err != nil {
		return nil, err
	}
	provider.Name = name
	provider.Issuer = issuer
	provider.Audiences = audiences
	provider.JWKSMode = mode
	provider.JWKSURI = jwksURI
	provider.JWKS = jwks
	if input.Enabled != nil {
		provider.Enabled = *input.Enabled
	}
	if err := s.ensureProviderUnique(ctx, poolUUID, provider.Name, provider.Issuer, provider.UUID); err != nil {
		return nil, err
	}
	if err := s.providers.Update(ctx, provider); err != nil {
		return nil, fmt.Errorf("update machine identity provider: %w", err)
	}
	s.invalidateJWKS(provider.UUID)
	sanitizeProviderJWKS(provider)
	return provider, nil
}

func (s *MachineIdentityService) DeleteProvider(ctx context.Context, poolUUID, providerUUID uuid.UUID) error {
	if _, err := s.GetProvider(ctx, poolUUID, providerUUID); err != nil {
		return err
	}
	if err := s.providers.Delete(ctx, providerUUID); err != nil {
		if errors.Is(err, ErrMachineProviderNotFound) {
			return ErrMachineProviderNotFound
		}
		return fmt.Errorf("delete machine identity provider: %w", err)
	}
	s.invalidateJWKS(providerUUID)
	return nil
}

func (s *MachineIdentityService) RefreshProviderJWKS(ctx context.Context, poolUUID, providerUUID uuid.UUID) (*model.MachineIdentityProvider, error) {
	provider, err := s.GetProvider(ctx, poolUUID, providerUUID)
	if err != nil {
		return nil, err
	}
	if provider.JWKSMode == model.JWKSModeStatic {
		return nil, userError(ErrInvalidMachineProvider, "static JWKS cannot be refreshed; update the document instead")
	}
	s.invalidateJWKS(provider.UUID)
	if _, err := s.keysForProvider(ctx, provider, true); err != nil {
		return nil, userError(ErrInvalidMachineProvider, "failed to refresh JWKS: %s", err.Error())
	}
	return provider, nil
}

func (s *MachineIdentityService) ensureProviderUnique(ctx context.Context, poolUUID uuid.UUID, name, issuer string, self uuid.UUID) error {
	if existing, err := s.providers.GetByName(ctx, poolUUID, name); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrMachineProviderNameTaken, `machine identity provider name %q is already used in this pool`, name)
	} else if err != nil && !errors.Is(err, ErrMachineProviderNotFound) {
		return fmt.Errorf("check provider name: %w", err)
	}
	if existing, err := s.providers.GetByIssuer(ctx, poolUUID, issuer); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrMachineProviderIssuerTaken, `machine identity provider issuer %q is already used in this pool`, issuer)
	} else if err != nil && !errors.Is(err, ErrMachineProviderNotFound) {
		return fmt.Errorf("check provider issuer: %w", err)
	}
	return nil
}

func (s *MachineIdentityService) normalizeProvider(
	name, issuer string, audiences []string, mode model.JWKSMode, jwksURI string, jwks json.RawMessage,
) (string, string, []string, model.JWKSMode, string, json.RawMessage, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", nil, "", "", nil, userError(ErrInvalidMachineProvider, "name is required")
	}
	issuer, err := normalizeHTTPSURL(issuer, "issuer")
	if err != nil {
		return "", "", nil, "", "", nil, err
	}
	normalizedAudiences := make([]string, 0, len(audiences))
	seen := map[string]struct{}{}
	for _, aud := range audiences {
		aud = strings.TrimSpace(aud)
		if aud == "" {
			continue
		}
		if _, ok := seen[aud]; ok {
			continue
		}
		seen[aud] = struct{}{}
		normalizedAudiences = append(normalizedAudiences, aud)
	}
	if len(normalizedAudiences) == 0 {
		return "", "", nil, "", "", nil, userError(ErrInvalidMachineProvider, "at least one audience is required")
	}
	mode = model.JWKSMode(strings.TrimSpace(string(mode)))
	switch mode {
	case model.JWKSModeDiscovery:
		jwksURI = ""
		jwks = nil
	case model.JWKSModeURI:
		jwksURI, err = normalizeHTTPSURL(jwksURI, "jwks_uri")
		if err != nil {
			return "", "", nil, "", "", nil, err
		}
		jwks = nil
	case model.JWKSModeStatic:
		jwksURI = ""
		if err := validateStaticJWKS(jwks); err != nil {
			return "", "", nil, "", "", nil, err
		}
	default:
		return "", "", nil, "", "", nil, userError(ErrInvalidMachineProvider, "jwks_mode must be discovery, uri, or static")
	}
	return name, issuer, normalizedAudiences, mode, jwksURI, jwks, nil
}

func sanitizeProviderJWKS(provider *model.MachineIdentityProvider) {
	if provider == nil {
		return
	}
	if provider.JWKSMode != model.JWKSModeStatic {
		provider.JWKS = nil
	}
}

func validateStaticJWKS(raw json.RawMessage) error {
	if len(raw) == 0 {
		return userError(ErrInvalidMachineProvider, "jwks is required when jwks_mode is static")
	}
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil {
		return userError(ErrInvalidMachineProvider, "jwks must be a JWKS document")
	}
	if len(set.Keys) == 0 {
		return userError(ErrInvalidMachineProvider, "jwks must contain at least one key")
	}
	return nil
}

func normalizeHTTPSURL(raw, field string) (string, error) {
	value := strings.TrimSpace(raw)
	value = strings.TrimRight(value, "/")
	if value == "" {
		return "", userError(ErrInvalidMachineProvider, "%s is required", field)
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", userError(ErrInvalidMachineProvider, "%s must be an absolute HTTPS URL", field)
	}
	if u.Scheme != "https" {
		return "", userError(ErrInvalidMachineProvider, "%s must use https", field)
	}
	return value, nil
}

type CreateMachineInput struct {
	Name          string
	Description   string
	Enabled       *bool
	ProviderUUID  *uuid.UUID
	Subject       string
	SubjectPrefix string
	Claims        []model.ClaimCondition
}

type UpdateMachineInput struct {
	Name          *string
	Description   *string
	Enabled       *bool
	ProviderUUID  **uuid.UUID
	ClearProvider bool
	Subject       *string
	SubjectPrefix *string
	Claims        *[]model.ClaimCondition
}

func (s *MachineIdentityService) ListMachines(ctx context.Context, poolUUID uuid.UUID) ([]*model.Machine, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	list, err := s.machines.ListByPool(ctx, poolUUID)
	if err != nil {
		return nil, fmt.Errorf("list machines: %w", err)
	}
	if list == nil {
		list = []*model.Machine{}
	}
	return list, nil
}

func (s *MachineIdentityService) GetMachine(ctx context.Context, poolUUID, machineUUID uuid.UUID) (*model.Machine, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	machine, err := s.machines.GetByUUID(ctx, machineUUID)
	if err != nil {
		if errors.Is(err, ErrMachineNotFound) {
			return nil, ErrMachineNotFound
		}
		return nil, fmt.Errorf("get machine: %w", err)
	}
	if machine.MachinePoolUUID != poolUUID {
		return nil, ErrMachineNotFound
	}
	return machine, nil
}

func (s *MachineIdentityService) CreateMachine(ctx context.Context, poolUUID uuid.UUID, input CreateMachineInput) (*model.Machine, error) {
	if _, err := s.GetPool(ctx, poolUUID); err != nil {
		return nil, err
	}
	machine, err := s.buildMachine(ctx, poolUUID, uuid.Nil, input.Name, input.Description, true, input.Enabled, input.ProviderUUID, input.Subject, input.SubjectPrefix, input.Claims)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMachineNameUnique(ctx, poolUUID, machine.Name, uuid.Nil); err != nil {
		return nil, err
	}
	if err := s.machines.Create(ctx, machine); err != nil {
		return nil, fmt.Errorf("create machine: %w", err)
	}
	return machine, nil
}

func (s *MachineIdentityService) UpdateMachine(ctx context.Context, poolUUID, machineUUID uuid.UUID, input UpdateMachineInput) (*model.Machine, error) {
	machine, err := s.GetMachine(ctx, poolUUID, machineUUID)
	if err != nil {
		return nil, err
	}
	name := machine.Name
	description := machine.Description
	enabled := machine.Enabled
	providerUUID := machine.ProviderUUID
	subject := machine.Subject
	subjectPrefix := machine.SubjectPrefix
	claims := machine.Claims
	if input.Name != nil {
		name = *input.Name
	}
	if input.Description != nil {
		description = *input.Description
	}
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if input.ClearProvider {
		providerUUID = nil
	} else if input.ProviderUUID != nil {
		providerUUID = *input.ProviderUUID
	}
	if input.Subject != nil {
		subject = *input.Subject
	}
	if input.SubjectPrefix != nil {
		subjectPrefix = *input.SubjectPrefix
	}
	if input.Claims != nil {
		claims = *input.Claims
	}
	updated, err := s.buildMachine(ctx, poolUUID, machine.UUID, name, description, enabled, nil, providerUUID, subject, subjectPrefix, claims)
	if err != nil {
		return nil, err
	}
	updated.UUID = machine.UUID
	updated.CreatedAt = machine.CreatedAt
	if err := s.ensureMachineNameUnique(ctx, poolUUID, updated.Name, updated.UUID); err != nil {
		return nil, err
	}
	if err := s.machines.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("update machine: %w", err)
	}
	return updated, nil
}

func (s *MachineIdentityService) DeleteMachine(ctx context.Context, poolUUID, machineUUID uuid.UUID) error {
	if _, err := s.GetMachine(ctx, poolUUID, machineUUID); err != nil {
		return err
	}
	if err := s.machines.Delete(ctx, machineUUID); err != nil {
		if errors.Is(err, ErrMachineNotFound) {
			return ErrMachineNotFound
		}
		return fmt.Errorf("delete machine: %w", err)
	}
	return nil
}

func (s *MachineIdentityService) buildMachine(
	ctx context.Context,
	poolUUID, self uuid.UUID,
	name, description string,
	enabled bool,
	enabledPtr *bool,
	providerUUID *uuid.UUID,
	subject, subjectPrefix string,
	claims []model.ClaimCondition,
) (*model.Machine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, userError(ErrInvalidMachine, "name is required")
	}
	if enabledPtr != nil {
		enabled = *enabledPtr
	}
	subject = strings.TrimSpace(subject)
	subjectPrefix = strings.TrimSpace(subjectPrefix)
	normalizedClaims, err := normalizeClaimConditions(claims)
	if err != nil {
		return nil, err
	}
	if providerUUID != nil {
		provider, err := s.providers.GetByUUID(ctx, *providerUUID)
		if err != nil {
			if errors.Is(err, ErrMachineProviderNotFound) {
				return nil, userError(ErrInvalidMachine, "provider not found")
			}
			return nil, fmt.Errorf("get provider for machine: %w", err)
		}
		if provider.MachinePoolUUID != poolUUID {
			return nil, userError(ErrInvalidMachine, "provider must belong to the same machine pool")
		}
		if subject == "" && subjectPrefix == "" && len(normalizedClaims) == 0 {
			return nil, userError(ErrInvalidMachine, "a JWT machine must have subject, subject_prefix, or claims")
		}
	} else {
		subject = ""
		subjectPrefix = ""
		normalizedClaims = nil
	}
	id := self
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &model.Machine{
		UUID:            id,
		MachinePoolUUID: poolUUID,
		Name:            name,
		Description:     strings.TrimSpace(description),
		Enabled:         enabled,
		ProviderUUID:    providerUUID,
		Subject:         subject,
		SubjectPrefix:   subjectPrefix,
		Claims:          normalizedClaims,
	}, nil
}

func (s *MachineIdentityService) ensureMachineNameUnique(ctx context.Context, poolUUID uuid.UUID, name string, self uuid.UUID) error {
	if existing, err := s.machines.GetByName(ctx, poolUUID, name); err == nil && existing != nil && existing.UUID != self {
		return userError(ErrMachineNameTaken, `machine name %q is already used in this pool`, name)
	} else if err != nil && !errors.Is(err, ErrMachineNotFound) {
		return fmt.Errorf("check machine name: %w", err)
	}
	return nil
}

func normalizeClaimConditions(claims []model.ClaimCondition) ([]model.ClaimCondition, error) {
	if len(claims) == 0 {
		return nil, nil
	}
	out := make([]model.ClaimCondition, 0, len(claims))
	for _, claim := range claims {
		name := strings.TrimSpace(claim.Claim)
		if name == "" {
			return nil, userError(ErrInvalidMachine, "claim name is required")
		}
		op := model.ClaimOp(strings.TrimSpace(string(claim.Op)))
		switch op {
		case model.ClaimOpEq, model.ClaimOpPrefix:
			value, ok := claimValueString(claim.Value)
			if !ok || value == "" {
				return nil, userError(ErrInvalidMachine, "claim %q value must be a non-empty string", name)
			}
			out = append(out, model.ClaimCondition{Claim: name, Op: op, Value: value})
		case model.ClaimOpIn:
			values, ok := claimValueStringList(claim.Value)
			if !ok || len(values) == 0 {
				return nil, userError(ErrInvalidMachine, "claim %q value must be a non-empty list of strings", name)
			}
			out = append(out, model.ClaimCondition{Claim: name, Op: op, Value: values})
		default:
			return nil, userError(ErrInvalidMachine, "claim op must be eq, in, or prefix")
		}
	}
	return out, nil
}

func claimValueString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v), true
	case json.Number:
		return v.String(), true
	default:
		return "", false
	}
}

func claimValueStringList(value any) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				return nil, false
			}
			out = append(out, item)
		}
		return out, len(out) > 0
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := claimValueString(item)
			if !ok || s == "" {
				return nil, false
			}
			out = append(out, s)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

type CreateMachineAPIKeyInput struct {
	Name      string
	ExpiresAt *time.Time
}

func (s *MachineIdentityService) ListAPIKeys(ctx context.Context, poolUUID, machineUUID uuid.UUID) ([]*model.MachineAPIKey, error) {
	if _, err := s.GetMachine(ctx, poolUUID, machineUUID); err != nil {
		return nil, err
	}
	list, err := s.keys.ListByMachine(ctx, machineUUID)
	if err != nil {
		return nil, fmt.Errorf("list machine api keys: %w", err)
	}
	if list == nil {
		list = []*model.MachineAPIKey{}
	}
	return list, nil
}

func (s *MachineIdentityService) CreateAPIKey(ctx context.Context, poolUUID, machineUUID uuid.UUID, input CreateMachineAPIKeyInput) (*model.MachineAPIKey, error) {
	if _, err := s.GetMachine(ctx, poolUUID, machineUUID); err != nil {
		return nil, err
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now().UTC()) {
		return nil, userError(ErrInvalidMachineAPIKey, "expires_at must be in the future")
	}
	var (
		prefix string
		secret string
		raw    string
		err    error
	)
	for i := 0; i < 5; i++ {
		prefix, secret, raw, err = newMachineAPIKeySecret()
		if err != nil {
			return nil, err
		}
		if _, err := s.keys.GetByPrefix(ctx, prefix); errors.Is(err, ErrMachineAPIKeyNotFound) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("check machine api key prefix: %w", err)
		}
		if i == 4 {
			return nil, fmt.Errorf("generate unique machine api key prefix: exhausted retries")
		}
	}
	key := &model.MachineAPIKey{
		UUID:        uuid.New(),
		MachineUUID: machineUUID,
		Name:        strings.TrimSpace(input.Name),
		Prefix:      prefix,
		Secret:      raw,
		TokenHash:   hashToken(raw),
		ExpiresAt:   input.ExpiresAt,
	}
	_ = secret
	if err := s.keys.Create(ctx, key); err != nil {
		return nil, fmt.Errorf("create machine api key: %w", err)
	}
	return key, nil
}

func (s *MachineIdentityService) RevokeAPIKey(ctx context.Context, poolUUID, machineUUID, keyUUID uuid.UUID) error {
	machine, err := s.GetMachine(ctx, poolUUID, machineUUID)
	if err != nil {
		return err
	}
	key, err := s.keys.GetByUUID(ctx, keyUUID)
	if err != nil {
		if errors.Is(err, ErrMachineAPIKeyNotFound) {
			return ErrMachineAPIKeyNotFound
		}
		return fmt.Errorf("get machine api key: %w", err)
	}
	if key.MachineUUID != machine.UUID {
		return ErrMachineAPIKeyNotFound
	}
	if err := s.keys.Revoke(ctx, keyUUID, time.Now().UTC()); err != nil {
		return fmt.Errorf("revoke machine api key: %w", err)
	}
	return nil
}

func newMachineAPIKeySecret() (prefix, secret, raw string, err error) {
	prefixBytes := make([]byte, 6)
	if _, err = randRead(prefixBytes); err != nil {
		return "", "", "", fmt.Errorf("generate machine api key prefix: %w", err)
	}
	secret, err = randomURLString(32)
	if err != nil {
		return "", "", "", err
	}
	prefix = model.MachineAPIKeyPrefix + base64.RawURLEncoding.EncodeToString(prefixBytes)
	raw = prefix + "." + secret
	return prefix, secret, raw, nil
}

func remapMachinePoolSlugError(err error) error {
	var ue *UserError
	if errors.As(err, &ue) {
		return userError(ErrInvalidMachinePool, "%s", ue.Message)
	}
	return err
}

func randRead(b []byte) (int, error) {
	return rand.Read(b)
}
