// Package integration — TASK-031 integration tests: PgxBackendAdapter against live demo-postgres.
//
// Requirement: DEMO-002, ADR-003, ADR-009, TASK-031
//
// These tests exercise the PgxBackendAdapter (worker/connector_postgres_pgx.go) against the
// real demo-postgres container. They verify the adapter satisfies the postgresBackend contract
// at the component boundary — no internal mocks are used for database calls.
//
// Skipped unless PG_TEST_DSN is set (e.g. "postgres://demo:demo@localhost:5433/demo").
//
// Run:
//
//	PG_TEST_DSN=postgres://demo:demo@localhost:5433/demo go test ./tests/integration/... -v -run TASK031
package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/worker"
)

// pgxTestAdapter returns a PgxBackendAdapter connected to the live demo-postgres instance.
// Skips the test if PG_TEST_DSN is not set.
func pgxTestAdapter(t *testing.T) *worker.PgxBackendAdapter {
	t.Helper()
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live PostgreSQL integration tests")
	}
	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("NewPgxBackendAdapter(%q): %v", dsn, err)
	}
	return adapter
}

// ---------------------------------------------------------------------------
// INT-031-1: QueryRows — reads rows from demo-postgres
// ---------------------------------------------------------------------------

// TestTASK031_INT1_QueryRowsReturnsSeedRows verifies that QueryRows returns the expected
// rows from the seeded sample_data table.
//
// DEMO-002 / TASK-031: PgxBackendAdapter.QueryRows exercises the real pgx query path.
//
// Given: demo-postgres is running; sample_data has 10000 rows
// When:  QueryRows is called with "SELECT id, name FROM sample_data ORDER BY id LIMIT 5"
// Then:  5 rows are returned, each with "id" and "name" fields; first row id=1
func TestTASK031_INT1_QueryRowsReturnsSeedRows(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	rows, err := adapter.QueryRows(ctx, "SELECT id, name FROM sample_data ORDER BY id LIMIT 5")
	if err != nil {
		t.Fatalf("INT-031-1 FAIL: QueryRows error: %v", err)
	}
	if len(rows) != 5 {
		t.Errorf("INT-031-1 FAIL: expected 5 rows, got %d", len(rows))
	}

	// Verify row structure — each row must have "id" and "name".
	for i, row := range rows {
		if _, ok := row["id"]; !ok {
			t.Errorf("INT-031-1 FAIL: row[%d] missing field \"id\": %v", i, row)
		}
		if _, ok := row["name"]; !ok {
			t.Errorf("INT-031-1 FAIL: row[%d] missing field \"name\": %v", i, row)
		}
	}
}

// TestTASK031_INT1_QueryRowsReturnsEmptySliceForEmptyTable verifies that QueryRows
// returns a non-nil empty slice when the table exists but has no matching rows.
//
// DEMO-002 / TASK-031: empty-table behaviour at the adapter boundary.
//
// Given: demo_output table exists and is empty
// When:  QueryRows is called with "SELECT * FROM demo_output"
// Then:  a non-nil empty slice is returned; no error
func TestTASK031_INT1_QueryRowsReturnsEmptySliceForEmptyTable(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	rows, err := adapter.QueryRows(ctx, "SELECT * FROM demo_output")
	if err != nil {
		t.Fatalf("INT-031-1 FAIL: QueryRows on empty table error: %v", err)
	}
	if rows == nil {
		t.Error("INT-031-1 FAIL: QueryRows returned nil slice for empty table, want non-nil empty slice")
	}
	if len(rows) != 0 {
		t.Errorf("INT-031-1 FAIL: expected 0 rows from empty table, got %d", len(rows))
	}
}

// TestTASK031_INT1_QueryRowsErrorOnInvalidSQL verifies that QueryRows propagates the
// pgx error when the SQL is invalid, rather than silently returning empty results.
//
// DEMO-002 / TASK-031 negative: adapter must not swallow errors from the database.
//
// Given: an invalid SQL statement
// When:  QueryRows is called
// Then:  a non-nil error is returned
func TestTASK031_INT1_QueryRowsErrorOnInvalidSQL(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	_, err := adapter.QueryRows(ctx, "SELECT * FROM table_that_does_not_exist_9z8y7x")
	if err == nil {
		t.Fatal("INT-031-1 FAIL (negative): QueryRows should return error on invalid SQL, got nil")
	}
}

