package service

import "context"

// ResourceMetrics tracks IPAM resource counts and mutation operations.
// Implementations must not query the database on metric scrape; counts are
// kept in memory and updated only when create/delete succeeds.
type ResourceMetrics interface {
	NetworkCreated(ctx context.Context)
	NetworkUpdated(ctx context.Context)
	NetworkDeleted(ctx context.Context)
	SubnetCreated(ctx context.Context)
	SubnetUpdated(ctx context.Context)
	SubnetDeleted(ctx context.Context)
}
