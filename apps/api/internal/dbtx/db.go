// Package dbtx provides transaction and error handling shared by the application modules.
package dbtx

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid request")
	ErrDenied   = errors.New("operation not permitted")
	ErrConflict = errors.New("operation conflicts with current state")
)

func Run[T any](ctx context.Context, db *sql.DB, work func(*sql.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	result, err := work(tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return result, nil
}

func Now(ctx context.Context, tx *sql.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRowContext(ctx, "SELECT clock_timestamp()").Scan(&now)
	return now.UTC(), err
}

func Lock(ctx context.Context, tx *sql.Tx, key string) error {
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", key)
	return err
}

func Changed(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return ErrConflict
	}
	return err
}
