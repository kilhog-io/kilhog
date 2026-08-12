package handler

import (
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

	router := NewRouter(Dependencies{
		NetworkService: svc,
		APIKey:         testKey,
		Metrics:        metricsProvider,
	})

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
	router := NewRouter(Dependencies{NetworkService: svc})

	t.Run("healthz stays public", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
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
