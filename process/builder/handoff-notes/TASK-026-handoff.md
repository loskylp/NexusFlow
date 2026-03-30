# Handoff Note — TASK-026: Design-time schema mapping validation

**Task:** TASK-026
**Status:** Complete
**Builder:** Nexus Builder (Cycle 2, Iteration 1)
**Date:** 2026-03-30

---

## What Was Built

### New package: `internal/pipeline`

**`internal/pipeline/validator.go`** — Schema mapping validator.

Exported function `ValidateSchemaMappings(p models.Pipeline) error` validates both pipeline transitions:

1. **DataSource → Process**: each `ProcessConfig.InputMappings[*].SourceField` must appear in `DataSourceConfig.OutputSchema`.
2. **Process → Sink**: each `SinkConfig.InputMappings[*].SourceField` must appear in `ProcessConfig.OutputSchema`.

Internal decomposition:
- `validateProcessInputMappings` — delegates to `validateMappings` with the DS→Process context labels.
- `validateSinkInputMappings` — delegates to `validateMappings` with the Process→Sink context labels.
- `validateMappings` — builds a field set from the schema slice (O(1) lookup) and iterates mappings in order; reports the first violation deterministically.
- `buildFieldSet` — converts `[]string` to `map[string]bool`.

Error message format: `"<phase> input mapping: source field '<field>' not found in <preceding phase> output schema"`

Examples:
- `process input mapping: source field "nonexistent" not found in datasource output schema`
- `sink input mapping: source field "ghost" not found in process output schema`

Empty mappings always pass. A non-empty mapping list against an empty/nil OutputSchema fails with the field name in the error.

### Modified: `api/handlers_pipelines.go`

- Added import of `github.com/nxlabs/nexusflow/internal/pipeline`.
- **`Create`**: calls `pipeline.ValidateSchemaMappings` after name validation, before the repository `Create` call. Returns 400 with the validator error message on failure.
- **`Update`**: calls `pipeline.ValidateSchemaMappings` after the ownership check and before the repository `Update` call. Returns 400 on failure.
- Updated docstrings for both handlers to document the schema validation behavior, the new 400 case, and the postcondition that no pipeline is persisted on a mapping error.

### New tests: `internal/pipeline/validator_test.go`

Unit tests covering all specified behaviours:

| Test | Behaviour |
|---|---|
| `TestValidate_EmptyMappingsPass` | Empty mappings are valid |
| `TestValidate_ValidProcessInputMappingPasses` | Valid DS→Process mapping passes |
| `TestValidate_ValidSinkInputMappingPasses` | Valid Process→Sink mapping passes |
| `TestValidate_MissingProcessSourceFieldReturnsError` | Missing field in DS schema returns error naming the field |
| `TestValidate_MissingSinkSourceFieldReturnsError` | Missing field in Process schema returns error naming the field |
| `TestValidate_EmptyOutputSchemaWithProcessMappingFails` | Non-empty process mappings against nil DS schema fails |
| `TestValidate_EmptyOutputSchemaWithSinkMappingFails` | Non-empty sink mappings against nil Process schema fails |
| `TestValidate_BothTransitionsValidPass` | Fully valid pipeline with mappings on both transitions passes |

### New tests: `api/handlers_pipelines_test.go`

Handler-level integration tests for schema validation wiring:

| Test | Behaviour |
|---|---|
| `TestCreate_ValidSchemaMappingsReturn201` | POST with valid mappings on both transitions returns 201 |
| `TestCreate_InvalidProcessMappingReturns400` | POST with invalid DS→Process mapping returns 400 naming the field |
| `TestCreate_InvalidSinkMappingReturns400` | POST with invalid Process→Sink mapping returns 400 naming the field |
| `TestUpdate_InvalidProcessMappingReturns400` | PUT with invalid mapping returns 400 naming the field |
| `TestUpdate_ValidSchemaMappingsReturn200` | PUT with valid mappings returns 200 |

---

## TDD Cycle

**Red:** Wrote `validator_test.go` and new handler tests before any implementation. Confirmed failures:
- Validator tests: package did not exist — build failure.
- Handler tests: handlers accepted all mappings — 201 returned instead of 400.

**Green:** Implemented `validator.go` with the minimum logic to pass all validator tests. Wired `ValidateSchemaMappings` into `Create` and `Update` handlers.

**Refactor:** Extracted `validateProcessInputMappings`, `validateSinkInputMappings`, `validateMappings`, and `buildFieldSet` as named helpers. Each function has a single responsibility and an accurate docstring. Handler docstrings updated to reflect the new validation step and 400 case.

---

## Test Results

```
go build ./...   — PASS (all packages)
go vet ./...     — PASS (no issues)
go test ./...    — PASS (all packages, all tests)
```

All pre-existing pipeline handler tests continue to pass without modification.

---

## Acceptance Criteria Verification

| AC | Criterion | Status |
|---|---|---|
| AC-1 | POST /api/pipelines with invalid schema mapping returns 400 with error identifying the invalid field | PASS — both Create and Update return 400; error message contains the missing field name |
| AC-2 | Valid schema mappings pass validation | PASS — valid mappings return 201 (Create) and 200 (Update) |
| AC-3 | Both DataSource→Process and Process→Sink mappings are validated | PASS — separate validator helpers cover both transitions; tests cover both |
| AC-4 | Error messages identify the specific field and mapping that failed | PASS — format: `"<phase> input mapping: source field '<field>' not found in <preceding phase> output schema"` |

---

## Deviations

None. Implementation is faithful to the task specification.

---

## Limitations

- Type mismatch validation is not implemented — the task specification and ADR-008 name only field existence checks for design-time validation; type checking remains a runtime concern (TASK-007).
- The validator checks the first failing mapping only and returns immediately; a caller wanting all violations simultaneously would need a different signature. This matches the specified error format and the HTTP handler's single-error response contract.
