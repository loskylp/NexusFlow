// Package worker — PostgreSQL connector implementations (TASK-031).
//
// PostgreSQLDataSourceConnector reads rows from a PostgreSQL table (or SQL query)
// and returns them as records. PostgreSQLSinkConnector writes records to a
// PostgreSQL table inside a single BEGIN/COMMIT/ROLLBACK transaction (ADR-009).
//
// Both connectors depend on the postgresBackend interface so they can be unit-tested
// against InMemoryDatabase and integration-tested against the demo-postgres container.
//
// Docker Compose wires a real pgx connection pool at startup (demo profile only).
// The demo-postgres container is pre-seeded with 10K rows via init scripts.
//
// See: DEMO-002, ADR-003, ADR-009, TASK-031
package worker

import (
	"context"
)

// postgresBackend is the narrow interface the PostgreSQL connectors depend on.
// InMemoryDatabase satisfies this interface for unit tests.
// A real pgx pool adapter satisfies it in the demo Docker Compose environment.
//
// The interface exposes the four operations needed: query rows for fetch,
// and begin/insert/commit/rollback for atomic writes.
type postgresBackend interface {
	// QueryRows executes the given SQL query with args and returns each row
	// as a map from column name to value. Returns an empty slice if no rows match.
	QueryRows(ctx context.Context, query string, args ...any) ([]map[string]any, error)

	// Begin starts a database transaction. Returns a transaction handle or error.
	BeginTx(ctx context.Context) (postgresTx, error)

	// RowCount returns the number of rows in the given table matching the optional
	// WHERE clause. Used by Snapshot to count rows in scope.
	RowCount(ctx context.Context, table string) (int, error)
}

// postgresTx represents an in-progress database transaction.
// Returned by postgresBackend.BeginTx.
type postgresTx interface {
	// InsertRow inserts one row into the given table inside this transaction.
	InsertRow(ctx context.Context, table string, row map[string]any) error

	// Commit finalises the transaction. All inserted rows become visible.
	Commit(ctx context.Context) error

	// Rollback discards the transaction. No rows from this transaction persist.
	Rollback(ctx context.Context) error
}

// -----------------------------------------------------------------------
// PostgreSQLDataSourceConnector
// -----------------------------------------------------------------------

// PostgreSQLDataSourceConnector reads rows from a PostgreSQL table and returns
// them as records. The query is either a full SQL SELECT statement provided in
// config["query"], or a simple full-table fetch constructed from config["table"].
//
// Config keys (mutually exclusive; "query" takes precedence if both are present):
//   - "table" (string): table name for a full-table SELECT. Optional.
//   - "query" (string): raw SQL SELECT statement. Optional.
//   - "limit" (float64): maximum rows to return. Optional; 0 = no limit.
//
// Output: each record is a map from column name to scanned value.
//
// See: DEMO-002, TASK-031
type PostgreSQLDataSourceConnector struct {
	db postgresBackend
}

// NewPostgreSQLDataSourceConnector constructs a PostgreSQLDataSourceConnector backed
// by the given postgresBackend.
//
// Preconditions:
//   - db is non-nil.
func NewPostgreSQLDataSourceConnector(db postgresBackend) *PostgreSQLDataSourceConnector {
	// TODO: implement
	panic("not implemented")
}

// Type implements DataSourceConnector.Type.
// Returns "postgres" — the connector type string used in pipeline DataSourceConfig.
func (c *PostgreSQLDataSourceConnector) Type() string { return "postgres" }

// Fetch retrieves rows from the configured PostgreSQL table (or query) and returns
// them as a slice of record maps.
//
// Args:
//   - ctx:    Execution context. Cancellation aborts the query.
//   - config: DataSourceConfig.Config from the pipeline definition.
//             Either "table" or "query" must be present.
//   - input:  Task input parameters. Not used by this connector; passed for
//             interface compliance.
//
// Returns:
//   - A slice of records, one per row fetched. Each record maps column name to value.
//   - An error if the database is unreachable, the query fails, or neither "table"
//     nor "query" is configured.
//
// Preconditions:
//   - ctx is not cancelled.
//   - At least one of config["table"] or config["query"] is a non-empty string.
//
// Postconditions:
//   - On success: returns a non-nil slice (may be empty if the table is empty).
//   - On error: returns nil slice and a wrapped error describing the failure.
func (c *PostgreSQLDataSourceConnector) Fetch(ctx context.Context, config map[string]any, input map[string]any) ([]map[string]any, error) {
	// TODO: implement
	panic("not implemented")
}

