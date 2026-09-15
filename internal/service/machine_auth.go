package service

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
)

var machineJWTAlgs = []jose.SignatureAlgorithm{
	jose.RS256, jose.RS384, jose.RS512,
	jose.PS256, jose.PS384, jose.PS512,
	jose.ES256, jose.ES384, jose.ES512,
	jose.EdDSA,
}

func LooksLikeMachineAPIKey(raw string) bool {
	return strings.HasPrefix(raw, model.MachineAPIKeyPrefix) && strings.Count(raw, ".") == 1
}

func (s *MachineIdentityService) AuthenticateAPIKey(ctx context.Context, raw string) (*Principal, error) {
	raw = strings.TrimSpace(raw)
	if !LooksLikeMachineAPIKey(raw) {
		return nil, ErrUnauthenticated
	}
	prefix, _, ok := strings.Cut(raw, ".")
	if !ok || prefix == "" {
		return nil, ErrUnauthenticated
	}
	key, err := s.keys.GetByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, ErrMachineAPIKeyNotFound) {
			return nil, ErrUnauthenticated
		}
		return nil, fmt.Errorf("lookup machine api key: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(key.TokenHash), []byte(hashToken(raw))) != 1 {
		return nil, ErrUnauthenticated
	}
	now := time.Now().UTC()
	if key.RevokedAt != nil {
		return nil, ErrUnauthenticated
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(now) {
		return nil, ErrUnauthenticated
	}
	machine, err := s.machines.GetByUUID(ctx, key.MachineUUID)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	if !machine.Enabled {
		return nil, ErrUnauthenticated
	}
	pool, err := s.pools.GetByUUID(ctx, machine.MachinePoolUUID)
	if err != nil || !pool.Enabled {
		return nil, ErrUnauthenticated
	}
	_ = s.keys.TouchLastUsed(ctx, key.UUID, now)
	return machinePrincipal(machine, model.AuthMethodAPIKey, "", ""), nil
}

func (s *MachineIdentityService) AuthenticateJWT(ctx context.Context, raw string) (*Principal, error) {
	issuer, err := unverifiedJWTIssuer(raw)
	if err != nil || issuer == "" {
		return nil, ErrUnauthenticated
	}
	providers, err := s.providers.ListEnabledByIssuer(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("list machine providers by issuer: %w", err)
	}
	if len(providers) == 0 {
		return nil, ErrUnauthenticated
	}

	var matches []*model.Machine
	var matchedIssuer, matchedSubject string
	for _, provider := range providers {
		claims, ok, err := s.verifyMachineJWT(ctx, provider, raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		machines, err := s.machines.ListEnabledByProvider(ctx, provider.UUID)
		if err != nil {
			return nil, fmt.Errorf("list machines for provider: %w", err)
		}
		for _, machine := range machines {
			if machineMatchesJWT(machine, claims) {
				matches = append(matches, machine)
				matchedIssuer = claimString(claims["iss"])
				matchedSubject = claimString(claims["sub"])
			}
		}
	}
	if len(matches) != 1 {
		return nil, ErrUnauthenticated
	}
	return machinePrincipal(matches[0], model.AuthMethodJWT, matchedIssuer, matchedSubject), nil
}

func machinePrincipal(machine *model.Machine, method, issuer, subject string) *Principal {
	poolUUID := machine.MachinePoolUUID
	machineUUID := machine.UUID
	return &Principal{
		Kind:            model.PrincipalKindMachine,
		MachinePoolUUID: &poolUUID,
		MachineUUID:     &machineUUID,
		MachineName:     machine.Name,
		AuthMethod:      method,
		Issuer:          issuer,
		Subject:         subject,
	}
}

func (s *MachineIdentityService) verifyMachineJWT(ctx context.Context, provider *model.MachineIdentityProvider, raw string) (map[string]any, bool, error) {
	parsed, err := josejwt.ParseSigned(raw, machineJWTAlgs)
	if err != nil {
		return nil, false, nil
	}
	kid := ""
	if len(parsed.Headers) > 0 {
		kid = parsed.Headers[0].KeyID
	}
	set, err := s.keysForProvider(ctx, provider, false)
	if err != nil {
		return nil, false, nil
	}
	key, ok := findJWK(set, kid)
	if !ok {
		set, err = s.keysForProvider(ctx, provider, true)
		if err != nil {
			return nil, false, nil
		}
		key, ok = findJWK(set, kid)
		if !ok {
			return nil, false, nil
		}
	}
	claims := map[string]any{}
	if err := parsed.Claims(key.Key, &claims); err != nil {
		return nil, false, nil
	}
	if err := validateJWTTimeAndIssuer(claims, provider.Issuer, time.Now().UTC()); err != nil {
		return nil, false, nil
	}
	if !audienceMatches(claims["aud"], provider.Audiences) {
		return nil, false, nil
	}
	return claims, true, nil
}

func validateJWTTimeAndIssuer(claims map[string]any, issuer string, now time.Time) error {
	if claimString(claims["iss"]) != issuer {
		return ErrUnauthenticated
	}
	if exp, ok := jwtNumericDate(claims["exp"]); ok && !now.Before(exp) {
		return ErrUnauthenticated
	}
	if nbf, ok := jwtNumericDate(claims["nbf"]); ok && now.Before(nbf) {
		return ErrUnauthenticated
	}
	return nil
}

func jwtNumericDate(raw any) (time.Time, bool) {
	switch v := raw.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC(), true
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.Unix(n, 0).UTC(), true
	case int64:
		return time.Unix(v, 0).UTC(), true
	default:
		return time.Time{}, false
	}
}

