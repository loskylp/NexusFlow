# Skill — Demo Script Execution

## What a demo script is

A demo script (`tests/demo/TASK-NNN-demo.md`) is a specification of observable behaviour written by the Verifier at task PASS time. It is not executed per task — it is executed collectively at the Demo Sign-off gate, once all cycle tasks are deployed to staging. The script alone is not evidence; screenshots taken during execution are the evidence.

## Execution protocol (at Demo Sign-off gate)

This protocol is run at the Demo Sign-off gate — not per task. All cycle tasks must be deployed to staging before execution begins.

For every demo script marked as browser-testable (not DB-only or curl-only):

1. Open the application in the browser (Playwright)
2. Execute each scenario in the demo script step by step
3. Take a screenshot at the key observable moment of each scenario
4. Save screenshots to `tests/demo/TASK-NNN/` with descriptive filenames:
   - `01-<scenario-slug>.png`
   - `02-<scenario-slug>.png`
   - etc.
5. Commit screenshots alongside the demo script

## Screenshot naming

```
tests/demo/TASK-005/
  01-login-success.png
  02-workspace-empty-state.png
  03-note-created.png
```

Filenames must be lowercase, hyphen-separated, and prefixed with a two-digit sequence number matching the scenario order in the demo script.

## Consolidated cycle demos

Per-task demo scripts are the norm. Consolidation is the exception.

A consolidated demo script covers all features of a cycle in a single end-to-end walkthrough. When a consolidated script is used, screenshots go in `tests/demo/cycle-N/`. This is only valid when the consolidated script explicitly covers every task in the cycle — no task may be left out. If any task lacks coverage in the consolidated script, that task must have its own demo script and per-task directory.

## curl / API-only scripts

Scripts that test backend endpoints only (no browser interaction) do not require Playwright screenshots. They require the full terminal session captured and noted in the verification report:

- **curl:** record both the exact command executed and the full response (status line + body). The command alone proves nothing; the response alone is unverifiable.
- **Test runner:** record the full test output log — this includes the test names, suite context, pass/fail lines, and summary. Do not truncate to only the final result line.

If a frontend button for the same feature exists, a separate browser scenario must be added to the demo script and executed via Playwright.

## Commit message

```
demo(TASK-NNN): add Playwright screenshots for <task title>
```
