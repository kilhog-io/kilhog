package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/kilhog-io/kilhog/internal/repository/db"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, dsn string) (*db.Store, error) {
	if err := ensureDatabase(ctx, dsn); err != nil {
		return nil, err
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres database: %w", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres database: %w", err)
	}

	return db.NewStore(sqlDB, db.DialectPostgres), nil
}

func ensureDatabase(ctx context.Context, dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("parse postgres dsn: %w", err)
	}

	dbName := strings.TrimPrefix(parsed.Path, "/")
	if dbName == "" {
		return fmt.Errorf("postgres dsn must include a database name")
	}

	adminDSN := *parsed
	adminDSN.Path = "/postgres"

	adminDB, err := sql.Open("pgx", adminDSN.String())
	if err != nil {
		return fmt.Errorf("open postgres admin database: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres admin database: %w", err)
	}

	var exists bool
	err = adminDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check postgres database existence: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := adminDB.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %q", dbName)); err != nil {
		return fmt.Errorf("create postgres database: %w", err)
	}

	return nil
}
