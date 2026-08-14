package db

import "fmt"

type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
	DialectD1       Dialect = "d1"
)

func ParseDialect(raw string) (Dialect, error) {
	switch Dialect(raw) {
	case DialectSQLite, DialectPostgres, DialectD1:
		return Dialect(raw), nil
	default:
		return "", fmt.Errorf("unsupported database driver %q: want sqlite, postgres, or d1", raw)
	}
}

// UsesSQLiteSyntax reports whether the dialect uses SQLite-compatible SQL
// (including Cloudflare D1).
func (d Dialect) UsesSQLiteSyntax() bool {
	return d == DialectSQLite || d == DialectD1
}

// SupportsSQLTransactions reports whether Begin/Commit are available.
// Cloudflare D1's Go driver does not support SQL transactions.
func (d Dialect) SupportsSQLTransactions() bool {
	return d != DialectD1
}

// MigrationDialect returns the embedded migration folder name for this dialect.
func (d Dialect) MigrationDialect() string {
	if d == DialectD1 {
		return string(DialectSQLite)
	}
	return string(d)
}
