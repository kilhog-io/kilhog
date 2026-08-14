package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// HTTPMetrics records request counts and durations without database access.
type HTTPMetrics struct {
	requests metric.Int64Counter
	duration metric.Float64Histogram
}

func newHTTPMetrics(meter metric.Meter) (*HTTPMetrics, error) {
	requests, err := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("Number of HTTP server requests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		return nil, fmt.Errorf("http.server.request.count: %w", err)
	}

	duration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of HTTP server requests"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("http.server.request.duration: %w", err)
	}

	return &HTTPMetrics{requests: requests, duration: duration}, nil
}

// Middleware records HTTP request metrics. Scrapes of /metrics itself are skipped
// to keep the endpoint cheap for Prometheus.
func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	if m == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		capture := &statusCapture{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, r)

		attrs := metric.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", routeLabel(r)),
			attribute.Int("http.response.status_code", capture.status),
			attribute.String("http.response.status_class", statusClass(capture.status)),
		)
		m.requests.Add(r.Context(), 1, attrs)
		m.duration.Record(r.Context(), time.Since(start).Seconds(), attrs)
	})
}

func routeLabel(r *http.Request) string {
	// Prefer the pattern matched by ServeMux (Go 1.22+) when available.
	if pattern := r.Pattern; pattern != "" {
		return pattern
	}
	return r.URL.Path
}

func statusClass(code int) string {
	if code < 100 || code >= 600 {
		return "unknown"
	}
	return strconv.Itoa(code/100) + "xx"
}

type statusCapture struct {
	http.ResponseWriter
	status int
}

func (c *statusCapture) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}
