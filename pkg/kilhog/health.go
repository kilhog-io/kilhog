package kilhog

import "context"

// Health checks server availability via GET /healthz.
func (c *Client) Health(ctx context.Context) (*HealthStatus, error) {
	var status HealthStatus
	if err := c.do(ctx, httpMethodGet, "/healthz", nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