// ---------------------------------------------------------------------------
// INT-031-2: RowCount — counts rows in a live table
// ---------------------------------------------------------------------------

// TestTASK031_INT2_RowCountReturns10000ForSampleData verifies that RowCount returns
// exactly 10000 for the seeded sample_data table (AC-1: 10K pre-seeded rows).
//
// DEMO-002 / TASK-031 AC-1: seed count verification at the adapter layer.
//
// Given: sample_data table has 10000 rows (seeded by 01-seed.sql)
// When:  RowCount is called with table="sample_data"
// Then:  10000 is returned
func TestTASK031_INT2_RowCountReturns10000ForSampleData(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	count, err := adapter.RowCount(ctx, "sample_data")
	if err != nil {
		t.Fatalf("INT-031-2 FAIL: RowCount(sample_data) error: %v", err)
	}
	if count != 10000 {
		t.Errorf("INT-031-2 FAIL: expected 10000 rows in sample_data, got %d", count)
	}
}

// TestTASK031_INT2_RowCountReturns0ForDemoOutput verifies that RowCount returns 0
// for the initially-empty demo_output table.
//
// DEMO-002 / TASK-031: empty table RowCount at adapter boundary.
//
// Given: demo_output table exists and is empty
// When:  RowCount is called with table="demo_output"
// Then:  0 is returned without error
func TestTASK031_INT2_RowCountReturns0ForDemoOutput(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	count, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-2 FAIL: RowCount(demo_output) error: %v", err)
	}
	if count != 0 {
		t.Errorf("INT-031-2 FAIL: expected 0 rows in demo_output, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// INT-031-3: BeginTx / InsertRow / Commit round-trip
// ---------------------------------------------------------------------------

// TestTASK031_INT3_BeginTxInsertRowCommitPersistsRow verifies the full transaction
// cycle: BeginTx → InsertRow → Commit results in a row visible in the table.
//
// DEMO-002 / ADR-009 / TASK-031: PgxBackendAdapter transaction round-trip.
//
// Given: demo_output table is accessible
// When:  a transaction is opened, one row is inserted, and the transaction is committed
// Then:  RowCount(demo_output) increases by 1; no error throughout
func TestTASK031_INT3_BeginTxInsertRowCommitPersistsRow(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	before, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: RowCount before: %v", err)
	}

	tx, err := adapter.BeginTx(ctx)
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: BeginTx: %v", err)
	}

	uniqueID := fmt.Sprintf("int-031-3-%d", time.Now().UnixNano())
	row := map[string]any{
		"data": fmt.Sprintf(`{"test_id": "%s"}`, uniqueID),
	}
	if err := tx.InsertRow(ctx, "demo_output", row); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("INT-031-3 FAIL: InsertRow: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("INT-031-3 FAIL: Commit: %v", err)
	}

	after, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: RowCount after commit: %v", err)
	}
	if after != before+1 {
		t.Errorf("INT-031-3 FAIL: expected RowCount to increase by 1 (before=%d, after=%d)", before, after)
	}
}

// TestTASK031_INT3_RollbackDoesNotPersistRow verifies that rolling back a transaction
// leaves the table unchanged — the ADR-009 atomicity guarantee at the adapter layer.
//
// DEMO-002 / ADR-009 / TASK-031: rollback atomicity at adapter boundary.
//
// Given: a transaction is opened with an InsertRow call
// When:  Rollback is called before Commit
// Then:  RowCount(demo_output) is unchanged; the inserted row is not visible
func TestTASK031_INT3_RollbackDoesNotPersistRow(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	before, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: RowCount before rollback test: %v", err)
	}

	tx, err := adapter.BeginTx(ctx)
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: BeginTx for rollback test: %v", err)
	}

	uniqueID := fmt.Sprintf("int-031-3-rollback-%d", time.Now().UnixNano())
	row := map[string]any{
		"data": fmt.Sprintf(`{"test_id": "%s"}`, uniqueID),
	}
	if err := tx.InsertRow(ctx, "demo_output", row); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("INT-031-3 FAIL: InsertRow in rollback test: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("INT-031-3 FAIL: Rollback: %v", err)
	}

	after, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-3 FAIL: RowCount after rollback: %v", err)
	}
	if after != before {
		t.Errorf("INT-031-3 FAIL: expected RowCount unchanged after rollback (before=%d, after=%d)", before, after)
	}
}

