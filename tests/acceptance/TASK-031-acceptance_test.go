// Package acceptance — TASK-031 acceptance tests: Mock-Postgres connector (DataSource + Sink).
//
// Requirement: DEMO-002, ADR-003, ADR-009, TASK-031
//
// Four acceptance criteria are verified:
//
//	AC-1: demo-postgres starts via `docker compose --profile demo up` with 10K pre-seeded rows
//	AC-2: PostgreSQL DataSource can query data from demo-postgres
//	AC-3: PostgreSQL Sink can write data to demo-postgres
//	AC-4: A demo pipeline can use demo-postgres as both DataSource and Sink
//
// AC-1 (Docker Compose healthcheck + seed count) is covered in the system tests
// (TASK-031-system_test.go) and verified against the live container above.
// The acceptance tests here provide full criterion coverage including in-memory
// positive/negative cases and live variants (gated on PG_TEST_DSN).
//
// Run (all — live + in-memory):
//
//	PG_TEST_DSN=postgres://demo:demo@localhost:5433/demo go test ./tests/acceptance/... -v -run TASK031
//
// Run (in-memory only):
//
//	go test ./tests/acceptance/... -v -run TASK031
package acceptance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nxlabs/nexusflow/worker"
)

// ---------------------------------------------------------------------------
// AC-1: demo-postgres starts with 10K pre-seeded rows
// ---------------------------------------------------------------------------

// TestTASK031_AC1_SeedScriptCreates10KRows_LiveRowCount verifies that demo-postgres
// has exactly 10000 rows in sample_data, confirming the seed script ran correctly.
//
// DEMO-002 / TASK-031 AC-1: demo-postgres starts with 10K pre-seeded rows.
//
// Given: demo-postgres is running with the demo Docker Compose profile
// When:  RowCount is queried on the sample_data table via PgxBackendAdapter
// Then:  the count is exactly 10000
func TestTASK031_AC1_SeedScriptCreates10KRows_LiveRowCount(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live AC-1 row count test")
	}

	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("AC-1 FAIL: NewPgxBackendAdapter: %v", err)
	}

	// Given: demo-postgres is running and seeded
	// When:  RowCount("sample_data") is called
	count, err := adapter.RowCount(context.Background(), "sample_data")

	// Then: exactly 10000 rows
	if err != nil {
		t.Fatalf("AC-1 FAIL: RowCount(sample_data): %v", err)
	}
	if count != 10000 {
		t.Errorf("AC-1 FAIL: expected 10000 rows in sample_data, got %d", count)
	}
}

// TestTASK031_AC1_DemoOutputTableExists_Live verifies that the demo_output table exists
// in demo-postgres, required for sink connector pipeline tests.
//
// DEMO-002 / TASK-031 AC-1: both seed tables present after startup.
//
// Given: demo-postgres started with seed script
// When:  RowCount("demo_output") is called
// Then:  no error (table exists); count is 0 (empty on first startup)
func TestTASK031_AC1_DemoOutputTableExists_Live(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live AC-1 demo_output table test")
	}

	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("AC-1 FAIL: NewPgxBackendAdapter: %v", err)
	}

	// Given: demo-postgres running
	// When:  RowCount("demo_output") is called
	count, err := adapter.RowCount(context.Background(), "demo_output")

	// Then: no error (table exists); count must be a non-negative integer
	if err != nil {
		t.Errorf("AC-1 FAIL: demo_output table query returned error (table missing?): %v", err)
	}
	if count < 0 {
		t.Errorf("AC-1 FAIL: demo_output RowCount is negative: %d", count)
	}
}

// TestTASK031_AC1_Negative_NonDemoWorkerStartsWithoutPostgres verifies that a worker
// starting without DEMO_POSTGRES_DSN does NOT attempt to register PostgreSQL connectors.
// This is verified against the source code — the negative startup path is preserved.
//
// [VERIFIER-ADDED]: a trivially permissive implementation that always registers connectors
// (even with nil DSN) would fail non-demo deployments and violate AC-1.
//
// Given: cmd/worker/main.go registers postgres connectors only when DSN is set
// When:  the source is inspected for the conditional guard
// Then:  both the skip-log and the nil-adapter guard are present in source
func TestTASK031_AC1_Negative_NonDemoWorkerStartsWithoutPostgres(t *testing.T) {
	src, err := os.ReadFile("../../cmd/worker/main.go")
	if err != nil {
		t.Fatalf("AC-1 FAIL: cannot read cmd/worker/main.go: %v", err)
	}
	content := string(src)

	checks := []struct {
		label string
		str   string
	}{
		{"skip-when-unset log", "DEMO_POSTGRES_DSN not set — PostgreSQL connectors not registered"},
		{"nil-adapter guard", "NewPgxBackendAdapter returned nil — this is a bug"},
		{"registration confirmation log", "worker: PostgreSQL connectors registered"},
	}
	for _, check := range checks {
		if !containsStr(content, check.str) {
			t.Errorf("AC-1 FAIL (negative): cmd/worker/main.go missing %s: %q", check.label, check.str)
		}
	}
}

