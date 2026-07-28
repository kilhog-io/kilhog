package db

import "fmt"

type Dialect string

const (
	DialectSQLite  Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

func ParseDialect(raw string) (Dialect, error) {
	switch Dialect(raw) {
	case DialectSQLite, DialectPostgres:
		return Dialect(raw), nil
	default:
		return "", fmt.Errorf("unsupported database driver %q: want sqlite or postgres", raw)
	}
}
