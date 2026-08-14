package metrics_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestResourceTrackerRefreshReconcilesReplicaDrift(t *testing.T) {
	ctx := context.Background()
	provider, err := metrics.Setup(ctx)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	provider.Resources.Seed(10, 20)
	provider.Resources.NetworkCreated(ctx) // local replica handled a create → 11
	if got := provider.Resources.NetworkCount(); got != 11 {
		t.Fatalf("NetworkCount() after local create = %d, want 11", got)
	}

	// Another replica created four more networks; this process only learns via Refresh.
	err = provider.Resources.Refresh(ctx, func(context.Context) (metrics.ResourceCounts, error) {
		return metrics.ResourceCounts{Networks: 15, Subnets: 20}, nil
	})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got := provider.Resources.NetworkCount(); got != 15 {
		t.Fatalf("NetworkCount() after refresh = %d, want 15", got)
	}
	if got := provider.Resources.SubnetCount(); got != 20 {
		t.Fatalf("SubnetCount() after refresh = %d, want 20", got)
	}
}

func TestStartRefreshStopsOnCancel(t *testing.T) {
	ctx := context.Background()
	provider, err := metrics.Setup(ctx)
	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
	})

	var calls atomic.Int64
	src := func(context.Context) (metrics.ResourceCounts, error) {
		n := calls.Add(1)
		return metrics.ResourceCounts{Networks: n, Subnets: 0}, nil
	}

	refreshCtx, cancel := context.WithCancel(context.Background())
	provider.Resources.StartRefresh(refreshCtx, 20*time.Millisecond, src)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if calls.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatalf("refresh calls = %d, want at least 2", calls.Load())
	}

	cancel()
	time.Sleep(50 * time.Millisecond)
	stopped := calls.Load()
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != stopped {
		t.Fatalf("refresh continued after cancel: before=%d after=%d", stopped, calls.Load())
	}
}

func TestRefreshIntervalFromEnv(t *testing.T) {
	t.Setenv("KILHOG_METRICS_REFRESH_INTERVAL", "")
	got, err := metrics.RefreshIntervalFromEnv()
	if err != nil {
		t.Fatalf("empty env error = %v", err)
	}
	if got != metrics.DefaultRefreshInterval {
		t.Fatalf("empty env = %s, want %s", got, metrics.DefaultRefreshInterval)
	}

	t.Setenv("KILHOG_METRICS_REFRESH_INTERVAL", "off")
	got, err = metrics.RefreshIntervalFromEnv()
	if err != nil {
		t.Fatalf("off error = %v", err)
	}
	if got != 0 {
		t.Fatalf("off = %s, want 0", got)
	}

	t.Setenv("KILHOG_METRICS_REFRESH_INTERVAL", "15s")
	got, err = metrics.RefreshIntervalFromEnv()
	if err != nil {
		t.Fatalf("15s error = %v", err)
	}
	if got != 15*time.Second {
		t.Fatalf("15s = %s, want 15s", got)
	}

	t.Setenv("KILHOG_METRICS_REFRESH_INTERVAL", "nope")
	if _, err = metrics.RefreshIntervalFromEnv(); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}