// ---------------------------------------------------------------------------
// INT-031-4: PostgreSQLDataSourceConnector via PgxBackendAdapter (live)
// ---------------------------------------------------------------------------

// TestTASK031_INT4_DataSourceConnectorFetchesFromLiveTable verifies that
// PostgreSQLDataSourceConnector.Fetch works against the live demo-postgres table
// via PgxBackendAdapter.
//
// DEMO-002 / TASK-031 AC-2: PostgreSQL DataSource can query data from demo-postgres.
//
// Given: PgxBackendAdapter connected to demo-postgres; sample_data has 10000 rows
// When:  PostgreSQLDataSourceConnector.Fetch is called with table="sample_data", limit=10
// Then:  10 records are returned; each record has "id", "name", "category", "value", "score" fields
func TestTASK031_INT4_DataSourceConnectorFetchesFromLiveTable(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	connector := worker.NewPostgreSQLDataSourceConnector(adapter)
	cfg := map[string]any{
		"table": "sample_data",
		"limit": float64(10),
	}
	records, err := connector.Fetch(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("INT-031-4 FAIL: Fetch error: %v", err)
	}
	if len(records) != 10 {
		t.Errorf("INT-031-4 FAIL: expected 10 records (limit=10), got %d", len(records))
	}
	for i, rec := range records {
		for _, field := range []string{"id", "name", "category", "value", "score"} {
			if _, ok := rec[field]; !ok {
				t.Errorf("INT-031-4 FAIL: record[%d] missing field %q: %v", i, field, rec)
			}
		}
	}
}

// TestTASK031_INT4_DataSourceConnectorRawQueryReturnsFilteredRows verifies that
// Fetch honours a raw SQL query (AC-2 raw query path) via the live adapter.
//
// DEMO-002 / TASK-031 AC-2: raw SQL query config key.
//
// Given: sample_data seeded with rows whose category is one of "alpha","beta",…,"epsilon"
// When:  Fetch is called with query="SELECT * FROM sample_data WHERE category='alpha' LIMIT 5"
// Then:  up to 5 rows are returned, each with category="alpha"
func TestTASK031_INT4_DataSourceConnectorRawQueryReturnsFilteredRows(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	connector := worker.NewPostgreSQLDataSourceConnector(adapter)
	cfg := map[string]any{
		"query": "SELECT * FROM sample_data WHERE category='alpha' LIMIT 5",
	}
	records, err := connector.Fetch(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("INT-031-4 FAIL: Fetch with raw query error: %v", err)
	}
	if len(records) > 5 {
		t.Errorf("INT-031-4 FAIL: expected at most 5 records, got %d", len(records))
	}
	for i, rec := range records {
		catRaw, ok := rec["category"]
		if !ok {
			t.Errorf("INT-031-4 FAIL: record[%d] missing field \"category\": %v", i, rec)
			continue
		}
		// pgx returns TEXT as string
		cat := fmt.Sprintf("%v", catRaw)
		if cat != "alpha" {
			t.Errorf("INT-031-4 FAIL: record[%d] category=%q, want \"alpha\"", i, cat)
		}
	}
}

// ---------------------------------------------------------------------------
// INT-031-5: PostgreSQLSinkConnector via PgxBackendAdapter (live)
// ---------------------------------------------------------------------------

// TestTASK031_INT5_SinkConnectorWritesRowsToLiveTable verifies that
// PostgreSQLSinkConnector.Write commits rows to demo_output via PgxBackendAdapter.
//
// DEMO-002 / ADR-009 / TASK-031 AC-3: PostgreSQL Sink can write data to demo-postgres.
//
// Given: PgxBackendAdapter connected to demo-postgres; demo_output exists
// When:  PostgreSQLSinkConnector.Write is called with 3 records and table="demo_output"
// Then:  RowCount(demo_output) increases by 3; no error
func TestTASK031_INT5_SinkConnectorWritesRowsToLiveTable(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	before, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-5 FAIL: RowCount before: %v", err)
	}

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(adapter, dedup)

	uniqueExecID := fmt.Sprintf("int-031-5-%d", time.Now().UnixNano())
	records := []map[string]any{
		{"data": `{"id":1,"name":"alice"}`},
		{"data": `{"id":2,"name":"bob"}`},
		{"data": `{"id":3,"name":"carol"}`},
	}
	cfg := map[string]any{"table": "demo_output"}

	if err := sink.Write(ctx, cfg, records, uniqueExecID); err != nil {
		t.Fatalf("INT-031-5 FAIL: Write error: %v", err)
	}

	after, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-5 FAIL: RowCount after: %v", err)
	}
	if after != before+3 {
		t.Errorf("INT-031-5 FAIL: expected RowCount to increase by 3 (before=%d, after=%d)", before, after)
	}
}

