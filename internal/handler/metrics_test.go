package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kilhog-io/kilhog/internal/metrics"
	"github.com/kilhog-io/kilhog/internal/service"
)

func TestMetricsEndpoint(t *testing.T) {
	repos := openHandlerRepositories(t)
	metricsProvider, err := metrics.Setup(t.Context())
	if err != nil {
		t.Fatalf("metrics.Setup() error = %v", err)
	}
	t.Cleanup(func() {
		_ = metricsProvider.Shutdown(t.Context())
	})

	metricsProvider.Resources.Seed(1, 2)

	networkSvc := service.NewNetworkService(
		repos.Networks,
		repos.Subnets,
		service.WithNetworkMetrics(metricsProvider.Resources),
	)

	router := NewRouter(Dependencies{
		Store:          repos.Store,
		NetworkService: networkSvc,
		Metrics:        metricsProvider,
		APIKey:         "secret",
	})

	// Create a network so functional counters move without scraping SQL.
	reqCreate := httptest.NewRequest(http.MethodPost, "/networks", strings.NewReader(`{"name":"metrics-lab"}`))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Authorization", "Bearer secret")
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", recCreate.Code, recCreate.Body.String())
	}

	if got := metricsProvider.Resources.NetworkCount(); got != 2 {
		t.Fatalf("NetworkCount() = %d, want 2 (seeded 1 + created 1)", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)
	for _, want := range []string{"kilhog_networks", "kilhog_subnets", "go_goroutine_count"} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q\n%s", want, text)
		}
	}
}
