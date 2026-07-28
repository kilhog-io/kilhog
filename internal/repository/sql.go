package repository

import (
	"fmt"

	"github.com/kilhog-io/kilhog/internal/repository/db"
)

func placeholder(dialect db.Dialect, index int) string {
	if dialect == db.DialectPostgres {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func placeholders(dialect db.Dialect, count int) string {
	if count == 0 {
		return ""
	}

	parts := make([]string, count)
	for i := range parts {
		parts[i] = placeholder(dialect, i+1)
	}

	switch dialect {
	case db.DialectPostgres:
		return joinComma(parts)
	default:
		return joinComma(repeat("?", count))
	}
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += ", " + part
	}
	return result
}

func repeat(value string, count int) []string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = value
	}
	return parts
}
