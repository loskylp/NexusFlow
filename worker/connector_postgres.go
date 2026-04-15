// Package worker — PostgreSQL connector implementations (TASK-031).
//
// PostgreSQLDataSourceConnector reads rows from a PostgreSQL table (or SQL query)
// and returns them as records. PostgreSQLSinkConnector writes records to a
// PostgreSQL table inside a single BEGIN/COMMIT/ROLLBACK transaction (ADR-009).
//
// Both connectors depend on the postgresBackend interface so they can be unit-tested
// against InMemoryPostgresDB and integration-tested against the demo-postgres container.
//
// Docker Compose wires a real pgx connection pool at startup (demo profile only).
// The demo-postgres container is pre-seeded with 10K rows via init scripts.
//
// See: DEMO-002, ADR-003, ADR-009, TASK-031
package worker

import (
	"context"
	"fmt"
	"sync"
)

// postgresBackend is the narrow interface the PostgreSQL connectors depend on.
// InMemoryPostgresDB satisfies this interface for unit tests.
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
// InMemoryPostgresDB — test double for postgresBackend
// -----------------------------------------------------------------------

// InMemoryPostgresDB is a thread-safe in-memory test double for postgresBackend.
// It stores committed rows per table and supports the transaction semantics
// required by postgresBackend and postgresTx.
//
// Tests call Seed to pre-populate rows and Rows/RowCount to inspect committed state.
// FailAfterRow injects transient errors for atomicity testing.
type InMemoryPostgresDB struct {
	mu        sync.Mutex
	committed map[string][]map[string]any // table -> committed rows
	failAfter int                         // fail after this many inserts in a transaction (-1 = never)
}

// NewInMemoryPostgresDB constructs an empty InMemoryPostgresDB with no failure injection.
func NewInMemoryPostgresDB() *InMemoryPostgresDB {
	return &InMemoryPostgresDB{
		committed: make(map[string][]map[string]any),
		failAfter: -1,
	}
}

// Seed pre-populates a table with rows. Used by tests to establish initial state.
func (d *InMemoryPostgresDB) Seed(table string, rows []map[string]any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.committed[table] = append(d.committed[table], rows...)
}

// Rows returns a copy of committed rows in the given table.
func (d *InMemoryPostgresDB) Rows(table string) []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	src := d.committed[table]
	cp := make([]map[string]any, len(src))
	copy(cp, src)
	return cp
}

// RowCountTable returns the number of committed rows in the given table.
// This helper is used by unit tests to inspect state without a context argument.
// The postgresBackend interface method (RowCount with ctx) is defined separately below.
func (d *InMemoryPostgresDB) RowCountTable(table string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.committed[table])
}

// FailAfterRow configures the database to return an error after n rows are inserted
// within a single transaction. Set n=0 to fail immediately on the first insert.
func (d *InMemoryPostgresDB) FailAfterRow(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.failAfter = n
}