// ---------------------------------------------------------------------------
// AC-2: PostgreSQL DataSource can query data from demo-postgres
// ---------------------------------------------------------------------------

// TestTASK031_AC2_DataSourceFetchesRowsFromTable_InMemory verifies AC-2 at the
// component level using InMemoryPostgresDB. Positive case: Fetch returns all seeded records.
//
// DEMO-002 / TASK-031 AC-2: PostgreSQL DataSource can query data from demo-postgres.
//
// Given: a PostgreSQLDataSourceConnector backed by InMemoryPostgresDB seeded with 5 rows
// When:  Fetch is called with config["table"]="sample_data"
// Then:  5 records are returned without error
func TestTASK031_AC2_DataSourceFetchesRowsFromTable_InMemory(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	rows := make([]map[string]any, 5)
	for i := range rows {
		rows[i] = map[string]any{"id": i + 1, "name": fmt.Sprintf("record-%d", i+1)}
	}
	db.Seed("sample_data", rows)

	connector := worker.NewPostgreSQLDataSourceConnector(db)

	// Given: connector backed by in-memory DB seeded with 5 rows
	// When:  Fetch is called with table="sample_data"
	records, err := connector.Fetch(context.Background(), map[string]any{"table": "sample_data"}, nil)

	// Then: 5 records returned without error
	if err != nil {
		t.Fatalf("AC-2 FAIL: Fetch returned unexpected error: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("AC-2 FAIL: expected 5 records, got %d", len(records))
	}
}

// TestTASK031_AC2_DataSourceFetchesRowsFromTable_Negative_MissingConfig verifies
// that Fetch returns an error when neither "table" nor "query" is configured.
//
// DEMO-002 / TASK-031 AC-2 negative: misconfigured DataSource must fail loudly.
//
// Given: a PostgreSQLDataSourceConnector with an empty config map
// When:  Fetch is called
// Then:  a non-nil error is returned
func TestTASK031_AC2_DataSourceFetchesRowsFromTable_Negative_MissingConfig(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	connector := worker.NewPostgreSQLDataSourceConnector(db)

	// Given: no table or query in config
	// When:  Fetch is called
	_, err := connector.Fetch(context.Background(), map[string]any{}, nil)

	// Then: error returned (not nil)
	if err == nil {
		t.Fatal("AC-2 FAIL (negative): Fetch should return error when config has no table or query, got nil")
	}
}

// TestTASK031_AC2_DataSourceFetchesRowsFromTable_Negative_EmptyTableName verifies
// that Fetch returns an error when config["table"] is an empty string.
//
// [VERIFIER-ADDED]: guards against a trivially permissive implementation that generates
// an invalid SQL statement "SELECT * FROM " without detecting the empty table name.
//
// Given: config["table"] is set to ""
// When:  Fetch is called
// Then:  a non-nil error is returned
func TestTASK031_AC2_DataSourceFetchesRowsFromTable_Negative_EmptyTableName(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	connector := worker.NewPostgreSQLDataSourceConnector(db)

	// Given: config["table"] = ""
	// When:  Fetch is called
	_, err := connector.Fetch(context.Background(), map[string]any{"table": ""}, nil)

	// Then: error returned
	if err == nil {
		t.Fatal("AC-2 FAIL (negative): Fetch should return error when table is empty string, got nil")
	}
}

// TestTASK031_AC2_DataSourceFetchesRowsFromTable_Live verifies AC-2 against the live
// demo-postgres container. Skipped unless PG_TEST_DSN is set.
//
// DEMO-002 / TASK-031 AC-2 (live): PostgreSQL DataSource queries real demo-postgres.
//
// Given: demo-postgres running with 10000 rows in sample_data
// When:  Fetch is called with table="sample_data", limit=100
// Then:  100 records are returned; each has the expected columns
func TestTASK031_AC2_DataSourceFetchesRowsFromTable_Live(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live AC-2 test")
	}

	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("AC-2 FAIL: NewPgxBackendAdapter: %v", err)
	}

	connector := worker.NewPostgreSQLDataSourceConnector(adapter)
	cfg := map[string]any{
		"table": "sample_data",
		"limit": float64(100),
	}

	// Given: demo-postgres is healthy and seeded
	// When:  Fetch is called with limit=100
	records, err := connector.Fetch(context.Background(), cfg, nil)

	// Then: 100 records returned; each has expected columns
	if err != nil {
		t.Fatalf("AC-2 FAIL (live): Fetch error: %v", err)
	}
	if len(records) != 100 {
		t.Errorf("AC-2 FAIL (live): expected 100 records (limit=100), got %d", len(records))
	}
	for i, rec := range records {
		for _, field := range []string{"id", "name", "category", "value"} {
			if _, ok := rec[field]; !ok {
				t.Errorf("AC-2 FAIL (live): record[%d] missing field %q: %v", i, field, rec)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// AC-3: PostgreSQL Sink can write data to demo-postgres
// ---------------------------------------------------------------------------

// TestTASK031_AC3_SinkWritesRowsToTable_InMemory verifies AC-3 at the component level
// using InMemoryPostgresDB. Positive case: Write commits all records atomically.
//
// DEMO-002 / ADR-009 / TASK-031 AC-3: PostgreSQL Sink can write data to demo-postgres.
//
// Given: a PostgreSQLSinkConnector backed by InMemoryPostgresDB
// When:  Write is called with 3 records and table="demo_output"
// Then:  no error; 3 committed rows appear in the table
func TestTASK031_AC3_SinkWritesRowsToTable_InMemory(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
		{"id": 3, "name": "carol"},
	}
	cfg := map[string]any{"table": "demo_output"}

	// Given: sink backed by in-memory DB
	// When:  Write is called
	err := sink.Write(context.Background(), cfg, records, "ac3-write:1")

	// Then: no error; 3 rows committed
	if err != nil {
		t.Fatalf("AC-3 FAIL: Write returned unexpected error: %v", err)
	}
	if len(db.Rows("demo_output")) != 3 {
		t.Errorf("AC-3 FAIL: expected 3 committed rows, got %d", len(db.Rows("demo_output")))
	}
}

// TestTASK031_AC3_SinkWritesRowsToTable_Negative_Idempotency verifies AC-3 negative
// case: a second Write with the same executionID must return ErrAlreadyApplied (ADR-003).
//
// DEMO-002 / ADR-003 / TASK-031 AC-3 negative: idempotency guard.
//
// Given: a successful Write has been recorded for executionID "ac3-idem:1"
// When:  Write is called again with the same executionID
// Then:  ErrAlreadyApplied is returned; row count remains 1
func TestTASK031_AC3_SinkWritesRowsToTable_Negative_Idempotency(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{{"id": 1, "name": "alice"}}
	cfg := map[string]any{"table": "demo_output"}

	if err := sink.Write(context.Background(), cfg, records, "ac3-idem:1"); err != nil {
		t.Fatalf("AC-3 FAIL: first Write error: %v", err)
	}

	// Given: executionID "ac3-idem:1" already applied
	// When:  Write is called again with same executionID
	err := sink.Write(context.Background(), cfg, records, "ac3-idem:1")

	// Then: ErrAlreadyApplied returned; row count unchanged
	if !errors.Is(err, worker.ErrAlreadyApplied) {
		t.Errorf("AC-3 FAIL (negative idempotency): expected ErrAlreadyApplied, got %v", err)
	}
	if len(db.Rows("demo_output")) != 1 {
		t.Errorf("AC-3 FAIL (negative idempotency): row count should remain 1, got %d",
			len(db.Rows("demo_output")))
	}
}

// TestTASK031_AC3_SinkWritesRowsToTable_Negative_Atomicity verifies AC-3 negative
// case: when an insert fails mid-batch, the transaction is rolled back and no rows persist
// (ADR-009: all-or-nothing atomicity).
//
// DEMO-002 / ADR-009 / TASK-031 AC-3 negative: atomicity — rollback on failure.
//
// Given: InMemoryPostgresDB configured to fail after the first insert
// When:  Write is called with a 2-record batch
// Then:  Write returns an error; no rows are committed (0 in the table)
func TestTASK031_AC3_SinkWritesRowsToTable_Negative_Atomicity(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.FailAfterRow(1) // fail after first row in the transaction
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(db, dedup)

	records := []map[string]any{
		{"id": 1, "name": "alice"},
		{"id": 2, "name": "bob"},
	}
	cfg := map[string]any{"table": "demo_output"}

	// Given: DB will fail after first insert
	// When:  Write is called
	err := sink.Write(context.Background(), cfg, records, "ac3-atomic:1")

	// Then: error returned; 0 rows committed
	if err == nil {
		t.Fatal("AC-3 FAIL (negative atomicity): Write should return error on forced failure, got nil")
	}
	if len(db.Rows("demo_output")) != 0 {
		t.Errorf("AC-3 FAIL (negative atomicity): expected 0 rows after rollback, got %d (atomicity violation)",
			len(db.Rows("demo_output")))
	}
}

// TestTASK031_AC3_SinkWritesRowsToTable_Negative_MissingTableConfig verifies that
// Write returns an error when config["table"] is absent.
//
// [VERIFIER-ADDED]: a trivially permissive Write that silently passes on missing table
// config would not detect misconfigured pipelines.
//
// Given: Write is called with no "table" key in config
// When:  Write runs
// Then:  a non-nil error is returned; no rows are written
func TestTASK031_AC3_SinkWritesRowsToTable_Negative_MissingTableConfig(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(db, dedup)

	// Given: no "table" in config
	// When:  Write is called
	err := sink.Write(context.Background(), map[string]any{},
		[]map[string]any{{"id": 1}}, "ac3-notbl:1")

	// Then: error returned
	if err == nil {
		t.Fatal("AC-3 FAIL (negative): Write should return error when config has no table key, got nil")
	}
}

// TestTASK031_AC3_SinkWritesRowsToTable_Live verifies AC-3 against the live demo-postgres.
// Skipped unless PG_TEST_DSN is set.
//
// DEMO-002 / ADR-009 / TASK-031 AC-3 (live): sink writes to real demo-postgres.
//
// Given: demo-postgres running with demo_output table accessible
// When:  PostgreSQLSinkConnector.Write is called with 5 records
// Then:  RowCount(demo_output) increases by 5; no error
func TestTASK031_AC3_SinkWritesRowsToTable_Live(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live AC-3 test")
	}

	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("AC-3 FAIL: NewPgxBackendAdapter: %v", err)
	}

	before, err := adapter.RowCount(context.Background(), "demo_output")
	if err != nil {
		t.Fatalf("AC-3 FAIL: RowCount before: %v", err)
	}

	dedup := worker.NewInMemoryDedupStore()
	sink := worker.NewPostgreSQLSinkConnector(adapter, dedup)

	uniqueExecID := fmt.Sprintf("ac3-live-%d", time.Now().UnixNano())
	records := []map[string]any{
		{"data": `{"id":1,"source":"ac3-live"}`},
		{"data": `{"id":2,"source":"ac3-live"}`},
		{"data": `{"id":3,"source":"ac3-live"}`},
		{"data": `{"id":4,"source":"ac3-live"}`},
		{"data": `{"id":5,"source":"ac3-live"}`},
	}
	cfg := map[string]any{"table": "demo_output"}

	// Given: live demo-postgres with demo_output table
	// When:  Write is called with 5 records
	err = sink.Write(context.Background(), cfg, records, uniqueExecID)

	// Then: no error; row count increases by 5
	if err != nil {
		t.Fatalf("AC-3 FAIL (live): Write error: %v", err)
	}

	after, err := adapter.RowCount(context.Background(), "demo_output")
	if err != nil {
		t.Fatalf("AC-3 FAIL (live): RowCount after: %v", err)
	}
	if after != before+5 {
		t.Errorf("AC-3 FAIL (live): expected RowCount to increase by 5 (before=%d, after=%d)", before, after)
	}
}

