package main

import (
	"context"
	"database/sql"
	"fmt"
)

type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func withTx(ctx context.Context, inTx bool, db *sql.DB, f func(queryer) error) error {
	if !inTx {
		return f(db)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	if err := f(tx); err != nil {
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("rollback tx: %w", err)
		}

		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