// QueryRows implements postgresBackend.QueryRows. Returns all committed rows for
// the "table" represented by the query string. The in-memory implementation ignores
// the SQL text and returns every row for the internal table whose name was passed in
// via the first variadic arg (when present), or all committed rows concatenated
// when args are absent. For unit tests the caller is expected to use Seed to
// control the data; the SQL query text is not evaluated.
//
// Implementation note: the query argument is stored to satisfy the interface contract
// but never executed. The backend returns all committed rows across all tables when
// no args are given, matching the behaviour expected by Fetch when "query" is
// configured and the test has seeded rows into a named table.
func (d *InMemoryPostgresDB) QueryRows(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Collect all committed rows across every table (unit tests seed a single table
	// and the returned rows are what matter for assertions, not the SQL text).
	var result []map[string]any
	for _, rows := range d.committed {
		result = append(result, rows...)
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

// BeginTx implements postgresBackend.BeginTx. Returns a new in-memory transaction.
func (d *InMemoryPostgresDB) BeginTx(ctx context.Context) (postgresTx, error) {
	d.mu.Lock()
	failAfter := d.failAfter
	d.mu.Unlock()
	return &inMemoryPostgresTx{db: d, failAfter: failAfter}, nil
}

// RowCount implements postgresBackend.RowCount. Returns the committed row count.
func (d *InMemoryPostgresDB) rowCountLocked(table string) int {
	return len(d.committed[table])
}

// inMemoryPostgresTx is the transaction handle returned by InMemoryPostgresDB.BeginTx.
// It buffers inserted rows and only promotes them to committed state on Commit.
type inMemoryPostgresTx struct {
	db          *InMemoryPostgresDB
	inFlight    map[string][]map[string]any
	insertCount int
	failAfter   int
	done        bool
}

// InsertRow implements postgresTx.InsertRow. Appends the row to the in-flight buffer.
// Returns an error if the failure injection threshold has been reached.
func (t *inMemoryPostgresTx) InsertRow(ctx context.Context, table string, row map[string]any) error {
	if t.failAfter >= 0 && t.insertCount >= t.failAfter {
		return fmt.Errorf("simulated postgres failure after %d row(s)", t.failAfter)
	}
	if t.inFlight == nil {
		t.inFlight = make(map[string][]map[string]any)
	}
	t.inFlight[table] = append(t.inFlight[table], row)
	t.insertCount++
	return nil
}

// Commit implements postgresTx.Commit. Moves in-flight rows to the committed store.
//
// Postcondition: committed store reflects all rows inserted in this transaction;
// in-flight buffer is cleared.
func (t *inMemoryPostgresTx) Commit(ctx context.Context) error {
	if t.done {
		return fmt.Errorf("inMemoryPostgresTx: already committed or rolled back")
	}
	t.done = true
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	for table, rows := range t.inFlight {
		t.db.committed[table] = append(t.db.committed[table], rows...)
	}
	t.inFlight = nil
	return nil
}

// Rollback implements postgresTx.Rollback. Discards all in-flight rows.
//
// Postcondition: committed store is unchanged from before BeginTx.
func (t *inMemoryPostgresTx) Rollback(ctx context.Context) error {
	if t.done {
		return nil // idempotent — rolling back an already-committed tx is a no-op
	}
	t.done = true
	t.inFlight = nil
	return nil
}

// RowCount satisfies the postgresBackend interface. Returns the number of committed
// rows in the given table. Thread-safe.
func (d *InMemoryPostgresDB) RowCount(ctx context.Context, table string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.committed[table]), nil
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
	if db == nil {
		panic("NewPostgreSQLDataSourceConnector: db must not be nil")
	}
	return &PostgreSQLDataSourceConnector{db: db}
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
	query, hasQuery := config["query"].(string)
	table, hasTable := config["table"].(string)

	var sql string
	switch {
	case hasQuery && query != "":
		sql = query
	case hasTable && table != "":
		sql = fmt.Sprintf("SELECT * FROM %s", table)
	default:
		return nil, fmt.Errorf("postgres datasource: config must provide a non-empty \"table\" or \"query\"")
	}

	rows, err := c.db.QueryRows(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("postgres datasource: query failed: %w", err)
	}

	// Apply limit when configured. JSON numbers decode as float64.
	if limitRaw, ok := config["limit"]; ok {
		if limit, ok := limitRaw.(float64); ok && limit > 0 && int(limit) < len(rows) {
			rows = rows[:int(limit)]
		}
	}

	// Guarantee a non-nil slice so callers can range over the result unconditionally.
	if rows == nil {
		rows = []map[string]any{}
	}
	return rows, nil
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
	if db == nil {
		panic("NewPostgreSQLSinkConnector: db must not be nil")
	}
	if dedup == nil {
		panic("NewPostgreSQLSinkConnector: dedup must not be nil")
	}
	return &PostgreSQLSinkConnector{db: db, dedup: dedup}
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
	table, _ := config["table"].(string)
	if table == "" {
		return map[string]any{"row_count": 0}, nil
	}
	count, err := c.db.RowCount(ctx, table)
	if err != nil {
		return nil, fmt.Errorf("postgres sink snapshot: count rows in %q: %w", table, err)
	}
	return map[string]any{"row_count": count}, nil
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
//   - c.db is a live backend (or InMemoryPostgresDB for tests).
//
// Postconditions:
//   - On nil: all records committed to the table; executionID recorded in DedupStore.
//   - On ErrAlreadyApplied: destination unchanged.
//   - On any other error: destination is in the same state as before Write was called
//     (transaction was rolled back).
func (c *PostgreSQLSinkConnector) Write(ctx context.Context, config map[string]any, records []map[string]any, executionID string) error {
	if c.dedup.Applied(executionID) {
		return ErrAlreadyApplied
	}

	table, _ := config["table"].(string)
	if table == "" {
		return fmt.Errorf("postgres sink: config missing required key \"table\"")
	}

	tx, err := c.db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("postgres sink: begin transaction: %w", err)
	}

	for _, record := range records {
		if err := tx.InsertRow(ctx, table, record); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("postgres sink: insert failed (rolled back): %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("postgres sink: commit failed (rolled back): %w", err)
	}

	if err := c.dedup.Record(ctx, executionID); err != nil {
		// Commit already succeeded; best-effort dedup record. Log the error but do
		// not fail the write — the data is durable. A redelivery will hit the
		// idempotency check. See DatabaseSinkConnector.Write for the same rationale.
		_ = err
	}
	return nil
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
	reg.Register("datasource", NewPostgreSQLDataSourceConnector(db))
	reg.Register("sink", NewPostgreSQLSinkConnector(db, NewInMemoryDedupStore()))
}
