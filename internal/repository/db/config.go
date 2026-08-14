package db

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Driver      Dialect
	DSN         string
	AutoMigrate bool
}

func ConfigFromEnv() (Config, error) {
	driver, err := ParseDialect(envOrDefault("KILHOG_DB_DRIVER", string(DialectSQLite)))
	if err != nil {
		return Config{}, err
	}

	autoMigrate := true
	if raw := os.Getenv("KILHOG_AUTO_MIGRATE"); raw != "" {
		autoMigrate, err = strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse KILHOG_AUTO_MIGRATE: %w", err)
		}
	}

	return Config{
		Driver:      driver,
		DSN:         envOrDefault("KILHOG_DB_DSN", defaultDSN(driver)),
		AutoMigrate: autoMigrate,
	}, nil
}

func defaultDSN(driver Dialect) string {
	switch driver {
	case DialectD1:
		return "DB"
	default:
		return "file:kilhog.db?_pragma=foreign_keys(ON)"
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
