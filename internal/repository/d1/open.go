//go:build js && wasm

package d1

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kilhog-io/kilhog/internal/repository/db"

	_ "github.com/syumai/workers/cloudflare/d1" // register database/sql driver "d1"
)

// Open connects to a Cloudflare D1 binding via the syumai/workers D1 driver.
// dsn is the Wrangler binding name (e.g. "DB"), not a file path.
func Open(ctx context.Context, dsn string) (*db.Store, error) {
	binding := strings.TrimSpace(dsn)
	if binding == "" {
		return nil, fmt.Errorf("d1 binding name is required (set KILHOG_DB_DSN to the Wrangler binding, e.g. DB)")
	}

	sqlDB, err := sql.Open("d1", binding)
	if err != nil {
		return nil, fmt.Errorf("open d1 database %q: %w", binding, err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Do not Ping here: the D1 binding is resolved from the Workers runtime
	// context, which is only available during request handling. The first query
	// (e.g. migrations) establishes the connection.

	return db.NewStore(sqlDB, db.DialectD1), nil
}
