# Skill — Commit Discipline

## Who commits what

| Agent | Commits | Does not commit |
|---|---|---|
| Builder | Nothing | All source changes — Builder hands off uncommitted work to Verifier |
| Verifier | Source changes (Builder's code + Verifier's acceptance tests) after task PASS + CI green | Nothing before CI passes |
| Orchestrator | Process artefacts after each agent produces output | Source code |
| DevOps | Infrastructure files after first-push CI green confirmation | Nothing before CI passes |

## Verifier commit protocol (per task)

1. Verifier runs acceptance tests — all pass (task PASS verdict)
2. Verifier stages: Builder's implementation files + Verifier's acceptance test files
3. Verifier commits with message: `task(TASK-NNN): <task title> — verified PASS`
4. Verifier pushes to remote
5. Verifier waits for CI pipeline to complete (all jobs green)
6. **If CI regression FAIL:** Orchestrator dispatches Builder to fix → Verifier re-runs task tests + failing regression tests → push → wait → loop
7. **If CI regression PASS:** task is COMPLETE — Orchestrator updates project-state.md

A task is not COMPLETE until CI regression is green. A commit without a push is not a completed verification.

## Orchestrator commit protocol (per agent output)

After receiving output from any agent, commit the process artefacts that agent produced before routing to the next agent:

```
git add process/<agent>/
git commit -m "process(<agent>): <artefact name> — <one-line description>"
```

Examples:
- `process(analyst): requirements-v2.md — mid-cycle REQ-018 REQ-019`
- `process(verifier): verification-report-task-005.md — PASS 12/12`
- `process(planner): task-plan-v2.md — Cycle 2 plan with tagging tasks`

## DevOps Phase 1 commit protocol

After writing all infrastructure files (CI workflow, Dockerfile, compose files):

1. Stage and commit all infrastructure files
2. Push to remote
3. Wait for the GitHub Actions run to complete
4. Confirm all CI jobs pass
5. Only then mark DevOps Phase 1 COMPLETE

"The files look correct" is not the same as "the pipeline runs." Phase 1 is not complete until a real CI run is green.

## Lint before commit

Before committing, run the linter locally on the files in scope for the current task. Do not rely on CI to catch lint errors first. Full regression lint still runs on CI — local lint is a fast pre-check, not a replacement.
