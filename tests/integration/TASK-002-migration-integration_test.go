// Integration tests for TASK-002: Database schema and migration foundation
// REQ-009, REQ-019, REQ-020, ADR-008
//
// Tests component seams: RunMigrations/db.New against a live PostgreSQL database.
// These are integration tests — they validate the migration subsystem at its boundary.
// They require a running PostgreSQL instance at NEXUSFLOW_TEST_DSN.
//
// Run with:
//
//	NEXUSFLOW_TEST_DSN=postgresql://nexusflow:nexusflow_dev@localhost:5432/nexusflow?sslmode=disable \
//	  go test ./tests/integration/... -v -run TASK002
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/nxlabs/nexusflow/internal/db"
)

// dsn returns the test database DSN from the environment, or skips the test.
func dsn(t *testing.T) string {
	t.Helper()
	v := os.Getenv("NEXUSFLOW_TEST_DSN")
	if v == "" {
		t.Skip("NEXUSFLOW_TEST_DSN not set — skipping integration test")
	}
	return v
}

// TestTASK002_AC1_MigrationsApplyCleanly verifies that RunMigrations applies the initial
// schema to a fresh PostgreSQL database, creating all expected tables and schema_migrations entry.
//
// REQ-009, ADR-008
// Given: a fresh PostgreSQL database (no schema)
// When:  db.New is called with a valid DSN
// Then:  all 7 core tables exist; schema_migrations records version=1, dirty=false
func TestTASK002_AC1_MigrationsApplyCleanly(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-1: db.New failed: %v", err)
	}
	defer pool.Close()

	// Verify schema_migrations table records the applied version
	var version int
	var dirty bool
	row := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1")
	if err := row.Scan(&version, &dirty); err != nil {
		t.Fatalf("AC-1: cannot query schema_migrations: %v", err)
	}
	if version != 1 {
		t.Errorf("AC-1: expected migration version=1, got %d", version)
	}
	if dirty {
		t.Errorf("AC-1: migration is marked dirty=true; migration did not complete cleanly")
	}
	t.Logf("AC-1: schema_migrations: version=%d dirty=%v", version, dirty)

	// Verify all 7 expected tables exist (ADR-008 data model)
	expectedTables := []string{
		"users", "workers", "pipelines", "pipeline_chains",
		"tasks", "task_state_log", "task_logs",
	}
	for _, tbl := range expectedTables {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT FROM information_schema.tables
				WHERE table_schema='public' AND table_name=$1
			)`, tbl,
		).Scan(&exists)
		if err != nil {
			t.Errorf("AC-1: error checking table %q: %v", tbl, err)
			continue
		}
		if !exists {
			t.Errorf("AC-1: expected table %q is MISSING from schema", tbl)
		} else {
			t.Logf("AC-1: table %q: EXISTS", tbl)
		}
	}

	// Verify default partition for task_logs exists
	var partExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema='public' AND table_name='task_logs_default'
		)`,
	).Scan(&partExists); err != nil {
		t.Errorf("AC-1: error checking task_logs_default: %v", err)
	} else if !partExists {
		t.Errorf("AC-1: task_logs_default partition is MISSING")
	} else {
		t.Log("AC-1: task_logs_default partition: EXISTS")
	}

	// Verify state transition trigger exists
	var triggerExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT FROM information_schema.triggers
			WHERE trigger_name='trg_task_state_transition' AND event_object_table='task_state_log'
		)`,
	).Scan(&triggerExists); err != nil {
		t.Errorf("AC-1: error checking trigger: %v", err)
	} else if !triggerExists {
		t.Errorf("AC-1: trg_task_state_transition trigger is MISSING")
	} else {
		t.Log("AC-1: trg_task_state_transition trigger: EXISTS")
	}
}

// TestTASK002_AC1_MigrationsAreIdempotent verifies that calling RunMigrations a second time
// (when schema is already at version 1) returns nil without error (ErrNoChange handled).
//
// REQ-009, ADR-008
// Given: a database already at migration version 1
// When:  RunMigrations is called again
// Then:  no error is returned (ErrNoChange is swallowed)
func TestTASK002_AC1_MigrationsAreIdempotent(t *testing.T) {
	testDSN := dsn(t)
	if err := db.RunMigrations(testDSN); err != nil {
		t.Errorf("AC-1 idempotency: RunMigrations returned error on already-migrated DB: %v", err)
	} else {
		t.Log("AC-1 idempotency: second RunMigrations call returned nil (ErrNoChange handled)")
	}
}

// TestTASK002_AC4_StateTransitionConstraint_ValidTransitions verifies that the
// enforce_task_state_transition trigger allows all valid (from_state, to_state) pairs.
//
// REQ-009, ADR-008 Domain Invariant 1
// Given: a task exists in the database
// When:  a valid state transition is recorded in task_state_log
// Then:  the INSERT succeeds
func TestTASK002_AC4_StateTransitionConstraint_ValidTransitions(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-4: db.New failed: %v", err)
	}
	defer pool.Close()

	// Insert prerequisite data
	const (
		userID     = "11111111-1111-1111-1111-000000000001"
		pipelineID = "11111111-1111-1111-1111-000000000002"
		taskID     = "11111111-1111-1111-1111-000000000003"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role) VALUES ($1, 'ac4testuser', 'hash', 'user')
		ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pipelines (id, name, user_id) VALUES ($1, 'ac4pipe', $2)
		ON CONFLICT (id) DO NOTHING`, pipelineID, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test pipeline: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id) VALUES ($1, $2, $3, 'submitted', 'exec-ac4')
		ON CONFLICT (id) DO NOTHING`, taskID, pipelineID, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test task: %v", err)
	}

	validTransitions := [][2]string{
		{"submitted", "queued"},
		{"queued", "assigned"},
		{"assigned", "running"},
		{"running", "completed"},
		{"assigned", "queued"},   // failover reassignment
		{"running", "failed"},
		{"failed", "queued"},     // retry
		{"queued", "cancelled"},
		{"assigned", "cancelled"},
		{"running", "cancelled"},
	}
	for _, tr := range validTransitions {
		from, to := tr[0], tr[1]
		_, err := pool.Exec(ctx,
			`INSERT INTO task_state_log (task_id, from_state, to_state) VALUES ($1, $2, $3)`,
			taskID, from, to,
		)
		if err != nil {
			t.Errorf("AC-4: valid transition %s->%s was REJECTED (should be accepted): %v", from, to, err)
		} else {
			t.Logf("AC-4: valid transition %s->%s: accepted", from, to)
		}
	}
}

// TestTASK002_AC4_StateTransitionConstraint_InvalidTransitions verifies that the
// enforce_task_state_transition trigger rejects invalid (from_state, to_state) pairs.
//
// REQ-009, ADR-008 Domain Invariant 1
// Given: a task exists in the database
// When:  an invalid state transition is recorded in task_state_log
// Then:  the INSERT fails with a check_violation error
func TestTASK002_AC4_StateTransitionConstraint_InvalidTransitions(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-4: db.New failed: %v", err)
	}
	defer pool.Close()

	// Insert prerequisite data
	const (
		userID     = "22222222-2222-2222-2222-000000000001"
		pipelineID = "22222222-2222-2222-2222-000000000002"
		taskID     = "22222222-2222-2222-2222-000000000003"
	)
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, password_hash, role) VALUES ($1, 'ac4neguser', 'hash', 'user')
		ON CONFLICT (id) DO NOTHING`, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO pipelines (id, name, user_id) VALUES ($1, 'ac4negpipe', $2)
		ON CONFLICT (id) DO NOTHING`, pipelineID, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test pipeline: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id) VALUES ($1, $2, $3, 'submitted', 'exec-ac4neg')
		ON CONFLICT (id) DO NOTHING`, taskID, pipelineID, userID); err != nil {
		t.Fatalf("AC-4: cannot insert test task: %v", err)
	}

	// [VERIFIER-ADDED] Each invalid transition must be rejected — terminal states have no forward transitions
	invalidTransitions := [][2]string{
		{"completed", "queued"},    // the explicit AC-4 example
		{"completed", "failed"},
		{"completed", "running"},
		{"cancelled", "running"},
		{"cancelled", "queued"},
		{"submitted", "running"},   // must go submitted->queued->assigned->running
		{"submitted", "completed"},
		{"queued", "completed"},    // must go through assigned->running
		{"running", "submitted"},   // no backward transitions
	}
	for _, tr := range invalidTransitions {
		from, to := tr[0], tr[1]
		_, err := pool.Exec(ctx,
			`INSERT INTO task_state_log (task_id, from_state, to_state) VALUES ($1, $2, $3)`,
			taskID, from, to,
		)
		if err == nil {
			t.Errorf("AC-4: invalid transition %s->%s was ACCEPTED (should be rejected)", from, to)
		} else {
			t.Logf("AC-4: invalid transition %s->%s: correctly rejected: %v", from, to, err)
		}
	}
}

