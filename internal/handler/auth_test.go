package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kilhog-io/kilhog/internal/metrics"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestAPIKeyMiddleware(t *testing.T) {
	repos := openHandlerRepositories(t)
	svc := service.NewNetworkService(repos.Networks, repos.Subnets)
	const testKey = "secret-test-key"

	metricsProvider, err := metrics.Setup(t.Context())
	if err != nil {
		t.Fatalf("metrics.Setup() error = %v", err)
	}
	t.Cleanup(func() {
		_ = metricsProvider.Shutdown(t.Context())
	})

	deps := authDepsFromRepos(repos, testKey)
	deps.NetworkService = svc
	deps.Metrics = metricsProvider
	router := NewRouter(deps)

	tests := []struct {
		name           string
		path           string
		headers        map[string]string
		wantStatusCode int
	}{
		{
			name:           "healthz without key",
			path:           "/healthz",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "metrics without key",
			path:           "/metrics",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "protected route without key",
			path:           "/networks",
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "protected route with invalid key",
			path:           "/networks",
			headers:        map[string]string{"Authorization": "Bearer wrong-key"},
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:           "protected route with bearer key",
			path:           "/networks",
			headers:        map[string]string{"Authorization": "Bearer " + testKey},
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "protected route with x-api-key header",
			path:           "/networks",
			headers:        map[string]string{"X-API-Key": testKey},
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatusCode {
				t.Fatalf("status code = %d, want %d, body = %s", rec.Code, tt.wantStatusCode, rec.Body.String())
			}
		})
	}
}

func TestAPIKeyMiddleware_ForbiddenWhenAPIKeyUnset(t *testing.T) {
	repos := openHandlerRepositories(t)
	svc := service.NewNetworkService(repos.Networks, repos.Subnets)
	deps := authDepsFromRepos(repos, "")
	deps.NetworkService = svc
	router := NewRouter(deps)

	t.Run("healthz stays public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("metrics stays public", func(t *testing.T) {
		metricsProvider, err := metrics.Setup(t.Context())
		if err != nil {
			t.Fatalf("metrics.Setup() error = %v", err)
		}
		t.Cleanup(func() {
			_ = metricsProvider.Shutdown(t.Context())
		})
		deps.Metrics = metricsProvider
		router := NewRouter(deps)

		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("functional route returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/networks", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status code = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})
}

func TestBootstrapAndLocalLogin(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	deps.NetworkService = service.NewNetworkService(repos.Networks, repos.Subnets)
	router := NewRouter(deps)

	bootstrapBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(bootstrapBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, want 201, body = %s", rec.Code, rec.Body.String())
	}

	var bootstrapResp struct {
		Data struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bootstrapResp); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	if bootstrapResp.Data.Session.Token == "" {
		t.Fatal("expected session token")
	}

	req = httptest.NewRequest(http.MethodGet, "/networks", nil)
	req.Header.Set("Authorization", "Bearer "+bootstrapResp.Data.Session.Token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("networks with session status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	// Second bootstrap must fail.
	req = httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(bootstrapBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second bootstrap status = %d, want 409", rec.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password123",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAdminIdentityPoolCRUD(t *testing.T) {
	repos := openHandlerRepositories(t)
	deps := authDepsFromRepos(repos, "")
	router := NewRouter(deps)

	bootstrapBody, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/bootstrap", bytes.NewReader(bootstrapBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("bootstrap status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var bootstrapResp struct {
		Data struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &bootstrapResp)
	token := bootstrapResp.Data.Session.Token

	poolBody, _ := json.Marshal(map[string]any{
		"name":      "Corp SSO",
		"slug":      "corp",
		"issuer":    "https://idp.example.com",
		"client_id": "kilhog",
	})
	req = httptest.NewRequest(http.MethodPost, "/auth/identity-pools", bytes.NewReader(poolBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create pool status = %d, body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/auth/oidc/pools", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public pools status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
