//go:build !(js && wasm)

package d1

import (
	"context"
	"fmt"

	"github.com/kilhog-io/kilhog/internal/repository/db"
)

// Open is unavailable outside the Cloudflare Workers WASM runtime.
func Open(ctx context.Context, dsn string) (*db.Store, error) {
	_ = ctx
	_ = dsn
	return nil, fmt.Errorf("d1 driver is only available when building for Cloudflare Workers (GOOS=js GOARCH=wasm)")
}