// TestTASK002_AC5_SchemaMatchesADR008 verifies that the schema created by migration 000001
// matches the data model specified in ADR-008 (column names, types, constraints).
//
// REQ-009, ADR-008
// Given: migration 000001 has been applied
// When:  the information_schema is queried for each expected column
// Then:  all ADR-008 data model fields are present with the correct data types
func TestTASK002_AC5_SchemaMatchesADR008(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-5: db.New failed: %v", err)
	}
	defer pool.Close()

	// ADR-008 Core data model column expectations
	// Format: [table, column, expected_udt_name_substring]
	type colExpect struct{ table, column, typeHint string }
	expectations := []colExpect{
		// User { id, username, passwordHash, role, active, createdAt }
		{"users", "id", "uuid"},
		{"users", "username", "text"},
		{"users", "password_hash", "text"},
		{"users", "role", "text"},
		{"users", "active", "bool"},
		{"users", "created_at", "timestamptz"},

		// Pipeline { id, name, userId, dataSourceConfig, processConfig, sinkConfig, schemaMappings, createdAt, updatedAt }
		{"pipelines", "id", "uuid"},
		{"pipelines", "name", "text"},
		{"pipelines", "user_id", "uuid"},
		{"pipelines", "data_source_config", "jsonb"},
		{"pipelines", "process_config", "jsonb"},
		{"pipelines", "sink_config", "jsonb"},
		{"pipelines", "created_at", "timestamptz"},
		{"pipelines", "updated_at", "timestamptz"},

		// PipelineChain { id, name, userId, pipelineIds (ordered), createdAt }
		{"pipeline_chains", "id", "uuid"},
		{"pipeline_chains", "name", "text"},
		{"pipeline_chains", "user_id", "uuid"},
		{"pipeline_chains", "pipeline_ids", "_uuid"},
		{"pipeline_chains", "created_at", "timestamptz"},

		// Task { id, pipelineId, chainId?, userId, status, retryConfig, retryCount, executionId, workerId?, input, createdAt, updatedAt }
		{"tasks", "id", "uuid"},
		{"tasks", "pipeline_id", "uuid"},
		{"tasks", "chain_id", "uuid"},
		{"tasks", "user_id", "uuid"},
		{"tasks", "status", "text"},
		{"tasks", "retry_config", "jsonb"},
		{"tasks", "retry_count", "int4"},
		{"tasks", "execution_id", "text"},
		{"tasks", "worker_id", "text"},
		{"tasks", "input", "jsonb"},
		{"tasks", "created_at", "timestamptz"},
		{"tasks", "updated_at", "timestamptz"},

		// TaskStateLog { id, taskId, fromState, toState, reason, timestamp }
		{"task_state_log", "id", "uuid"},
		{"task_state_log", "task_id", "uuid"},
		{"task_state_log", "from_state", "text"},
		{"task_state_log", "to_state", "text"},
		{"task_state_log", "reason", "text"},
		{"task_state_log", "timestamp", "timestamptz"},

		// Worker { id, tags, status, lastHeartbeat, registeredAt }
		{"workers", "id", "text"},
		{"workers", "tags", "_text"},
		{"workers", "status", "text"},
		{"workers", "last_heartbeat", "timestamptz"},
		{"workers", "registered_at", "timestamptz"},

		// TaskLog { id, taskId, line, level, timestamp }
		{"task_logs", "id", "uuid"},
		{"task_logs", "task_id", "uuid"},
		{"task_logs", "line", "text"},
		{"task_logs", "level", "text"},
		{"task_logs", "timestamp", "timestamptz"},
	}

	for _, ex := range expectations {
		var udtName string
		err := pool.QueryRow(ctx,
			`SELECT udt_name FROM information_schema.columns
			 WHERE table_schema='public' AND table_name=$1 AND column_name=$2`,
			ex.table, ex.column,
		).Scan(&udtName)
		if err != nil {
			t.Errorf("AC-5: column %s.%s NOT FOUND in schema: %v", ex.table, ex.column, err)
			continue
		}
		if udtName != ex.typeHint {
			t.Errorf("AC-5: column %s.%s has type %q, expected %q", ex.table, ex.column, udtName, ex.typeHint)
		} else {
			t.Logf("AC-5: %s.%s: type=%s OK", ex.table, ex.column, udtName)
		}
	}

	// Verify CHECK constraint on users.role
	var roleConstr string
	if err := pool.QueryRow(ctx,
		`SELECT check_clause FROM information_schema.check_constraints
		 WHERE constraint_name LIKE '%users%role%' OR constraint_name LIKE '%role%'
		 LIMIT 1`,
	).Scan(&roleConstr); err != nil {
		// Try pg_constraint directly
		var conDef string
		pgErr := pool.QueryRow(ctx,
			`SELECT pg_get_constraintdef(c.oid)
			 FROM pg_constraint c JOIN pg_class t ON c.conrelid=t.oid
			 WHERE t.relname='users' AND c.contype='c' AND c.conname LIKE '%role%'`,
		).Scan(&conDef)
		if pgErr != nil {
			t.Logf("AC-5 OBS: could not verify role CHECK constraint via pg_constraint: %v", pgErr)
		} else {
			t.Logf("AC-5: users.role CHECK constraint: %s", conDef)
		}
	} else {
		t.Logf("AC-5: users.role CHECK constraint: %s", roleConstr)
	}

	// Verify task_logs is partitioned (cast relkind to text to avoid binary char scan issue)
	var relkind string
	if err := pool.QueryRow(ctx,
		`SELECT relkind::text FROM pg_class WHERE relname='task_logs' AND relnamespace=(SELECT oid FROM pg_namespace WHERE nspname='public')`,
	).Scan(&relkind); err != nil {
		t.Errorf("AC-5: cannot check task_logs partitioning: %v", err)
	} else if relkind != "p" {
		t.Errorf("AC-5: task_logs relkind=%q, expected 'p' (partitioned)", relkind)
	} else {
		t.Log("AC-5: task_logs is a partitioned table (relkind=p): OK")
	}

	// Verify foreign key: pipelines.user_id -> users.id
	var fkCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM information_schema.referential_constraints rc
		 JOIN information_schema.key_column_usage kcu ON rc.constraint_name=kcu.constraint_name
		 WHERE kcu.table_name='pipelines' AND kcu.column_name='user_id'`,
	).Scan(&fkCount); err != nil || fkCount == 0 {
		t.Errorf("AC-5: FK pipelines.user_id -> users.id is MISSING (count=%d, err=%v)", fkCount, err)
	} else {
		t.Log("AC-5: FK pipelines.user_id -> users.id: EXISTS")
	}

	// [VERIFIER-ADDED] Verify ADR-008 domain invariant: no cascade on user deactivation
	// REQ-020: "deactivation does not cancel in-flight tasks"
	// Check that tasks.user_id FK does NOT have ON DELETE CASCADE
	var deleteRule string
	if err := pool.QueryRow(ctx,
		`SELECT rc.delete_rule
		 FROM information_schema.referential_constraints rc
		 JOIN information_schema.key_column_usage kcu ON rc.constraint_name=kcu.constraint_name
		 WHERE kcu.table_name='tasks' AND kcu.column_name='user_id'`,
	).Scan(&deleteRule); err != nil {
		t.Logf("AC-5: could not check tasks.user_id delete rule: %v", err)
	} else if deleteRule == "CASCADE" {
		t.Errorf("AC-5 REQ-020: tasks.user_id has ON DELETE CASCADE — must NOT cascade; deactivation must not cancel in-flight tasks")
	} else {
		t.Logf("AC-5 REQ-020: tasks.user_id delete_rule=%q (not CASCADE): OK", deleteRule)
	}
}

// TestTASK002_AC5_IndexesExist verifies that the performance indexes defined in the migration
// are present in the schema. These are not in ADR-008 directly but are required by AC-5
// "schema matches the data model" in the context of the task plan's stated fields.
//
// [VERIFIER-ADDED] REQ-009, ADR-008
func TestTASK002_AC5_IndexesExist(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-5: db.New failed: %v", err)
	}
	defer pool.Close()

	expectedIndexes := []string{
		"idx_pipelines_user_id",
		"idx_pipeline_chains_user_id",
		"idx_tasks_user_id",
		"idx_tasks_pipeline_id",
		"idx_tasks_status",
		"idx_tasks_worker_id",
		"idx_task_state_log_task_id",
		"idx_task_logs_task_id",
		"idx_task_logs_timestamp",
	}
	for _, idx := range expectedIndexes {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT FROM pg_indexes WHERE schemaname='public' AND indexname=$1)`, idx,
		).Scan(&exists); err != nil || !exists {
			t.Errorf("AC-5: expected index %q is MISSING", idx)
		} else {
			t.Logf("AC-5: index %q: EXISTS", idx)
		}
	}
}

