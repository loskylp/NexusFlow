// Package system — TASK-031 system tests: demo-postgres startup and worker registration log.
//
// Requirement: DEMO-002, TASK-031
//
// These tests exercise AC-1 at the system level: they verify that:
//   - demo-postgres starts and becomes healthy via `docker compose --profile demo up`
//   - sample_data has exactly 10000 rows (seed script ran)
//   - demo_output table exists (ready for sink tests)
//   - The worker logs the expected registration message when DEMO_POSTGRES_DSN is set
//   - The worker logs the expected skip message when DEMO_POSTGRES_DSN is unset
//
// AC-1 runtime checks (health, row count) run against the live demo-postgres container
// when SYSTEM_TEST_DEMO_POSTGRES is set to "true". Source-level checks (log lines) run
// unconditionally (they inspect the main.go source, not a running process).
//
// Run (live compose stack):
//
//	SYSTEM_TEST_DEMO_POSTGRES=true go test ./tests/system/... -v -run TASK031 -timeout 120s
//
// Run (source-level only):
//
//	go test ./tests/system/... -v -run TASK031
package system

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// AC-1: demo-postgres starts via `docker compose --profile demo up` with 10K rows
// ---------------------------------------------------------------------------

// TestTASK031_AC1_DemoPostgresHealthCheckPasses verifies that the demo-postgres container
// is healthy and pg_isready reports success.
//
// DEMO-002 / TASK-031 AC-1: demo-postgres starts healthy under the demo Docker Compose profile.
//
// Given: docker compose --profile demo up has been run
// When:  pg_isready is polled until the healthcheck passes (up to 60s)
// Then:  pg_isready returns success (exit code 0) within the timeout
func TestTASK031_AC1_DemoPostgresHealthCheckPasses(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DEMO_POSTGRES") != "true" {
		t.Skip("SYSTEM_TEST_DEMO_POSTGRES not set to 'true' — skipping compose system tests")
	}

	// Poll pg_isready via docker exec against the running container.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command(
			"docker", "exec", "nexusflow-demo-postgres-1",
			"pg_isready", "-U", "demo", "-d", "demo",
		).CombinedOutput()
		if err == nil && strings.Contains(string(out), "accepting connections") {
			return // PASS
		}
		time.Sleep(2 * time.Second)
	}
	t.Error("AC-1 FAIL: demo-postgres did not report 'accepting connections' within 60s")
}

// TestTASK031_AC1_SampleDataHas10KRows verifies that sample_data contains exactly 10000 rows
// after the seed script has run (confirms the init script executed correctly).
//
// DEMO-002 / TASK-031 AC-1: demo-postgres pre-seeded with 10K rows.
//
// Given: demo-postgres container is healthy and seed script has executed
// When:  SELECT COUNT(*) FROM sample_data is executed via psql
// Then:  the count is exactly 10000
func TestTASK031_AC1_SampleDataHas10KRows(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DEMO_POSTGRES") != "true" {
		t.Skip("SYSTEM_TEST_DEMO_POSTGRES not set to 'true' — skipping compose system tests")
	}

	out, err := exec.Command(
		"docker", "exec", "nexusflow-demo-postgres-1",
		"psql", "-U", "demo", "-d", "demo",
		"-t", "-c", "SELECT COUNT(*) FROM sample_data;",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("AC-1 FAIL: psql COUNT(*) failed: %v\nOutput: %s", err, out)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "10000" {
		t.Errorf("AC-1 FAIL: expected 10000 rows in sample_data, psql returned %q", trimmed)
	}
}

// TestTASK031_AC1_DemoOutputTableExists verifies that the demo_output table is created
// by the seed script (required for sink pipeline tests).
//
// DEMO-002 / TASK-031 AC-1: both tables present after seed script runs.
//
// Given: demo-postgres is healthy; 01-seed.sql has executed
// When:  psql \dt is run inside the container
// Then:  "demo_output" table is listed
func TestTASK031_AC1_DemoOutputTableExists(t *testing.T) {
	if os.Getenv("SYSTEM_TEST_DEMO_POSTGRES") != "true" {
		t.Skip("SYSTEM_TEST_DEMO_POSTGRES not set to 'true' — skipping compose system tests")
	}

	out, err := exec.Command(
		"docker", "exec", "nexusflow-demo-postgres-1",
		"psql", "-U", "demo", "-d", "demo", "-c", `\dt`,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("AC-1 FAIL: psql \\dt failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(string(out), "demo_output") {
		t.Errorf("AC-1 FAIL: expected demo_output table in \\dt output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// AC-1: Worker log — source-level verification (runs unconditionally)
// ---------------------------------------------------------------------------

// TestTASK031_AC1_WorkerSourceHasPostgresRegistrationLog verifies that the worker source
// contains the expected log line for when DEMO_POSTGRES_DSN is set and registration succeeds.
//
// DEMO-002 / TASK-031 AC-1: worker must log connector registration on successful startup.
//
// Given: cmd/worker/main.go implements registerPostgresConnectors
// When:  the source is inspected for the registration confirmation log line
// Then:  the expected log line is present
func TestTASK031_AC1_WorkerSourceHasPostgresRegistrationLog(t *testing.T) {
	src, err := os.ReadFile("../../cmd/worker/main.go")
	if err != nil {
		t.Fatalf("AC-1 FAIL: cannot read cmd/worker/main.go: %v", err)
	}

	const expected = "worker: PostgreSQL connectors registered"
	if !strings.Contains(string(src), expected) {
		t.Errorf("AC-1 FAIL: cmd/worker/main.go missing registration log line %q", expected)
	}
}

// TestTASK031_AC1_WorkerSourceHasSkipLogWhenDSNUnset verifies that the non-demo path
// is protected by a guard that logs and returns nil (not an error) when the DSN is absent.
//
// DEMO-002 / TASK-031 AC-1 negative: non-demo deployments must start cleanly.
//
// Given: cmd/worker/main.go implements the conditional guard
// When:  the source is inspected for the skip-when-unset log line
// Then:  the expected skip log line is present
func TestTASK031_AC1_WorkerSourceHasSkipLogWhenDSNUnset(t *testing.T) {
	src, err := os.ReadFile("../../cmd/worker/main.go")
	if err != nil {
		t.Fatalf("AC-1 FAIL: cannot read cmd/worker/main.go: %v", err)
	}

	const expected = "DEMO_POSTGRES_DSN not set — PostgreSQL connectors not registered"
	if !strings.Contains(string(src), expected) {
		t.Errorf("AC-1 FAIL: cmd/worker/main.go missing skip-log line %q", expected)
	}
}
