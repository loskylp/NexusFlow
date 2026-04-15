// Package worker — pgx adapter for postgresBackend (TASK-031).
//
// PgxBackendAdapter wraps a pgx connection pool and satisfies the postgresBackend
// interface used by PostgreSQLDataSourceConnector and PostgreSQLSinkConnector.
//
// This file is separated from connector_postgres.go so the in-memory test double
// (InMemoryPostgresDB) and the real pgx pool do not share a file — each has a
// single reason to change (SRP).
//
// See: DEMO-002, TASK-031
package worker

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxBackendAdapter adapts a *pgxpool.Pool to the postgresBackend interface.
// Used by the worker process when DEMO_POSTGRES_DSN is set (demo profile).
type PgxBackendAdapter struct {
	pool *pgxpool.Pool
}

// NewPgxBackendAdapter connects to PostgreSQL using the given DSN and returns a
// PgxBackendAdapter ready for use as a postgresBackend.
//
// The pool is created with pgxpool.New using the default pool configuration.
// The caller should ensure the demo-postgres container is healthy before calling
// this function — connection errors during startup are returned as non-nil errors.
//
// Preconditions:
//   - dsn is a valid PostgreSQL connection string (postgres://user:pass@host:port/db).
//
// Postconditions:
//   - Returns a non-nil adapter on success.
//   - Returns a non-nil error if the DSN cannot be parsed or the pool cannot be created.
func NewPgxBackendAdapter(dsn string) (*PgxBackendAdapter, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("NewPgxBackendAdapter: create pool for %q: %w", dsn, err)
	}
	return &PgxBackendAdapter{pool: pool}, nil
}

// QueryRows implements postgresBackend.QueryRows.
// Executes the SQL query with the given args and returns each row as a map from
// column name to scanned value. Returns an empty slice (not an error) when the
// result set is empty.
//
// Preconditions:
//   - ctx is not cancelled.
//   - query is a valid SQL SELECT statement.
func (a *PgxBackendAdapter) QueryRows(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pgx QueryRows: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	var result []map[string]any

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("pgx QueryRows: scan row: %w", err)
		}
		row := make(map[string]any, len(fieldDescs))
		for i, fd := range fieldDescs {
			row[string(fd.Name)] = values[i]
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgx QueryRows: iterate rows: %w", err)
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// BeginTx implements postgresBackend.BeginTx.
// Starts a new database transaction and returns a PgxTxAdapter wrapping it.
func (a *PgxBackendAdapter) BeginTx(ctx context.Context) (postgresTx, error) {
	tx, err := a.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("pgx BeginTx: %w", err)
	}
	return &pgxTxAdapter{tx: tx}, nil
}

// RowCount implements postgresBackend.RowCount.
// Executes SELECT COUNT(*) FROM <table> and returns the count.
//
// Preconditions:
//   - table is a valid, unquoted table name. (The caller is responsible for
//     ensuring table names are safe; connector config values must be trusted.)
func (a *PgxBackendAdapter) RowCount(ctx context.Context, table string) (int, error) {
	var count int
	err := a.pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("pgx RowCount %q: %w", table, err)
	}
	return count, nil
}

// pgxTxAdapter wraps a pgx.Tx and satisfies the postgresTx interface.
type pgxTxAdapter struct {
	tx pgx.Tx
}

// InsertRow implements postgresTx.InsertRow.
// Constructs a parameterised INSERT statement from the column/value map and
// executes it within the open transaction.
//
// Column order is determined by iterating the map, which is non-deterministic in Go.
// This is acceptable for the demo profile; production code should use sqlc-generated
// queries with explicit column lists.
func (t *pgxTxAdapter) InsertRow(ctx context.Context, table string, row map[string]any) error {
	if len(row) == 0 {
		return nil // nothing to insert; treat as a no-op
	}

	cols := make([]string, 0, len(row))
	vals := make([]any, 0, len(row))
	placeholders := make([]string, 0, len(row))
	i := 1
	for col, val := range row {
		cols = append(cols, col)
		vals = append(vals, val)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		i++
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		joinStrings(cols, ", "),
		joinStrings(placeholders, ", "),
	)

	if _, err := t.tx.Exec(ctx, sql, vals...); err != nil {
		return fmt.Errorf("pgxTxAdapter InsertRow into %q: %w", table, err)
	}
	return nil
}

// Commit implements postgresTx.Commit.
func (t *pgxTxAdapter) Commit(ctx context.Context) error {
	if err := t.tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgxTxAdapter Commit: %w", err)
	}
	return nil
}

// Rollback implements postgresTx.Rollback.
func (t *pgxTxAdapter) Rollback(ctx context.Context) error {
	if err := t.tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
		return fmt.Errorf("pgxTxAdapter Rollback: %w", err)
	}
	return nil
}

// joinStrings joins a slice of strings with the given separator.
// Defined here to avoid a dependency on strings.Join for a one-line helper.
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
