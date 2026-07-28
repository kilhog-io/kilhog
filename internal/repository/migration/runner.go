package migration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kilhog-io/kilhog/internal/repository/db"
)

type Runner struct {
	store *db.Store
}

func NewRunner(store *db.Store) *Runner {
	return &Runner{store: store}
}

func (r *Runner) Upgrade(ctx context.Context) error {
	release, err := r.store.AcquireMigrationLock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = release()
	}()

	current, err := r.currentVersion(ctx)
	if err != nil {
		return err
	}

	scripts, err := loadScripts(string(r.store.Dialect), "up")
	if err != nil {
		return err
	}

	for _, script := range scripts {
		if script.Version <= current {
			continue
		}

		if err := r.apply(ctx, script); err != nil {
			return fmt.Errorf("apply migration %03d_%s: %w", script.Version, script.Name, err)
		}
	}

	return nil
}

func (r *Runner) Downgrade(ctx context.Context, targetVersion int) error {
	release, err := r.store.AcquireMigrationLock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = release()
	}()

	current, err := r.currentVersion(ctx)
	if err != nil {
		return err
	}

	if targetVersion >= current {
		return fmt.Errorf("target version %d must be lower than current version %d", targetVersion, current)
	}

	scripts, err := loadScripts(string(r.store.Dialect), "down")
	if err != nil {
		return err
	}

	sortDesc(scripts)

	for _, script := range scripts {
		if script.Version <= targetVersion {
			continue
		}

		if err := r.revert(ctx, script); err != nil {
			return fmt.Errorf("revert migration %03d_%s: %w", script.Version, script.Name, err)
		}
	}

	return nil
}

func (r *Runner) currentVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := r.store.DB.QueryRowContext(ctx, "SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		if isMissingMigrationTable(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read current migration version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

func isMissingMigrationTable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") || strings.Contains(message, "does not exist")
}

func (r *Runner) apply(ctx context.Context, script Script) error {
	return r.store.WithTx(ctx, nil, func(q db.Querier) error {
		for _, statement := range splitStatements(script.SQL) {
			if _, err := q.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("execute statement: %w", err)
			}
		}

		if _, err := q.ExecContext(ctx, insertMigrationVersion(r.store.Dialect), script.Version); err != nil {
			return fmt.Errorf("record migration version: %w", err)
		}

		return nil
	})
}

func (r *Runner) revert(ctx context.Context, script Script) error {
	return r.store.WithTx(ctx, nil, func(q db.Querier) error {
		if _, err := q.ExecContext(ctx, deleteMigrationVersion(r.store.Dialect), script.Version); err != nil {
			if !isMissingMigrationTable(err) {
				return fmt.Errorf("remove migration version: %w", err)
			}
		}

		for _, statement := range splitStatements(script.SQL) {
			if _, err := q.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("execute statement: %w", err)
			}
		}

		return nil
	})
}

func splitStatements(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := stripSQLComments(strings.TrimSpace(part))
		if statement == "" {
			continue
		}
		statements = append(statements, statement)
	}
	return statements
}

func stripSQLComments(sql string) string {
	lines := strings.Split(sql, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func insertMigrationVersion(dialect db.Dialect) string {
	if dialect == db.DialectPostgres {
		return "INSERT INTO schema_migrations (version) VALUES ($1)"
	}
	return "INSERT INTO schema_migrations (version) VALUES (?)"
}

func deleteMigrationVersion(dialect db.Dialect) string {
	if dialect == db.DialectPostgres {
		return "DELETE FROM schema_migrations WHERE version = $1"
	}
	return "DELETE FROM schema_migrations WHERE version = ?"
}

func sortDesc(scripts []Script) {
	for i := 0; i < len(scripts); i++ {
		for j := i + 1; j < len(scripts); j++ {
			if scripts[j].Version > scripts[i].Version {
				scripts[i], scripts[j] = scripts[j], scripts[i]
			}
		}
	}
}
