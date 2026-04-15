# Routing Instruction

**To:** @nexus-verifier
**Phase:** EXECUTION -- Cycle 4
**Task:** Verify TASK-031 (Mock-Postgres with seed data -- PostgreSQL DataSource + Sink connectors) against all acceptance criteria and author + execute the demo script against the live demo-postgres container.
**Iteration:** 1 of 3
**Verifier mode:** Initial verification
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| TASK-031 spec + acceptance criteria | [process/planner/task-plan.md -- TASK-031](../planner/task-plan.md#task-031-demo-infrastructure----mock-postgres-with-seed-data) | Acceptance criteria, demo script path, dependencies |
| Builder handoff | [process/builder/handoff-notes/TASK-031-handoff.md](../builder/handoff-notes/TASK-031-handoff.md) | What was built, deviations, integration check steps |
| DEMO-002 requirement | [process/analyst/requirements.md#demo-002](../analyst/requirements.md#demo-002) | Source requirement for Mock-Postgres demo connector |
| ADR-009 (demo connectors) | [process/architect/adrs/adr-009-demo-connectors.md](../architect/adrs/adr-009-demo-connectors.md) | Architectural decision for Mock-Postgres |
| TASK-030 precedent | [process/verifier/verification-reports/TASK-030-report.md](../verifier/verification-reports/TASK-030-report.md) | Peer demo-infra connector verified last -- same pattern (backend interface + adapter + nil-wired registration) |

---

## Skills required

- [`.claude/skills/bash-execution.md`](../../.claude/skills/bash-execution.md) -- absolute paths; no `cd dir && cmd`
- [`.claude/skills/demo-script-execution.md`](../../.claude/skills/demo-script-execution.md) -- author `tests/demo/TASK-031-demo.md` and execute it
- [`.claude/skills/commit-discipline.md`](../../.claude/skills/commit-discipline.md) -- commit the Verification Report when complete
- [`.claude/skills/traceability-links.md`](../../.claude/skills/traceability-links.md) -- link Verification Report back to ACs and requirements

---

## Context

TASK-031 delivers the second demo-infrastructure connector pair (the first was TASK-030 MinIO Fake-S3, verified PASS earlier today). The pattern mirrors TASK-030: a `postgresBackend` interface with an in-memory implementation for unit tests and a `PgxBackendAdapter` for the demo profile, registered in `cmd/worker/main.go` only when `DEMO_POSTGRES_DSN` is set.

**Builder-reported state (commit e4d5d87):**
- PostgreSQLDataSourceConnector + PostgreSQLSinkConnector implemented in `worker/connector_postgres.go` over the `postgresBackend` interface
- `PgxBackendAdapter` in `worker/connector_postgres_pgx.go` (pgx/v5) used for the demo profile
- 14 unit tests pass in `worker/connector_postgres_test.go`
- `cmd/worker/main.go` lines 109-113 and 244-260: `registerPostgresConnectors` called unconditionally at startup; logs "DEMO_POSTGRES_DSN not set -- PostgreSQL connectors not registered" when env is empty (non-demo deployments unaffected); explicit nil-guard on NewPgxBackendAdapter
- `deploy/demo-postgres/01-seed.sql` seeds 10K rows via `generate_series`; `docker-compose.yml` wires healthcheck + volume mount on the `demo-postgres` service under the `demo` profile

**Nil-wiring verified by Orchestrator before dispatch** -- `registerPostgresConnectors` is invoked from `main.go`, not just declared. Re-verify during runtime integration check.

**Acceptance focus (from task-plan.md TASK-031):**
1. `docker compose --profile demo up` brings `demo-postgres` to healthy with 10K pre-seeded rows
2. PostgreSQLDataSourceConnector can query data from demo-postgres (live, not in-memory)
3. PostgreSQLSinkConnector can write data to demo-postgres (live, not in-memory)
4. A demo pipeline can use demo-postgres as both DataSource and Sink

**Integration verification to run:**
1. `docker compose --profile demo up` -- worker log should contain a "PostgreSQL connectors registered" line (or equivalent) with the DSN redacted; `demo-postgres` healthcheck must go green; `SELECT count(*) FROM <seed_table>` must return 10000
2. With `DEMO_POSTGRES_DSN` unset -- worker log contains "DEMO_POSTGRES_DSN not set -- PostgreSQL connectors not registered" and startup completes normally
3. Exercise TASK-018 (sink atomicity/idempotency) and TASK-007 (tag-based assignment) paths against the PostgreSQL sink -- these are declared dependencies; regression in either is a FAIL

**Demo script:** `tests/demo/TASK-031-demo.md` does not yet exist. Author it per `demo-script-execution.md` and execute it as part of this pass. Model it on `tests/demo/TASK-030-demo.md`.

**Iteration bounds:** max 3 per Manifest v1. If the implementation cannot converge in 3 iterations, escalate to the Orchestrator -- do not silently extend the loop.

Return a Verification Report at `process/verifier/verification-reports/TASK-031-report.md` with PASS/FAIL per AC, any observations (OBS-031-N), and the completed demo script output. Commit the report when complete.

---

**Next:** Invoke @nexus-orchestrator -- Verifier complete for TASK-031.
