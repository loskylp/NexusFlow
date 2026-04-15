// Package worker_test — unit tests for TASK-031: PostgreSQL connector (DataSource + Sink).
//
// Tests cover PostgreSQLDataSourceConnector (Fetch) and PostgreSQLSinkConnector
// (Write, Snapshot, idempotency, atomicity), plus connector registration.
// All tests use inMemoryPostgresDB and InMemoryDedupStore — no live PostgreSQL
// instance is required.
//
// See: DEMO-002, ADR-003, ADR-009, TASK-031
package worker_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nxlabs/nexusflow/worker"
)

// -----------------------------------------------------------------------
// inMemoryPostgresDB — test double for postgresBackend
// -----------------------------------------------------------------------
// inMemoryPostgresDB implements the unexported postgresBackend interface via the
// exported InMemoryPostgresDB façade. It mirrors the semantics of InMemoryDatabase
// but exposes the context-aware signature required by postgresBackend.

// -----------------------------------------------------------------------
// PostgreSQLDataSourceConnector tests
// -----------------------------------------------------------------------

// TestPostgreSQLDataSourceConnector_Type verifies the connector type string is "postgres".
func TestPostgreSQLDataSourceConnector_Type(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	c := worker.NewPostgreSQLDataSourceConnector(db)
	if got := c.Type(); got != "postgres" {
		t.Errorf("Type() = %q; want %q", got, "postgres")
	}
}

// TestPostgreSQLDataSourceConnector_Fetch_ByTable verifies that Fetch constructs a
// SELECT * FROM <table> query and returns one record per seeded row.
func TestPostgreSQLDataSourceConnector_Fetch_ByTable(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.Seed("sample_data", []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	})
	c := worker.NewPostgreSQLDataSourceConnector(db)

	records, err := c.Fetch(context.Background(), map[string]any{"table": "sample_data"}, nil)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}

// TestPostgreSQLDataSourceConnector_Fetch_ByQuery verifies that when config["query"]
// is set, Fetch passes it directly to QueryRows without constructing a table query.
func TestPostgreSQLDataSourceConnector_Fetch_ByQuery(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.Seed("sample_data", []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
		{"id": 3, "name": "carol"},
	})
	c := worker.NewPostgreSQLDataSourceConnector(db)

	// The in-memory backend returns all seeded rows regardless of SQL text.
	// The important thing is the connector forwards the raw query.
	records, err := c.Fetch(context.Background(),
		map[string]any{"query": "SELECT * FROM sample_data WHERE id < 3"},
		nil,
	)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	// InMemoryPostgresDB returns all rows for any query — just verify no error.
	_ = records
}

// TestPostgreSQLDataSourceConnector_Fetch_Limit verifies that the "limit" config
// key caps the number of records returned.
func TestPostgreSQLDataSourceConnector_Fetch_Limit(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	rows := make([]map[string]any, 10)
	for i := range rows {
		rows[i] = map[string]any{"id": i}
	}
	db.Seed("big_table", rows)
	c := worker.NewPostgreSQLDataSourceConnector(db)

	records, err := c.Fetch(context.Background(),
		map[string]any{"table": "big_table", "limit": float64(5)},
		nil,
	)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("expected 5 records with limit=5, got %d", len(records))
	}
}

// TestPostgreSQLDataSourceConnector_Fetch_NoConfig verifies that Fetch returns an
// error when neither "table" nor "query" is present in config.
func TestPostgreSQLDataSourceConnector_Fetch_NoConfig(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	c := worker.NewPostgreSQLDataSourceConnector(db)

	_, err := c.Fetch(context.Background(), map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected Fetch to return error when neither table nor query is configured, got nil")
	}
}

// TestPostgreSQLDataSourceConnector_Fetch_EmptyTable verifies that Fetch returns an
// empty (non-nil) slice when the table has no rows.
func TestPostgreSQLDataSourceConnector_Fetch_EmptyTable(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	c := worker.NewPostgreSQLDataSourceConnector(db)

	records, err := c.Fetch(context.Background(), map[string]any{"table": "empty_table"}, nil)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if records == nil {
		t.Error("expected non-nil slice for empty table, got nil")
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

// -----------------------------------------------------------------------
// PostgreSQLSinkConnector tests
// -----------------------------------------------------------------------

// TestPostgreSQLSinkConnector_Type verifies the connector type string is "postgres".
func TestPostgreSQLSinkConnector_Type(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)
	if got := c.Type(); got != "postgres" {
		t.Errorf("Type() = %q; want %q", got, "postgres")
	}
}

// TestPostgreSQLSinkConnector_Write_HappyPath verifies that a successful Write
// commits all records atomically to the destination table.
func TestPostgreSQLSinkConnector_Write_HappyPath(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}
	cfg := map[string]any{"table": "users"}

	if err := c.Write(context.Background(), cfg, records, "task-1:1"); err != nil {
		t.Fatalf("Write returned unexpected error: %v", err)
	}

	if len(db.Rows("users")) != 2 {
		t.Errorf("expected 2 committed rows, got %d", len(db.Rows("users")))
	}
}