func audienceMatches(raw any, allowed []string) bool {
	tokenAudiences := jwtAudienceList(raw)
	if len(tokenAudiences) == 0 {
		return false
	}
	allowedSet := map[string]struct{}{}
	for _, aud := range allowed {
		allowedSet[aud] = struct{}{}
	}
	for _, aud := range tokenAudiences {
		if _, ok := allowedSet[aud]; ok {
			return true
		}
	}
	return false
}

func jwtAudienceList(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func findJWK(set *jose.JSONWebKeySet, kid string) (jose.JSONWebKey, bool) {
	if set == nil {
		return jose.JSONWebKey{}, false
	}
	if kid != "" {
		keys := set.Key(kid)
		if len(keys) > 0 {
			return keys[0], true
		}
		return jose.JSONWebKey{}, false
	}
	if len(set.Keys) == 1 {
		return set.Keys[0], true
	}
	return jose.JSONWebKey{}, false
}

func (s *MachineIdentityService) keysForProvider(ctx context.Context, provider *model.MachineIdentityProvider, force bool) (*jose.JSONWebKeySet, error) {
	cacheKey := provider.UUID.String()
	if !force {
		s.jwksMu.Lock()
		entry, ok := s.jwksCache[cacheKey]
		s.jwksMu.Unlock()
		if ok && time.Since(entry.fetchedAt) < jwksCacheTTL {
			return entry.set, nil
		}
	}

	var (
		set *jose.JSONWebKeySet
		err error
	)
	switch provider.JWKSMode {
	case model.JWKSModeStatic:
		set, err = parseJWKS(provider.JWKS)
	case model.JWKSModeURI:
		set, err = s.fetchJWKS(ctx, provider.JWKSURI)
	case model.JWKSModeDiscovery:
		jwksURI, discErr := s.discoverJWKSURI(ctx, provider.Issuer)
		if discErr != nil {
			return nil, discErr
		}
		set, err = s.fetchJWKS(ctx, jwksURI)
	default:
		return nil, fmt.Errorf("unsupported jwks mode %q", provider.JWKSMode)
	}
	if err != nil {
		return nil, err
	}
	s.jwksMu.Lock()
	s.jwksCache[cacheKey] = jwksCacheEntry{set: set, fetchedAt: time.Now()}
	s.jwksMu.Unlock()
	return set, nil
}

func (s *MachineIdentityService) invalidateJWKS(id uuid.UUID) {
	s.jwksMu.Lock()
	delete(s.jwksCache, id.String())
	s.jwksMu.Unlock()
}

func (s *MachineIdentityService) discoverJWKSURI(ctx context.Context, issuer string) (string, error) {
	endpoint := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build discovery request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch openid configuration: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openid configuration status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read openid configuration: %w", err)
	}
	var doc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("parse openid configuration: %w", err)
	}
	if strings.TrimSpace(doc.JWKSURI) == "" {
		return "", fmt.Errorf("openid configuration missing jwks_uri")
	}
	return strings.TrimSpace(doc.JWKSURI), nil
}

func (s *MachineIdentityService) fetchJWKS(ctx context.Context, jwksURI string) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, fmt.Errorf("build jwks request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read jwks: %w", err)
	}
	return parseJWKS(body)
}

func parseJWKS(raw []byte) (*jose.JSONWebKeySet, error) {
	var set jose.JSONWebKeySet
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("parse jwks: %w", err)
	}
	if len(set.Keys) == 0 {
		return nil, fmt.Errorf("jwks contains no keys")
	}
	return &set, nil
}

func unverifiedJWTIssuer(raw string) (string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", ErrUnauthenticated
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", ErrUnauthenticated
		}
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", ErrUnauthenticated
	}
	return strings.TrimSpace(claims.Issuer), nil
}

func MachineMatchesJWT(machine *model.Machine, claims map[string]any) bool {
	return machineMatchesJWT(machine, claims)
}

func machineMatchesJWT(machine *model.Machine, claims map[string]any) bool {
	if machine == nil || machine.ProviderUUID == nil {
		return false
	}
	sub := claimString(claims["sub"])
	if machine.Subject != "" && sub != machine.Subject {
		return false
	}
	if machine.SubjectPrefix != "" && !strings.HasPrefix(sub, machine.SubjectPrefix) {
		return false
	}
	for _, cond := range machine.Claims {
		if !claimConditionMatches(cond, claims) {
			return false
		}
	}
	return true
}

func claimConditionMatches(cond model.ClaimCondition, claims map[string]any) bool {
	raw, ok := claims[cond.Claim]
	if !ok {
		return false
	}
	value := claimString(raw)
	switch cond.Op {
	case model.ClaimOpEq:
		expected, ok := cond.Value.(string)
		return ok && value == expected
	case model.ClaimOpPrefix:
		expected, ok := cond.Value.(string)
		return ok && strings.HasPrefix(value, expected)
	case model.ClaimOpIn:
		list, ok := claimValueStringList(cond.Value)
		if !ok {
			return false
		}
		for _, expected := range list {
			if value == expected {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func claimString(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}