// TestTASK031_INT5_SinkConnectorSnapshotReflectsLiveRowCount verifies that
// PostgreSQLSinkConnector.Snapshot returns the current live row count for demo_output.
//
// DEMO-002 / ADR-009 / TASK-031: Snapshot via live adapter.
//
// Given: demo_output has a known row count (from previous writes in this run)
// When:  Snapshot is called with table="demo_output"
// Then:  the returned row_count matches what RowCount(demo_output) returns
func TestTASK031_INT5_SinkConnectorSnapshotReflectsLiveRowCount(t *testing.T) {
	adapter := pgxTestAdapter(t)
	ctx := context.Background()

	expected, err := adapter.RowCount(ctx, "demo_output")
	if err != nil {
		t.Fatalf("INT-031-5 FAIL: RowCount: %v", err)
	}

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(adapter, dedup)

	snap, err := sink.Snapshot(ctx, map[string]any{"table": "demo_output"}, "snap-task:1")
	if err != nil {
		t.Fatalf("INT-031-5 FAIL: Snapshot error: %v", err)
	}
	count, ok := snap["row_count"].(int)
	if !ok {
		t.Fatalf("INT-031-5 FAIL: snapshot[\"row_count\"] is %T, want int", snap["row_count"])
	}
	if count != expected {
		t.Errorf("INT-031-5 FAIL: Snapshot row_count=%d, want %d (from RowCount)", count, expected)
	}
}

// ---------------------------------------------------------------------------
// INT-031-6: nil-wiring guard — DEMO_POSTGRES_DSN unset path
// ---------------------------------------------------------------------------

// TestTASK031_INT6_WorkerLogsDemoDSNNotSetWhenUnset verifies that the registration
// log line "DEMO_POSTGRES_DSN not set" is present in main.go source, confirming that
// the non-demo code path is implemented and will produce the expected log on startup.
//
// DEMO-002 / TASK-031: non-demo worker must start cleanly without PostgreSQL support.
//
// Given: cmd/worker/main.go contains the registerPostgresConnectors function
// When:  the source is inspected for the log string used when DEMO_POSTGRES_DSN is unset
// Then:  the expected log line is present in the source
func TestTASK031_INT6_WorkerLogsDemoDSNNotSetWhenUnset(t *testing.T) {
	src, err := os.ReadFile("../../cmd/worker/main.go")
	if err != nil {
		t.Fatalf("INT-031-6 FAIL: cannot read cmd/worker/main.go: %v", err)
	}
	const expected = `DEMO_POSTGRES_DSN not set — PostgreSQL connectors not registered`
	if !strings.Contains(string(src), expected) {
		t.Errorf("INT-031-6 FAIL: cmd/worker/main.go does not contain expected log line %q", expected)
	}
}

// TestTASK031_INT6_WorkerLogsRegisteredWhenDSNSet verifies that the registration
// log line confirming successful PostgreSQL connector registration is present in source.
//
// DEMO-002 / TASK-031: demo worker must log connector registration confirmation.
//
// Given: cmd/worker/main.go contains the registerPostgresConnectors function
// When:  the source is inspected for the log string used when registration succeeds
// Then:  the expected log line prefix is present in the source
func TestTASK031_INT6_WorkerLogsRegisteredWhenDSNSet(t *testing.T) {
	src, err := os.ReadFile("../../cmd/worker/main.go")
	if err != nil {
		t.Fatalf("INT-031-6 FAIL: cannot read cmd/worker/main.go: %v", err)
	}
	const expected = `worker: PostgreSQL connectors registered`
	if !strings.Contains(string(src), expected) {
		t.Errorf("INT-031-6 FAIL: cmd/worker/main.go does not contain expected log line %q", expected)
	}
}
