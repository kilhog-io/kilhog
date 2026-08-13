package metrics_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kilhog-io/kilhog/internal/metrics"
)

func TestSetupExposesRuntimeAndFunctionalMetrics(t *testing.T) {
	ctx := context.Background()
	provider, err := metrics.Setup(ctx)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
	})

	provider.Resources.Seed(2, 5)
	provider.Resources.NetworkCreated(ctx)
	provider.Resources.SubnetDeleted(ctx)

	if got := provider.Resources.NetworkCount(); got != 3 {
		t.Fatalf("NetworkCount() = %d, want 3", got)
	}
	if got := provider.Resources.SubnetCount(); got != 4 {
		t.Fatalf("SubnetCount() = %d, want 4", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	provider.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	for _, want := range []string{
		"kilhog_networks",
		"kilhog_subnets",
		"go_goroutine_count",
		"kilhog_network_operations_total",
		"kilhog_subnet_operations_total",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics body missing %q\n%s", want, text)
		}
	}

	if !strings.Contains(text, `kilhog_networks{`) && !strings.Contains(text, "kilhog_networks ") {
		// Accept either labeled or unlabeled gauge sample lines.
		if !strings.Contains(text, "kilhog_networks") {
			t.Fatalf("expected kilhog_networks sample, body:\n%s", text)
		}
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	ctx := context.Background()
	provider, err := metrics.Setup(ctx)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := provider.HTTP.Middleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	provider.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, "http_server_request_count_total") && !strings.Contains(body, "http_server_request_count") {
		t.Fatalf("expected http request count metric, body:\n%s", body)
	}
	if !strings.Contains(body, "http_server_request_duration") {
		t.Fatalf("expected http request duration metric, body:\n%s", body)
	}
}
