package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// DefaultRefreshInterval is how often cluster-wide IPAM gauges are reconciled
// from the database. Scrapes still read in-memory values only.
const DefaultRefreshInterval = 30 * time.Second

// ResourceCounts is a point-in-time snapshot of persisted IPAM resources.
type ResourceCounts struct {
	Networks int64
	Subnets  int64
}

// ResourceCountSource loads network and subnet totals from persistence.
// Used at startup and on the background refresh loop — never on GET /metrics.
type ResourceCountSource func(ctx context.Context) (ResourceCounts, error)

// RefreshIntervalFromEnv reads KILHOG_METRICS_REFRESH_INTERVAL.
// Empty uses DefaultRefreshInterval. "0" or "off" disables the background loop.
func RefreshIntervalFromEnv() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("KILHOG_METRICS_REFRESH_INTERVAL"))
	if raw == "" {
		return DefaultRefreshInterval, nil
	}
	if raw == "0" || strings.EqualFold(raw, "off") {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid KILHOG_METRICS_REFRESH_INTERVAL %q: %w", raw, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("KILHOG_METRICS_REFRESH_INTERVAL must be >= 0")
	}
	return d, nil
}

// Refresh overwrites in-memory gauges from persistence so replicas converge
// after mutations handled by other instances.
func (t *ResourceTracker) Refresh(ctx context.Context, src ResourceCountSource) error {
	if t == nil || src == nil {
		return nil
	}
	counts, err := src(ctx)
	if err != nil {
		return fmt.Errorf("load resource counts: %w", err)
	}
	t.Seed(counts.Networks, counts.Subnets)
	return nil
}

// StartRefresh periodically reconciles gauges from src until ctx is cancelled.
// A non-positive interval disables the loop. The first tick waits for interval
// (call Refresh once at startup before this).
func (t *ResourceTracker) StartRefresh(ctx context.Context, interval time.Duration, src ResourceCountSource) {
	if t == nil || src == nil || interval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := t.Refresh(ctx, src); err != nil {
					slog.Warn("metrics refresh failed", "error", err)
					continue
				}
				slog.Debug("metrics refreshed",
					"networks", t.NetworkCount(),
					"subnets", t.SubnetCount(),
				)
			}
		}
	}()
}
