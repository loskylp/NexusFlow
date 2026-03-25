# Skill — Bash Execution

## Rule

Never use `cd <directory> && <command>` compound forms. Always run commands directly from the working directory.

Claude Code treats `cd <dir> && <command>` as a compound requiring explicit approval on every invocation, even when a wildcard permission covers the command. Running from the working directory avoids this and is always safe.

## Wrong

```bash
cd /Users/pablo/projects/MyApp && npm test
cd /Users/pablo/projects/MyApp && git diff --staged
cd backend && npx eslint src/
```

## Right

```bash
npm test
git diff --staged
npx eslint backend/src/
```

## How to apply

- The working directory is always the project root unless the Orchestrator's routing instruction specifies otherwise.
- For commands that target a subdirectory, pass the path as an argument or flag — do not `cd` into it.
- If a tool requires being inside a specific directory, use the tool's built-in path option (e.g., `npm --prefix backend test`) rather than `cd`.
- Apply this rule to every Bash tool call, without exception.
