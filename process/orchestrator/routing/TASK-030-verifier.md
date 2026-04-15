# Routing Instruction

**To:** @nexus-verifier
**Phase:** EXECUTION -- Cycle 4
**Task:** Verify TASK-030 (MinIO Fake-S3 connector -- DataSource + Sink) against all acceptance criteria and the demo script, running the live MinIO container.
**Iteration:** 1 of 3
**Verifier mode:** Initial verification
**Return to:** Orchestrator when complete

---

## Required documents

| Document | Link | Why needed |
|---|---|---|
| TASK-030 spec + acceptance criteria | [process/planner/task-plan.md -- TASK-030](../planner/task-plan.md#task-030-demo-infrastructure----minio-fake-s3) | Acceptance criteria, demo script path, dependencies |
| Builder handoff (wiring fix) | [process/builder/handoff-notes/TASK-030-wiring-handoff.md](../builder/handoff-notes/TASK-030-wiring-handoff.md) | What was built, deviations, integration check steps |
| Builder original handoff | [process/builder/handoff-notes/TASK-030-handoff.md](../builder/handoff-notes/TASK-030-handoff.md) | First-pass connector implementation notes |
| Demo script | [tests/demo/TASK-030-demo.md](../../tests/demo/TASK-030-demo.md) | Demo walkthrough for cycle demo |
| DEMO-001 requirement | [process/analyst/requirements.md#demo-001](../analyst/requirements.md#demo-001) | Source requirement for MinIO demo connector |
| ADR-009 (demo connectors) | [process/architect/adrs/adr-009-demo-connectors.md](../architect/adrs/adr-009-demo-connectors.md) | Architectural decision for Fake-S3 via MinIO |

---

## Skills required

- [`.claude/skills/bash-execution.md`](../../.claude/skills/bash-execution.md) -- absolute paths; no `cd dir && cmd`
- [`.claude/skills/demo-script-execution.md`](../../.claude/skills/demo-script-execution.md) -- demo script authorship/execution
- [`.claude/skills/commit-discipline.md`](../../.claude/skills/commit-discipline.md) -- commit the Verification Report when complete
- [`.claude/skills/traceability-links.md`](../../.claude/skills/traceability-links.md) -- link Verification Report back to ACs and requirements

---

## Context

This is TASK-030's first Verifier pass. Pre-Verifier wiring check caught a nil-wiring issue (RegisterMinIOConnectors was defined but never called from `cmd/worker/main.go`); Builder has now fixed it. Your job is to confirm the runtime wiring actually works end-to-end, not just the unit tests.

**Builder-reported state:**
- `go build ./cmd/worker/` compiles clean
- `go test ./worker/ -run TestMinIO -v` -- 9 PASS
- `cmd/worker/main.go` calls `registerMinIOConnectors(reg)` with env-driven endpoint
- `worker/minio_client.go` provides `MinioClientAdapter` (minio-go/v7 v7.0.91 -- v7.0.100 requires Go 1.25, project is Go 1.23)

**Builder-declared limitations -- confirm these are acceptable or raise as observations:**
1. Adapter uses `PutObject` instead of low-level multipart API (minio-go/v7 doesn't expose it at the high level); in-memory buffering between Create/Complete.
2. No unit tests for `MinioClientAdapter` (I/O wrapper -- integration-only). You must exercise it via the live container.

**Integration verification the Builder asks you to run:**
1. `docker compose --profile demo up` -- worker log should contain `worker: MinIO connectors registered (endpoint=minio:9000 ssl=false)`.
2. With `MINIO_ENDPOINT` unset -- worker log should contain `worker: MINIO_ENDPOINT not set -- MinIO connectors not registered` and startup should complete normally (non-demo deployments unaffected).

**Acceptance focus:**
- Exercise both DataSource and Sink modes against the live MinIO container.
- Validate integration with idempotency/dedup (TASK-018 dependency) and tag-based assignment (TASK-007 dependency).
- Execute the full demo script at `tests/demo/TASK-030-demo.md`.

**Iteration bounds:** max 3 per Manifest v1. If the implementation cannot converge in 3 iterations, escalate to the Orchestrator -- do not silently extend the loop.

Return a Verification Report at `process/verifier/verification-reports/TASK-030-report.md` with PASS/FAIL per AC, any observations (OBS-030-N), and the completed demo script output. Commit the report when complete.

---

**Next:** Invoke @nexus-orchestrator -- Verifier complete for TASK-030.