// TestTASK002_AC4_StateTransitionConstraint_TasksStatusValues verifies that the tasks.status
// CHECK constraint permits only the documented set of status values.
//
// [VERIFIER-ADDED] REQ-009, ADR-008
// Given: a pipeline and user exist
// When:  a task is inserted with an invalid status value
// Then:  the INSERT is rejected with a check_violation
func TestTASK002_AC4_TasksStatusCheckConstraint(t *testing.T) {
	testDSN := dsn(t)
	ctx := context.Background()

	pool, err := db.New(ctx, testDSN)
	if err != nil {
		t.Fatalf("AC-4: db.New failed: %v", err)
	}
	defer pool.Close()

	const (
		userID     = "33333333-3333-3333-3333-000000000001"
		pipelineID = "33333333-3333-3333-3333-000000000002"
	)
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, password_hash, role) VALUES ($1, 'ac4statususer', 'hash', 'user') ON CONFLICT (id) DO NOTHING`,
		userID); err != nil {
		t.Fatalf("AC-4: cannot insert user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO pipelines (id, name, user_id) VALUES ($1, 'ac4statuspipe', $2) ON CONFLICT (id) DO NOTHING`,
		pipelineID, userID); err != nil {
		t.Fatalf("AC-4: cannot insert pipeline: %v", err)
	}

	// Valid statuses — must all be accepted (use gen_random_uuid() to avoid PK conflicts on re-runs)
	validStatuses := []string{"submitted", "queued", "assigned", "running", "completed", "failed", "cancelled"}
	for _, status := range validStatuses {
		_, err := pool.Exec(ctx,
			`INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id) VALUES (gen_random_uuid(), $1, $2, $3, $4)`,
			pipelineID, userID, status, fmt.Sprintf("exec-%s", status),
		)
		if err != nil {
			t.Errorf("AC-4: valid status %q was REJECTED: %v", status, err)
		} else {
			t.Logf("AC-4: valid status %q: accepted", status)
		}
	}

	// Invalid status — must be rejected
	_, err = pool.Exec(ctx,
		`INSERT INTO tasks (id, pipeline_id, user_id, status, execution_id) VALUES (gen_random_uuid(), $1, $2, 'pending', 'exec-invalid')`,
		pipelineID, userID,
	)
	if err == nil {
		t.Error("AC-4: invalid status 'pending' was ACCEPTED (should be rejected by CHECK constraint)")
	} else {
		t.Logf("AC-4: invalid status 'pending': correctly rejected: %v", err)
	}
}