// -----------------------------------------------------------------------
// PostgreSQLSinkConnector
// -----------------------------------------------------------------------

// PostgreSQLSinkConnector writes records to a PostgreSQL table using a single
// BEGIN/COMMIT/ROLLBACK transaction (ADR-009 Database Sink atomicity).
//
// Idempotency (ADR-003): a DedupStore is checked before writing; executionID is
// recorded after the transaction commits successfully.
//
// Config keys:
//   - "table" (string): destination table name. Required.
//   - "dsn"   (string): PostgreSQL DSN for the target database (may differ from
//             the NexusFlow system database). Optional; if absent, the connector
//             uses the postgresBackend injected at construction time.
//
// See: DEMO-002, ADR-003, ADR-009, TASK-031
type PostgreSQLSinkConnector struct {
	db    postgresBackend
	dedup DedupStore
}

// NewPostgreSQLSinkConnector constructs a PostgreSQLSinkConnector with the given
// postgresBackend and DedupStore.
//
// Preconditions:
//   - db is non-nil.
//   - dedup is non-nil.
func NewPostgreSQLSinkConnector(db postgresBackend, dedup DedupStore) *PostgreSQLSinkConnector {
	// TODO: implement
	panic("not implemented")
}

// Type implements SinkConnector.Type.
// Returns "postgres" — the connector type string used in pipeline SinkConfig.
func (c *PostgreSQLSinkConnector) Type() string { return "postgres" }

// Snapshot implements SinkConnector.Snapshot.
// Returns a map with "row_count" reflecting the number of rows in the configured
// table at the time of the call. Used for Before/After comparison (ADR-009).
//
// Config keys:
//   - "table" (string): table to count. Required.
//
// Preconditions:
//   - config["table"] is a non-empty string.
//
// Postconditions:
//   - Returns a non-nil map containing "row_count" (int).
//   - Returns an error if the database is unreachable or the query fails.
func (c *PostgreSQLSinkConnector) Snapshot(ctx context.Context, config map[string]any, taskID string) (map[string]any, error) {
	// TODO: implement
	panic("not implemented")
}

// Write atomically inserts records into the destination table.
//
// Idempotency guard (ADR-003): if executionID is already in the DedupStore,
// returns ErrAlreadyApplied without touching the database.
//
// Transaction pattern (ADR-009):
//  1. BeginTx to start a transaction.
//  2. InsertRow for each record.
//  3. On first insert error: Rollback and return the error (zero rows at destination).
//  4. On all inserts succeeded: Commit the transaction.
//  5. Record executionID in the DedupStore.
//
// Args:
//   - ctx:         Execution context. Cancellation causes the transaction to be rolled back.
//   - config:      SinkConfig.Config from the pipeline definition.
//   - records:     Records after Process->Sink schema mapping.
//   - executionID: Unique identifier for this execution attempt (taskID:attempt).
//
// Preconditions:
//   - executionID is non-empty.
//   - config["table"] is a non-empty string.
//   - c.db is a live backend (or InMemoryDatabase for tests).
//
// Postconditions:
//   - On nil: all records committed to the table; executionID recorded in DedupStore.
//   - On ErrAlreadyApplied: destination unchanged.
//   - On any other error: destination is in the same state as before Write was called
//     (transaction was rolled back).
func (c *PostgreSQLSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	// TODO: implement
	panic("not implemented")
}

// -----------------------------------------------------------------------
// Registration
// -----------------------------------------------------------------------

// RegisterPostgreSQLConnectors registers the PostgreSQL DataSource and Sink
// connectors in the given registry using the provided postgresBackend.
//
// Intended to be called at worker startup when the "demo" Docker Compose profile
// is active and the demo-postgres container is available.
//
// The caller is responsible for constructing the postgresBackend from the
// DEMO_POSTGRES_DSN environment variable.
//
// Panics on duplicate registration (fail-fast: calling this twice is a startup bug).
//
// See: DEMO-002, TASK-031, cmd/worker/main.go
func RegisterPostgreSQLConnectors(reg *DefaultConnectorRegistry, db postgresBackend) {
	// TODO: implement
	panic("not implemented")
}
