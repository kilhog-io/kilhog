// Package metrics configures OpenTelemetry metrics exported in Prometheus
// format on GET /metrics. Functional gauges are backed by in-memory counters
// updated on create/delete so scrapes never hit the database.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

const scopeName = "github.com/kilhog-io/kilhog"

// Provider holds the OTel meter provider, Prometheus registry, and functional
// resource trackers used by the application.
type Provider struct {
	meterProvider *sdkmetric.MeterProvider
	registry      *prometheus.Registry
	handler       http.Handler
	Resources     *ResourceTracker
	HTTP          *HTTPMetrics
}

// Setup creates an OpenTelemetry MeterProvider backed by a Prometheus exporter,
// registers Go runtime metrics, and prepares functional / HTTP instruments.
func Setup(ctx context.Context) (*Provider, error) {
	reg := prometheus.NewRegistry()

	exporter, err := otelprometheus.New(
		otelprometheus.WithRegisterer(reg),
		otelprometheus.WithProducer(runtime.NewProducer()),
	)
	if err != nil {
		return nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("kilhog"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
		sdkmetric.WithResource(res),
	)

	if err := runtime.Start(
		runtime.WithMeterProvider(mp),
		runtime.WithMinimumReadMemStatsInterval(runtime.DefaultMinimumReadMemStatsInterval),
	); err != nil {
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("start runtime metrics: %w", err)
	}

	meter := mp.Meter(scopeName)

	resources, err := newResourceTracker(meter)
	if err != nil {
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("create resource metrics: %w", err)
	}

	httpMetrics, err := newHTTPMetrics(meter)
	if err != nil {
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("create http metrics: %w", err)
	}

	return &Provider{
		meterProvider: mp,
		registry:      reg,
		handler:       promhttp.HandlerFor(reg, promhttp.HandlerOpts{EnableOpenMetrics: true}),
		Resources:     resources,
		HTTP:          httpMetrics,
	}, nil
}

// Handler returns the Prometheus scrape handler for GET /metrics.
func (p *Provider) Handler() http.Handler {
	if p == nil || p.handler == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics unavailable", http.StatusServiceUnavailable)
		})
	}
	return p.handler
}

// Shutdown flushes and stops the meter provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.meterProvider == nil {
		return nil
	}
	if err := p.meterProvider.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown meter provider: %w", err)
	}
	return nil
}

// ResourceTracker keeps network/subnet counts in memory and exposes them as
// OTel observable gauges. Values are seeded at startup, updated on local
// mutations, and periodically reconciled from the database so replicas
// converge. Scrapes never query SQL.
type ResourceTracker struct {
	networks atomic.Int64
	subnets  atomic.Int64

	networkOps metric.Int64Counter
	subnetOps  metric.Int64Counter
}

func newResourceTracker(meter metric.Meter) (*ResourceTracker, error) {
	t := &ResourceTracker{}

	_, err := meter.Int64ObservableGauge(
		"kilhog.networks",
		metric.WithDescription("Number of networks currently stored"),
		metric.WithUnit("{network}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(t.networks.Load())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("kilhog.networks gauge: %w", err)
	}

	_, err = meter.Int64ObservableGauge(
		"kilhog.subnets",
		metric.WithDescription("Number of subnets currently stored"),
		metric.WithUnit("{subnet}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(t.subnets.Load())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("kilhog.subnets gauge: %w", err)
	}

	t.networkOps, err = meter.Int64Counter(
		"kilhog.network.operations",
		metric.WithDescription("Count of network create/update/delete operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("kilhog.network.operations counter: %w", err)
	}

	t.subnetOps, err = meter.Int64Counter(
		"kilhog.subnet.operations",
		metric.WithDescription("Count of subnet create/update/delete operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		return nil, fmt.Errorf("kilhog.subnet.operations counter: %w", err)
	}

	return t, nil
}

// Seed sets the initial network and subnet counts (typically once at startup).
func (t *ResourceTracker) Seed(networks, subnets int64) {
	if t == nil {
		return
	}
	t.networks.Store(networks)
	t.subnets.Store(subnets)
}

// NetworkCreated increments the in-memory network gauge and records a create op.
func (t *ResourceTracker) NetworkCreated(ctx context.Context) {
	if t == nil {
		return
	}
	t.networks.Add(1)
	t.networkOps.Add(ctx, 1, metric.WithAttributes(attrOperationCreate))
}

// NetworkUpdated records an update operation (count unchanged).
func (t *ResourceTracker) NetworkUpdated(ctx context.Context) {
	if t == nil {
		return
	}
	t.networkOps.Add(ctx, 1, metric.WithAttributes(attrOperationUpdate))
}

// NetworkDeleted decrements the in-memory network gauge and records a delete op.
func (t *ResourceTracker) NetworkDeleted(ctx context.Context) {
	if t == nil {
		return
	}
	t.networks.Add(-1)
	t.networkOps.Add(ctx, 1, metric.WithAttributes(attrOperationDelete))
}

// SubnetCreated increments the in-memory subnet gauge and records a create op.
func (t *ResourceTracker) SubnetCreated(ctx context.Context) {
	if t == nil {
		return
	}
	t.subnets.Add(1)
	t.subnetOps.Add(ctx, 1, metric.WithAttributes(attrOperationCreate))
}

// SubnetUpdated records an update operation (count unchanged).
func (t *ResourceTracker) SubnetUpdated(ctx context.Context) {
	if t == nil {
		return
	}
	t.subnetOps.Add(ctx, 1, metric.WithAttributes(attrOperationUpdate))
}

// SubnetDeleted decrements the in-memory subnet gauge and records a delete op.
func (t *ResourceTracker) SubnetDeleted(ctx context.Context) {
	if t == nil {
		return
	}
	t.subnets.Add(-1)
	t.subnetOps.Add(ctx, 1, metric.WithAttributes(attrOperationDelete))
}

// NetworkCount returns the in-memory network count (for tests).
func (t *ResourceTracker) NetworkCount() int64 {
	if t == nil {
		return 0
	}
	return t.networks.Load()
}

// SubnetCount returns the in-memory subnet count (for tests).
func (t *ResourceTracker) SubnetCount() int64 {
	if t == nil {
		return 0
	}
	return t.subnets.Load()
}
