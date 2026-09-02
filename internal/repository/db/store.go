package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"
)

type Store struct {
	DB      *sql.DB
	Dialect Dialect
	writeMu sync.Mutex
}

func NewStore(db *sql.DB, dialect Dialect) *Store {
	return &Store{DB: db, Dialect: dialect}
}

// DefaultSQLiteFlushTimeout bounds WAL checkpoint retries when Close has no
// deadline. Cloud Run sends SIGKILL 10s after SIGTERM; HTTP shutdown leaves
// this window to synchronize and close the SQLite file.
const DefaultSQLiteFlushTimeout = 2 * time.Second

func (s *Store) Ping(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("database is closed")
	}

	s.writeMu.Lock()
	sqlDB := s.DB
	s.writeMu.Unlock()

	if sqlDB == nil {
		return fmt.Errorf("database is closed")
	}
	return sqlDB.PingContext(ctx)
}

// Flush persists pending SQLite WAL pages into the main database file.
// It is a no-op for PostgreSQL and Cloudflare D1.
//
// On SQLite, idle pooled connections are dropped so TRUNCATE checkpoint can
// complete, then PRAGMA wal_checkpoint(TRUNCATE) is retried until the WAL is
// fully merged or ctx is done. Call this before Close on SIGTERM so Cloud Run
// does not SIGKILL the process with an unsynced WAL.
func (s *Store) Flush(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if s.DB == nil || s.Dialect != DialectSQLite {
		return nil
	}

	// Other pooled connections can keep WAL readers alive and make TRUNCATE
	// report busy. Drop idles and cap the pool to a single connection.
	s.DB.SetMaxIdleConns(0)
	s.DB.SetMaxOpenConns(1)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		var busy, logFrames, checkpointed int
		err := s.DB.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &checkpointed)
		if err != nil {
			return fmt.Errorf("checkpoint sqlite wal: %w", err)
		}
		if busy == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("checkpoint sqlite wal: still busy (log=%d checkpointed=%d): %w", logFrames, checkpointed, ctx.Err())
		case <-ticker.C:
		}
	}
}

// Close checkpoints SQLite (see Flush) then closes the database handle.
// Subsequent Close/Flush calls are no-ops.
func (s *Store) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultSQLiteFlushTimeout)
	defer cancel()
	return s.CloseContext(ctx)
}

// CloseContext is Close with an explicit deadline for the SQLite WAL checkpoint.
func (s *Store) CloseContext(ctx context.Context) error {
	if s == nil {
		return nil
	}

	flushErr := s.Flush(ctx)

	s.writeMu.Lock()
	sqlDB := s.DB
	s.DB = nil
	s.writeMu.Unlock()

	if sqlDB == nil {
		return flushErr
	}

	if err := sqlDB.Close(); err != nil {
		if flushErr != nil {
			return fmt.Errorf("flush sqlite: %w (close database: %v)", flushErr, err)
		}
		return fmt.Errorf("close database: %w", err)
	}
	if flushErr != nil {
		return fmt.Errorf("flush sqlite: %w", flushErr)
	}
	return nil
}

func (s *Store) Querier() Querier {
	return s.DB
}

func (s *Store) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := s.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return &Tx{Tx: tx}, nil
}

func (s *Store) WithTx(ctx context.Context, opts *sql.TxOptions, fn func(Querier) error) error {
	// Cloudflare D1 does not support SQL transactions in the Go driver.
	// Fall back to executing statements sequentially under the write lock.
	if !s.Dialect.SupportsSQLTransactions() {
		return fn(s.DB)
	}

	tx, err := s.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

func (s *Store) WithWriteLock(ctx context.Context, fn func(Querier) error) error {
	if s.Dialect.UsesSQLiteSyntax() {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
	}
	return fn(s.DB)
}

func (s *Store) WithWriteTx(ctx context.Context, fn func(Querier) error) error {
	return s.WithWriteLock(ctx, func(q Querier) error {
		return s.WithTx(ctx, nil, fn)
	})
}

func (s *Store) AcquireMigrationLock(ctx context.Context) (release func() error, err error) {
	switch s.Dialect {
	case DialectSQLite, DialectD1:
		s.writeMu.Lock()
		return func() error {
			s.writeMu.Unlock()
			return nil
		}, nil
	case DialectPostgres:
		if _, err := s.DB.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
			return nil, fmt.Errorf("acquire postgres migration lock: %w", err)
		}
		return func() error {
			_, err := s.DB.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
			return err
		}, nil
	default:
		return nil, fmt.Errorf("unsupported dialect %q", s.Dialect)
	}
}

const migrationLockKey int64 = 0x4b494c484f47 // "KILHOG"
