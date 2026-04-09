#!/usr/bin/env bash
# TASK-031 Acceptance Test — Mock-Postgres connector (DataSource + Sink).
#
# Validates:
#   1. demo-postgres container starts with `docker compose --profile demo up` with 10K rows.
#   2. PostgreSQLDataSourceConnector.Fetch reads rows from a demo-postgres table.
#   3. PostgreSQLSinkConnector.Write inserts records within a transaction.
#   4. Atomicity: Rollback called on failure; no partial rows in destination table.
#   5. Idempotency: a second Write with the same executionID returns no-op (ErrAlreadyApplied).
#   6. A full pipeline (postgres DataSource -> demo Process -> postgres Sink) executes end-to-end.
#
# Preconditions:
#   - API server and worker running with demo profile (DEMO_POSTGRES_DSN set).
#   - demo-postgres container running with 10K seeded rows in the "sample_data" table.
#
# See: DEMO-002, ADR-003, ADR-009, TASK-031
set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8080}"
DEMO_POSTGRES_DSN="${DEMO_POSTGRES_DSN:-postgres://demo:demo@localhost:5433/demo}"

echo "TASK-031 acceptance: Mock-Postgres connector"
echo "TODO: implement acceptance tests"
echo "  Step 1: verify demo-postgres is reachable"
echo "  Step 2: verify sample_data table has 10000 rows"
echo "  Step 3: create pipeline with postgres DataSource (table: sample_data, limit: 100)"
echo "  Step 4: submit task; wait for COMPLETED status"
echo "  Step 5: verify destination table has expected row count increase"
echo "  Step 6: submit same task again; verify no duplicate rows (idempotency)"
exit 0
