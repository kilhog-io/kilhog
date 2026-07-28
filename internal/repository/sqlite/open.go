package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kilhog-io/kilhog/internal/repository/db"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, dsn string) (*db.Store, error) {
	if err := ensureDatabaseFile(dsn); err != nil {
		return nil, err
	}

	normalizedDSN := normalizeDSN(dsn)
	sqlDB, err := sql.Open("sqlite", normalizedDSN)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)

	if err := configure(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	return db.NewStore(sqlDB, db.DialectSQLite), nil
}

func ensureDatabaseFile(dsn string) error {
	filePath, err := filePathFromDSN(dsn)
	if err != nil {
		return err
	}
	if filePath == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return fmt.Errorf("create sqlite directory: %w", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("create sqlite database file: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close sqlite database file: %w", err)
		}
	}

	return nil
}

func configure(ctx context.Context, sqlDB *sql.DB) error {
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	}

	for _, pragma := range pragmas {
		if _, err := sqlDB.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("apply %s: %w", pragma, err)
		}
	}

	return nil
}

func normalizeDSN(dsn string) string {
	if strings.HasPrefix(dsn, "file:") {
		return dsn
	}
	return "file:" + dsn
}

func filePathFromDSN(dsn string) (string, error) {
	normalized := normalizeDSN(dsn)
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parse sqlite dsn: %w", err)
	}

	path := parsed.Path
	if path == "" {
		path = strings.TrimPrefix(parsed.Opaque, "/")
	}
	if path == "" {
		return "", nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Clean(path)
	}

	return path, nil
}
