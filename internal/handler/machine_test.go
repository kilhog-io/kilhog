package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestMachineIdentityAdminAndAuth(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	deps.NetworkService = service.NewNetworkService(repos.Networks, repos.Subnets)
	router := NewRouter(deps)

	adminToken := bootstrapAdmin(t, router)

	poolBody, _ := json.Marshal(map[string]any{
		"name": "github-ci",
		"slug": "github-ci",
	})
	pool := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/machine-pools", adminToken, poolBody, http.StatusCreated))
	poolUUID := pool["uuid"].(string)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	jwks, _ := json.Marshal(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       &key.PublicKey,
		KeyID:     "kid-h",
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}}})
	providerBody, _ := json.Marshal(map[string]any{
		"name":      "github",
		"issuer":    "https://token.actions.example",
		"audiences": []string{"https://kilhog.example"},
		"jwks_mode": "static",
		"jwks":      json.RawMessage(jwks),
	})
	provider := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/machine-pools/"+poolUUID+"/providers", adminToken, providerBody, http.StatusCreated))
	providerUUID := provider["uuid"].(string)

	machineBody, _ := json.Marshal(map[string]any{
		"name":           "deploy",
		"provider_uuid":  providerUUID,
		"subject_prefix": "repo:org/app:",
		"claims":         []map[string]any{{"claim": "repository", "op": "eq", "value": "org/app"}},
	})
	machine := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/machine-pools/"+poolUUID+"/machines", adminToken, machineBody, http.StatusCreated))
	machineUUID := machine["uuid"].(string)

	keyResp := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/machine-pools/"+poolUUID+"/machines/"+machineUUID+"/api-keys", adminToken, []byte(`{"name":"ci"}`), http.StatusCreated))
	secret, _ := keyResp["secret"].(string)
	if secret == "" {
		t.Fatal("expected api key secret")
	}

	req := httptest.NewRequest(http.MethodGet, "/networks", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("networks with machine key status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/machine-pools", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("machine admin access status = %d, want 403", rec.Code)
	}

	rawJWT := signTestJWT(t, key, "kid-h", josejwt.Claims{
		Issuer:   "https://token.actions.example",
		Subject:  "repo:org/app:ref:refs/heads/main",
		Audience: josejwt.Audience{"https://kilhog.example"},
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: josejwt.NewNumericDate(time.Now()),
	}, map[string]any{"repository": "org/app"})
	req = httptest.NewRequest(http.MethodGet, "/networks", nil)
	req.Header.Set("Authorization", "Bearer "+rawJWT)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("networks with machine jwt status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawJWT)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestNonAdminCannotManageMachines(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	router := NewRouter(deps)
	adminToken := bootstrapAdmin(t, router)

	userBody, _ := json.Marshal(map[string]any{
		"username": "alice",
		"password": "password123",
		"role":     "user",
	})
	doJSON(t, router, http.MethodPost, "/users", adminToken, userBody, http.StatusCreated)

	loginBody, _ := json.Marshal(map[string]string{"username": "alice", "password": "password123"})
	login := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/login", "", loginBody, http.StatusOK))
	session := login["session"].(map[string]any)
	userToken := session["token"].(string)

	poolBody, _ := json.Marshal(map[string]any{"name": "ci", "slug": "ci"})
	doJSON(t, router, http.MethodPost, "/auth/machine-pools", userToken, poolBody, http.StatusForbidden)
}

func bootstrapAdmin(t *testing.T, router http.Handler) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "password123"})
	resp := decodeCreated[map[string]any](t, doJSON(t, router, http.MethodPost, "/auth/bootstrap", "", body, http.StatusCreated))
	session := resp["session"].(map[string]any)
	token, _ := session["token"].(string)
	if token == "" {
		t.Fatal("expected bootstrap session")
	}
	return token
}

func doJSON(t *testing.T, router http.Handler, method, path, token string, body []byte, want int) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s status = %d, want %d, body = %s", method, path, rec.Code, want, rec.Body.String())
	}
	return rec
}

func decodeCreated[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	return envelope.Data
}

func signTestJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims josejwt.Claims, extra map[string]any) string {
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
