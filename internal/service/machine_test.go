package service_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/google/uuid"
	"github.com/kilhog-io/kilhog/internal/model"
	"github.com/kilhog-io/kilhog/internal/repository"
	"github.com/kilhog-io/kilhog/internal/repository/db"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestMachineMatchesJWT(t *testing.T) {
	providerID := uuid.New()
	machine := &model.Machine{
		ProviderUUID:  &providerID,
		SubjectPrefix: "repo:org/app:",
		Claims: []model.ClaimCondition{
			{Claim: "repository", Op: model.ClaimOpEq, Value: "org/app"},
			{Claim: "ref", Op: model.ClaimOpPrefix, Value: "refs/heads/"},
			{Claim: "environment", Op: model.ClaimOpIn, Value: []string{"prod", "staging"}},
		},
	}
	claims := map[string]any{
		"sub":         "repo:org/app:ref:refs/heads/main",
		"repository":  "org/app",
		"ref":         "refs/heads/main",
		"environment": "prod",
	}
	if !service.MachineMatchesJWT(machine, claims) {
		t.Fatal("expected match")
	}
	claims["environment"] = "dev"
	if service.MachineMatchesJWT(machine, claims) {
		t.Fatal("expected environment mismatch")
	}
}

func TestMachineAPIKeyAndJWTAuth(t *testing.T) {
	ctx := context.Background()
	svc, auth, _ := openMachineAuth(t, "")

	pool, err := svc.CreatePool(ctx, service.CreateMachinePoolInput{Name: "ci", Slug: "ci"})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}

	keyPair := mustGenerateRSA(t)
	jwks := publicJWKS(t, keyPair, "kid-1")
	provider, err := svc.CreateProvider(ctx, pool.UUID, service.CreateMachineProviderInput{
		Name:      "github",
		Issuer:    "https://token.actions.example",
		Audiences: []string{"https://kilhog.example"},
		JWKSMode:  model.JWKSModeStatic,
		JWKS:      jwks,
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	machine, err := svc.CreateMachine(ctx, pool.UUID, service.CreateMachineInput{
		Name:          "deploy",
		ProviderUUID:  &provider.UUID,
		SubjectPrefix: "repo:org/app:",
		Claims:        []model.ClaimCondition{{Claim: "repository", Op: model.ClaimOpEq, Value: "org/app"}},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	createdKey, err := svc.CreateAPIKey(ctx, pool.UUID, machine.UUID, service.CreateMachineAPIKeyInput{Name: "ci-1"})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if createdKey.Secret == "" || !strings.HasPrefix(createdKey.Secret, model.MachineAPIKeyPrefix) {
		t.Fatalf("expected secret once, got %q", createdKey.Secret)
	}

	principal, err := auth.AuthenticateRequest(ctx, createdKey.Secret, "", "")
	if err != nil {
		t.Fatalf("AuthenticateRequest api key: %v", err)
	}
	if principal.Kind != model.PrincipalKindMachine || principal.AuthMethod != model.AuthMethodAPIKey {
		t.Fatalf("principal = %+v", principal)
	}
	if principal.MachineName != "deploy" {
		t.Fatalf("machine name = %q", principal.MachineName)
	}

	rawJWT := mustSignJWT(t, keyPair, "kid-1", josejwt.Claims{
		Issuer:   "https://token.actions.example",
		Subject:  "repo:org/app:ref:refs/heads/main",
		Audience: josejwt.Audience{"https://kilhog.example"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, map[string]any{"repository": "org/app"})

	principal, err = auth.AuthenticateRequest(ctx, "", rawJWT, "")
	if err != nil {
		t.Fatalf("AuthenticateRequest jwt: %v", err)
	}
	if principal.Kind != model.PrincipalKindMachine || principal.AuthMethod != model.AuthMethodJWT {
		t.Fatalf("jwt principal = %+v", principal)
	}

	badAud := mustSignJWT(t, keyPair, "kid-1", josejwt.Claims{
		Issuer:   "https://token.actions.example",
		Subject:  "repo:org/app:ref:refs/heads/main",
		Audience: josejwt.Audience{"https://other.example"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, map[string]any{"repository": "org/app"})
	if _, err := auth.AuthenticateRequest(ctx, "", badAud, ""); err == nil {
		t.Fatal("expected wrong audience to fail")
	}

	if err := svc.RevokeAPIKey(ctx, pool.UUID, machine.UUID, createdKey.UUID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := auth.AuthenticateRequest(ctx, createdKey.Secret, "", ""); err == nil {
		t.Fatal("expected revoked key to fail")
	}
}

func TestMachineJWTDiscoveryAndURI(t *testing.T) {
	keyPair := mustGenerateRSA(t)
	jwks := publicJWKS(t, keyPair, "kid-2")

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"jwks_uri": "https://" + r.Host + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	ctx := context.Background()
	svc, auth, _ := openMachineAuth(t, "")
	svc.SetHTTPClient(server.Client())

	pool, err := svc.CreatePool(ctx, service.CreateMachinePoolInput{Name: "wif", Slug: "wif"})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	issuer := strings.TrimRight(server.URL, "/")
	provider, err := svc.CreateProvider(ctx, pool.UUID, service.CreateMachineProviderInput{
		Name:      "idp",
		Issuer:    issuer,
		Audiences: []string{"kilhog"},
		JWKSMode:  model.JWKSModeDiscovery,
	})
	if err != nil {
		t.Fatalf("CreateProvider discovery: %v", err)
	}
	_, err = svc.CreateMachine(ctx, pool.UUID, service.CreateMachineInput{
		Name:         "job",
		ProviderUUID: &provider.UUID,
		Subject:      "machine-1",
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	rawJWT := mustSignJWT(t, keyPair, "kid-2", josejwt.Claims{
		Issuer:   issuer,
		Subject:  "machine-1",
		Audience: josejwt.Audience{"kilhog"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, nil)
	if _, err := auth.AuthenticateRequest(ctx, "", rawJWT, ""); err != nil {
		t.Fatalf("discovery jwt: %v", err)
	}

	uriProvider, err := svc.CreateProvider(ctx, pool.UUID, service.CreateMachineProviderInput{
		Name:      "idp-uri",
		Issuer:    issuer + "/uri",
		Audiences: []string{"kilhog"},
		JWKSMode:  model.JWKSModeURI,
		JWKSURI:   issuer + "/keys",
	})
	if err != nil {
		t.Fatalf("CreateProvider uri: %v", err)
	}
	_, err = svc.CreateMachine(ctx, pool.UUID, service.CreateMachineInput{
		Name:         "job-uri",
		ProviderUUID: &uriProvider.UUID,
		Subject:      "machine-uri",
	})
	if err != nil {
		t.Fatalf("CreateMachine uri: %v", err)
	}
	uriJWT := mustSignJWT(t, keyPair, "kid-2", josejwt.Claims{
		Issuer:   issuer + "/uri",
		Subject:  "machine-uri",
		Audience: josejwt.Audience{"kilhog"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, nil)
	if _, err := auth.AuthenticateRequest(ctx, "", uriJWT, ""); err != nil {
		t.Fatalf("uri jwt: %v", err)
	}
}

func TestMachineJWTAmbiguousMatch(t *testing.T) {
	ctx := context.Background()
	svc, auth, _ := openMachineAuth(t, "")
	keyPair := mustGenerateRSA(t)
	jwks := publicJWKS(t, keyPair, "kid-3")

	pool, err := svc.CreatePool(ctx, service.CreateMachinePoolInput{Name: "dup", Slug: "dup"})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	provider, err := svc.CreateProvider(ctx, pool.UUID, service.CreateMachineProviderInput{
		Name:      "idp",
		Issuer:    "https://issuer.example",
		Audiences: []string{"aud"},
		JWKSMode:  model.JWKSModeStatic,
		JWKS:      jwks,
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if _, err := svc.CreateMachine(ctx, pool.UUID, service.CreateMachineInput{
			Name:         name,
			ProviderUUID: &provider.UUID,
			Subject:      "same-sub",
		}); err != nil {
			t.Fatalf("CreateMachine %s: %v", name, err)
		}
	}
	rawJWT := mustSignJWT(t, keyPair, "kid-3", josejwt.Claims{
		Issuer:   "https://issuer.example",
		Subject:  "same-sub",
		Audience: josejwt.Audience{"aud"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, nil)
	if _, err := auth.AuthenticateRequest(ctx, "", rawJWT, ""); err == nil {
		t.Fatal("expected ambiguous jwt match to fail")
	}
}

func TestEnabledMachinePoolConfiguresAuth(t *testing.T) {
	svc, auth, _ := openMachineAuth(t, "")
	if _, err := svc.CreatePool(context.Background(), service.CreateMachinePoolInput{Name: "only", Slug: "only"}); err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	status, err := auth.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Configured || status.EnabledMachinePools != 1 {
		t.Fatalf("status = %+v", status)
	}
	if _, err := auth.AuthenticateRequest(context.Background(), "", "", ""); err == nil {
		t.Fatal("expected unauthenticated")
	} else if err != service.ErrUnauthenticated {
		t.Fatalf("err = %v, want unauthenticated", err)
	}
}

func TestMachineRequiresJWTConstraint(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := openMachineAuth(t, "")
	pool, err := svc.CreatePool(ctx, service.CreateMachinePoolInput{Name: "p", Slug: "p"})
	if err != nil {
		t.Fatalf("CreatePool: %v", err)
	}
	jwks := publicJWKS(t, mustGenerateRSA(t), "k")
	provider, err := svc.CreateProvider(ctx, pool.UUID, service.CreateMachineProviderInput{
		Name:      "idp",
		Issuer:    "https://issuer.example",
		Audiences: []string{"aud"},
		JWKSMode:  model.JWKSModeStatic,
		JWKS:      jwks,
	})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if _, err := svc.CreateMachine(ctx, pool.UUID, service.CreateMachineInput{
		Name:         "open",
		ProviderUUID: &provider.UUID,
	}); err == nil {
		t.Fatal("expected jwt machine without constraints to fail")
	}
}

func openMachineAuth(t *testing.T, apiKey string) (*service.MachineIdentityService, *service.AuthService, *repository.Repositories) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "kilhog.db")
	repos, err := repository.Open(context.Background(), db.Config{
		Driver:      db.DialectSQLite,
		DSN:         "file:" + dbPath + "?_pragma=foreign_keys(ON)",
		AutoMigrate: true,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repos.Close() })

	machines := service.NewMachineIdentityService(
		repos.MachinePools,
		repos.MachineProviders,
		repos.Machines,
		repos.MachineAPIKeys,
		nil,
	)
	auth := service.NewAuthService(repos.Users, repos.IdentityPools, repos.Sessions, repos.OIDCStates, machines, service.AuthConfig{APIKey: apiKey})
	return machines, auth, repos
}

func mustGenerateRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return key
}

func publicJWKS(t *testing.T, key *rsa.PrivateKey, kid string) []byte {
	t.Helper()
	set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       &key.PublicKey,
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}}}
	raw, err := json.Marshal(set)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return raw
}

func mustSignJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims josejwt.Claims, extra map[string]any) string {
	t.Helper()
	opts := (&jose.SignerOptions{}).WithType("JWT")
	opts.WithHeader("kid", kid)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, opts)
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}
	builder := josejwt.Signed(signer).Claims(claims)
	if extra != nil {
		builder = builder.Claims(extra)
	}
	raw, err := builder.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return raw
}