// TestPostgreSQLSinkConnector_Write_Idempotency verifies that a second Write with
// the same executionID returns ErrAlreadyApplied without touching the database (ADR-003).
func TestPostgreSQLSinkConnector_Write_Idempotency(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{{"id": 1}}
	cfg := map[string]any{"table": "users"}
	execID := "task-idem:1"

	if err := c.Write(context.Background(), cfg, records, execID); err != nil {
		t.Fatalf("first Write error: %v", err)
	}

	err := c.Write(context.Background(), cfg, records, execID)
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("second Write: expected ErrAlreadyApplied, got %v", err)
	}
	// Row count must remain 1 — the second write must not insert.
	if len(db.Rows("users")) != 1 {
		t.Errorf("expected row count to remain 1 after idempotent write, got %d", len(db.Rows("users")))
	}
}

// TestPostgreSQLSinkConnector_Write_RollbackOnFailure verifies that when an insert
// fails mid-batch, the transaction is rolled back and no rows are committed (ADR-009).
func TestPostgreSQLSinkConnector_Write_RollbackOnFailure(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.FailAfterRow(1) // fail after first insert
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}
	cfg := map[string]any{"table": "users"}

	err := c.Write(context.Background(), cfg, records, "task-fail:1")
	if err == nil {
		t.Fatal("expected Write to return error on forced failure, got nil")
	}
	if len(db.Rows("users")) != 0 {
		t.Errorf("expected 0 rows after rollback, got %d (atomicity violation)", len(db.Rows("users")))
	}
}

// TestPostgreSQLSinkConnector_Write_MissingTable verifies that Write returns an
// error when config["table"] is absent.
func TestPostgreSQLSinkConnector_Write_MissingTable(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	err := c.Write(context.Background(), map[string]any{}, []map[string]any{{"id": 1}}, "task-notbl:1")
	if err == nil {
		t.Fatal("expected Write to return error when config missing table, got nil")
	}
}

// TestPostgreSQLSinkConnector_Snapshot verifies that Snapshot returns "row_count"
// matching the number of committed rows in the configured table.
func TestPostgreSQLSinkConnector_Snapshot(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.Seed("events", []map[string]any{
		{"id": 1}, {"id": 2}, {"id": 3},
	})
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	snap, err := c.Snapshot(context.Background(), map[string]any{"table": "events"}, "task-snap:1")
	if err != nil {
		t.Fatalf("Snapshot returned unexpected error: %v", err)
	}
	count, ok := snap["row_count"].(int)
	if !ok {
		t.Fatalf("snapshot missing int \"row_count\", got %T: %v", snap["row_count"], snap["row_count"])
	}
	if count != 3 {
		t.Errorf("expected row_count=3, got %d", count)
	}
}

// TestPostgreSQLSinkConnector_Snapshot_MissingTable verifies that Snapshot returns
// row_count=0 when config["table"] is absent or empty.
func TestPostgreSQLSinkConnector_Snapshot_MissingTable(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	c := worker.NewPostgreSQLSinkConnector(db, dedup)

	snap, err := c.Snapshot(context.Background(), map[string]any{}, "task-snap-empty:1")
	if err != nil {
		t.Fatalf("Snapshot returned unexpected error: %v", err)
	}
	count, ok := snap["row_count"].(int)
	if !ok {
		t.Fatalf("snapshot missing int \"row_count\", got %T: %v", snap["row_count"], snap["row_count"])
	}
	if count != 0 {
		t.Errorf("expected row_count=0 for missing table config, got %d", count)
	}
}

// -----------------------------------------------------------------------
// Registration test
// -----------------------------------------------------------------------

// TestRegisterPostgreSQLConnectors_RegistersBothKinds verifies that
// RegisterPostgreSQLConnectors registers "postgres" as both a DataSource and a Sink
// in the registry, and that both can be resolved without error.
func TestRegisterPostgreSQLConnectors_RegistersBothKinds(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	db := worker.NewInMemoryPostgresDB()

	worker.RegisterPostgreSQLConnectors(reg, db)

	if _, err := reg.DataSource("postgres"); err != nil {
		t.Errorf("DataSource(\"postgres\") returned error after registration: %v", err)
	}
	if _, err := reg.Sink("postgres"); err != nil {
		t.Errorf("Sink(\"postgres\") returned error after registration: %v", err)
	}
}

// -----------------------------------------------------------------------
// InMemoryPostgresDB — exported test double declared here so tests compile.
// The real type lives in the worker package (connector_postgres.go).
// This block provides a compile-time interface-satisfaction check.
// -----------------------------------------------------------------------

// Compile-time assertion: InMemoryPostgresDB is exported from the worker package.
// If the type is absent or the methods change, this file will not compile.
var _ interface {
	Seed(table string, rows []map[string]any)
	Rows(table string) []map[string]any
	RowCountTable(table string) int
	FailAfterRow(n int)
} = (*worker.InMemoryPostgresDB)(nil)

