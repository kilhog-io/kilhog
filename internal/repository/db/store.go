package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

type Store struct {
	DB      *sql.DB
	Dialect Dialect
	writeMu sync.Mutex
}

func NewStore(db *sql.DB, dialect Dialect) *Store {
	return &Store{DB: db, Dialect: dialect}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

func (s *Store) Close() error {
	return s.DB.Close()
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
	if s.Dialect == DialectSQLite {
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
	case DialectSQLite:
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
