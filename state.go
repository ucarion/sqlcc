package main

import (
	"context"
	"fmt"
)

const initSQL1 = `create table %s (version int not null, dirty bool not null)`
const initSQL2 = `insert into %s values (0, false)`

func initState(ctx context.Context, stateTable string, q queryer) error {
	if _, err := q.ExecContext(ctx, fmt.Sprintf(initSQL1, stateTable)); err != nil {
		return fmt.Errorf("create state table: %w", err)
	}

	if _, err := q.ExecContext(ctx, fmt.Sprintf(initSQL2, stateTable)); err != nil {
		return fmt.Errorf("create state table: %w", err)
	}

	return nil
}

type state struct {
	version int
	dirty   bool
}

const stateSQL = `select version, dirty from %s limit 1`

func getState(ctx context.Context, stateTable string, q queryer) (state, error) {
	var s state
	row := q.QueryRowContext(ctx, fmt.Sprintf(stateSQL, stateTable))
	if err := row.Scan(&s.version, &s.dirty); err != nil {
		return state{}, fmt.Errorf("read state from db: %w", err)
	}

	return s, nil
}

const setStateSQL = `update %s set version = %v, dirty = %v`

func setState(ctx context.Context, stateTable string, q queryer, s state) error {
	if _, err := q.ExecContext(ctx, fmt.Sprintf(setStateSQL, stateTable, s.version, s.dirty)); err != nil {
		return fmt.Errorf("write state to db: %w", err)
	}

	return nil
}