// ---------------------------------------------------------------------------
// AC-4: A demo pipeline can use demo-postgres as both DataSource and Sink
// ---------------------------------------------------------------------------

// TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_RegistryResolution verifies
// AC-4 at the registry level: after RegisterPostgreSQLConnectors, both "postgres"
// DataSource and "postgres" Sink can be resolved from the DefaultConnectorRegistry.
//
// DEMO-002 / TASK-031 AC-4: pipeline definition with connector_type "postgres" resolves both.
//
// Given: a DefaultConnectorRegistry with PostgreSQL connectors registered
// When:  DataSource("postgres") and Sink("postgres") are called
// Then:  both return a non-nil connector without error
func TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_RegistryResolution(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	db := worker.NewInMemoryPostgresDB()

	// Given: RegisterPostgreSQLConnectors wired with in-memory backend
	worker.RegisterPostgreSQLConnectors(reg, db)

	// When: registry queried for "postgres" connectors
	ds, err := reg.DataSource("postgres")
	if err != nil {
		t.Errorf("AC-4 FAIL: DataSource(\"postgres\") returned error: %v", err)
	}
	if ds == nil {
		t.Error("AC-4 FAIL: DataSource(\"postgres\") returned nil connector")
	}

	sk, err := reg.Sink("postgres")
	if err != nil {
		t.Errorf("AC-4 FAIL: Sink(\"postgres\") returned error: %v", err)
	}
	if sk == nil {
		t.Error("AC-4 FAIL: Sink(\"postgres\") returned nil connector")
	}
}

// TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_TypeStrings verifies that
// the connectors returned by the registry report "postgres" as their Type().
//
// DEMO-002 / TASK-031 AC-4: pipeline executor matches connector by type string.
//
// Given: PostgreSQL connectors registered
// When:  Type() is called on each resolved connector
// Then:  both return "postgres"
func TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_TypeStrings(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	db := worker.NewInMemoryPostgresDB()
	worker.RegisterPostgreSQLConnectors(reg, db)

	ds, _ := reg.DataSource("postgres")
	if ds.Type() != "postgres" {
		t.Errorf("AC-4 FAIL: DataSource Type()=%q, want \"postgres\"", ds.Type())
	}

	sk, _ := reg.Sink("postgres")
	if sk.Type() != "postgres" {
		t.Errorf("AC-4 FAIL: Sink Type()=%q, want \"postgres\"", sk.Type())
	}
}

// TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_Negative_Unregistered
// verifies AC-4 negative case: when PostgreSQL connectors are NOT registered,
// DataSource("postgres") and Sink("postgres") must return errors.
//
// [VERIFIER-ADDED]: a trivially permissive registry would satisfy neither the positive
// nor the negative case correctly.
//
// Given: a fresh registry with NO PostgreSQL connectors registered
// When:  DataSource("postgres") and Sink("postgres") are queried
// Then:  both return errors
func TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_Negative_Unregistered(t *testing.T) {
	reg := worker.NewDefaultConnectorRegistry()
	// RegisterPostgreSQLConnectors intentionally NOT called.

	if _, err := reg.DataSource("postgres"); err == nil {
		t.Error("AC-4 FAIL (negative): DataSource(\"postgres\") should return error when not registered, got nil")
	}
	if _, err := reg.Sink("postgres"); err == nil {
		t.Error("AC-4 FAIL (negative): Sink(\"postgres\") should return error when not registered, got nil")
	}
}

// TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_EndToEnd_InMemory verifies
// AC-4 at the pipeline level (in-memory): a complete Fetch-then-Write round-trip using
// both registered connectors exercises the full postgres pipeline path.
//
// DEMO-002 / TASK-031 AC-4: demo pipeline uses postgres for both DataSource and Sink.
//
// Given: InMemoryPostgresDB seeded with 3 rows in "sample_data"; "demo_output" empty
// When:  DataSource.Fetch reads from sample_data, then Sink.Write writes to demo_output
// Then:  no errors; 3 rows committed in demo_output
func TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_EndToEnd_InMemory(t *testing.T) {
	db := worker.NewInMemoryPostgresDB()
	db.Seed("sample_data", []map[string]any{
		{"id": 1, "name": "alice", "category": "alpha"},
		{"id": 2, "name": "bob", "category": "beta"},
		{"id": 3, "name": "carol", "category": "gamma"},
	})

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterPostgreSQLConnectors(reg, db)

	ds, err := reg.DataSource("postgres")
	if err != nil {
		t.Fatalf("AC-4 FAIL: DataSource registry lookup: %v", err)
	}
	sk, err := reg.Sink("postgres")
	if err != nil {
		t.Fatalf("AC-4 FAIL: Sink registry lookup: %v", err)
	}

	// Phase 1 — DataSource Fetch
	// Given: sample_data has 3 rows
	// When:  Fetch is called with table="sample_data"
	records, err := ds.Fetch(
		context.Background(),
		map[string]any{"table": "sample_data"},
		nil,
	)

	// Then: 3 records returned
	if err != nil {
		t.Fatalf("AC-4 FAIL: DataSource.Fetch error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("AC-4 FAIL: expected 3 records from DataSource, got %d", len(records))
	}

	// Phase 2 — Sink Write
	// When:  Sink.Write is called with all fetched records
	err = sk.Write(
		context.Background(),
		map[string]any{"table": "demo_output"},
		records,
		"ac4-e2e-inmem:1",
	)

	// Then: no error; all records committed
	if err != nil {
		t.Fatalf("AC-4 FAIL: Sink.Write error: %v", err)
	}
	if len(db.Rows("demo_output")) != 3 {
		t.Errorf("AC-4 FAIL: expected 3 rows in demo_output after pipeline, got %d",
			len(db.Rows("demo_output")))
	}
}

// TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_EndToEnd_Live verifies AC-4
// against the live demo-postgres container. Skipped unless PG_TEST_DSN is set.
//
// DEMO-002 / TASK-031 AC-4 (live): full pipeline with postgres at both ends.
//
// Given: demo-postgres running; sample_data has 10000 rows; demo_output is accessible
// When:  DataSource.Fetch(limit=10) reads from sample_data, Sink.Write writes to demo_output
// Then:  no errors; RowCount(demo_output) increases by 10
func TestTASK031_AC4_PipelineCanUsePostgresBothSourceAndSink_EndToEnd_Live(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set — skipping live AC-4 end-to-end test")
	}

	adapter, err := worker.NewPgxBackendAdapter(dsn)
	if err != nil {
		t.Fatalf("AC-4 FAIL: NewPgxBackendAdapter: %v", err)
	}

	before, err := adapter.RowCount(context.Background(), "demo_output")
	if err != nil {
		t.Fatalf("AC-4 FAIL: RowCount(demo_output) before: %v", err)
	}

	reg := worker.NewDefaultConnectorRegistry()
	worker.RegisterPostgreSQLConnectors(reg, adapter)

	ds, err := reg.DataSource("postgres")
	if err != nil {
		t.Fatalf("AC-4 FAIL: DataSource registry lookup: %v", err)
	}
	sk, err := reg.Sink("postgres")
	if err != nil {
		t.Fatalf("AC-4 FAIL: Sink registry lookup: %v", err)
	}

	// Phase 1 — DataSource Fetch (limit 10 to keep the test fast)
	// Given: sample_data has 10000 rows
	// When:  Fetch with limit=10
	records, err := ds.Fetch(
		context.Background(),
		map[string]any{"table": "sample_data", "limit": float64(10)},
		nil,
	)
	if err != nil {
		t.Fatalf("AC-4 FAIL (live): DataSource.Fetch error: %v", err)
	}
	if len(records) != 10 {
		t.Errorf("AC-4 FAIL (live): expected 10 records from DataSource, got %d", len(records))
	}

	// Phase 2 — Sink Write (re-map each record to JSONB-compatible demo_output schema)
	// demo_output has a "data" JSONB column; we write the record as a JSON string.
	uniqueExecID := fmt.Sprintf("ac4-live-%d", time.Now().UnixNano())
	sinkRecords := make([]map[string]any, len(records))
	for i, rec := range records {
		sinkRecords[i] = map[string]any{
			"data": fmt.Sprintf(`{"source":"sample_data","id":%v}`, rec["id"]),
		}
	}

	err = sk.Write(
		context.Background(),
		map[string]any{"table": "demo_output"},
		sinkRecords,
		uniqueExecID,
	)
	if err != nil {
		t.Fatalf("AC-4 FAIL (live): Sink.Write error: %v", err)
	}

	after, err := adapter.RowCount(context.Background(), "demo_output")
	if err != nil {
		t.Fatalf("AC-4 FAIL (live): RowCount(demo_output) after: %v", err)
	}
	if after != before+10 {
		t.Errorf("AC-4 FAIL (live): expected RowCount to increase by 10 (before=%d, after=%d)", before, after)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// containsStr reports whether s contains the given substring. Reuses accept-level helper.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
